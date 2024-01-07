/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/database"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/egress_network_policy"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/ingress_network_policy"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/internet_network_policy"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/port_egress_network_policy"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/port_network_policy"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/protected_services"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	"github.com/otterize/intents-operator/src/shared/initonce"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/otterize/intents-operator/src/shared/reconcilergroup"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesconfig"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var intentsLegacyFinalizers = []string{
	"intents.otterize.com/svc-network-policy-finalizer",
	"intents.otterize.com/network-policy-finalizer",
	"intents.otterize.com/telemetry-reconciler-finalizer",
	"intents.otterize.com/kafka-finalizer",
	"intents.otterize.com/istio-policy-finalizer",
	"intents.otterize.com/database-finalizer",
	"intents.otterize.com/pods-finalizer",
}

type EnforcementConfig struct {
	EnforcementDefaultState              bool
	EnableNetworkPolicy                  bool
	EnableKafkaACL                       bool
	EnableIstioPolicy                    bool
	EnableDatabasePolicy                 bool
	EnableEgressNetworkPolicyReconcilers bool
	EnableAWSPolicy                      bool
}

// IntentsReconciler reconciles a Intents object
type IntentsReconciler struct {
	group                   *reconcilergroup.Group
	client                  client.Client
	initOnce                initonce.InitOnce
	networkPolicyReconciler *ingress_network_policy.NetworkPolicyReconciler
}

func NewIntentsReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	kafkaServerStore kafkaacls.ServersStore,
	networkPolicyReconciler *ingress_network_policy.NetworkPolicyReconciler,
	portNetpolReconciler *port_network_policy.PortNetworkPolicyReconciler,
	egressNetpolReconciler *egress_network_policy.EgressNetworkPolicyReconciler,
	portEgressNetpolReconciler *port_egress_network_policy.PortEgressNetworkPolicyReconciler,
	restrictToNamespaces []string,
	enforcementConfig EnforcementConfig,
	otterizeClient operator_cloud_client.CloudClient,
	operatorPodName string,
	operatorPodNamespace string,
	additionalReconcilers ...reconcilergroup.ReconcilerWithEvents,
) *IntentsReconciler {

	serviceIdResolver := serviceidresolver.NewResolver(client)
	reconcilers := []reconcilergroup.ReconcilerWithEvents{
		intents_reconcilers.NewCRDValidatorReconciler(client, scheme),
		intents_reconcilers.NewPodLabelReconciler(client, scheme),
		intents_reconcilers.NewKafkaACLReconciler(client, scheme, kafkaServerStore, enforcementConfig.EnableKafkaACL, kafkaacls.NewKafkaIntentsAdmin, enforcementConfig.EnforcementDefaultState, operatorPodName, operatorPodNamespace, serviceIdResolver),
		intents_reconcilers.NewIstioPolicyReconciler(client, scheme, restrictToNamespaces, enforcementConfig.EnableIstioPolicy, enforcementConfig.EnforcementDefaultState),
		networkPolicyReconciler,
	}
	reconcilers = append(reconcilers, additionalReconcilers...)
	reconcilersGroup := reconcilergroup.NewGroup(
		"intents-reconciler",
		client,
		scheme,
		&otterizev1alpha3.ClientIntents{},
		otterizev1alpha3.ClientIntentsFinalizerName,
		intentsLegacyFinalizers,
		reconcilers...,
	)

	reconcilersGroup.AddToGroup(portNetpolReconciler)

	intentsReconciler := &IntentsReconciler{
		group:                   reconcilersGroup,
		client:                  client,
		networkPolicyReconciler: networkPolicyReconciler,
	}

	if telemetriesconfig.IsUsageTelemetryEnabled() {
		telemetryReconciler := intents_reconcilers.NewTelemetryReconciler(client, scheme)
		intentsReconciler.group.AddToGroup(telemetryReconciler)
	}

	if otterizeClient != nil {
		otterizeCloudReconciler := intents_reconcilers.NewOtterizeCloudReconciler(client, scheme, otterizeClient, serviceIdResolver)
		intentsReconciler.group.AddToGroup(otterizeCloudReconciler)
	}

	if enforcementConfig.EnableDatabasePolicy {
		databaseReconciler := database.NewDatabaseReconciler(client, scheme, otterizeClient)
		intentsReconciler.group.AddToGroup(databaseReconciler)
	}

	if enforcementConfig.EnableEgressNetworkPolicyReconcilers {
		internetNetpolReconciler := internet_network_policy.NewInternetNetworkPolicyReconciler(client, scheme, restrictToNamespaces, enforcementConfig.EnableNetworkPolicy, enforcementConfig.EnforcementDefaultState)
		intentsReconciler.group.AddToGroup(internetNetpolReconciler)
		intentsReconciler.group.AddToGroup(egressNetpolReconciler)
		intentsReconciler.group.AddToGroup(portEgressNetpolReconciler)
	}

	return intentsReconciler
}

//+kubebuilder:rbac:groups=k8s.otterize.com,resources=clientintents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=clientintents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=clientintents/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;update;patch;list;watch
//+kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;update;patch;list;watch;delete;create
//+kubebuilder:rbac:groups="admissionregistration.k8s.io",resources=validatingwebhookconfigurations,verbs=get;update;patch;list
//+kubebuilder:rbac:groups="apiextensions.k8s.io",resources=customresourcedefinitions,verbs=get;list;watch;update;create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *IntentsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	err := r.initOnce.Do(func() error {
		return r.intentsReconcilerInit(ctx)
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	intents := &otterizev1alpha3.ClientIntents{}

	err = r.client.Get(ctx, req.NamespacedName, intents)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	result, err := r.group.Reconcile(ctx, req)
	if err != nil {
		return ctrl.Result{}, err
	}

	intents.Status.UpToDate = true
	if err := r.client.Status().Update(ctx, intents); err != nil {
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *IntentsReconciler) intentsReconcilerInit(ctx context.Context) error {
	return r.networkPolicyReconciler.CleanAllNamespaces(ctx)
}

// SetupWithManager sets up the controller with the Manager.
func (r *IntentsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&otterizev1alpha3.ClientIntents{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Watches(&otterizev1alpha3.ProtectedService{}, handler.EnqueueRequestsFromMapFunc(r.mapProtectedServiceToClientIntents)).
		Watches(&corev1.Endpoints{}, handler.EnqueueRequestsFromMapFunc(r.watchApiServerEndpoint)).
		Complete(r)
	if err != nil {
		return err
	}

	r.group.InjectRecorder(mgr.GetEventRecorderFor("intents-operator"))

	return nil
}

func (r *IntentsReconciler) watchApiServerEndpoint(_ context.Context, obj client.Object) []reconcile.Request {
	if obj.GetNamespace() != otterizev1alpha3.KubernetesAPIServerNamespace || obj.GetName() != otterizev1alpha3.KubernetesAPIServerName {
		return nil
	}

	intentsToReconcile := r.getIntentsToAPIServerService()
	return r.mapIntentsToRequests(intentsToReconcile)
}

func (r *IntentsReconciler) getIntentsToAPIServerService() []otterizev1alpha3.ClientIntents {
	intentsToReconcile := make([]otterizev1alpha3.ClientIntents, 0)
	fullServerName := fmt.Sprintf("svc:%s.%s", otterizev1alpha3.KubernetesAPIServerName, otterizev1alpha3.KubernetesAPIServerNamespace)
	var intentsToServer otterizev1alpha3.ClientIntentsList
	err := r.client.List(context.Background(),
		&intentsToServer,
		&client.MatchingFields{otterizev1alpha3.OtterizeTargetServerIndexField: fullServerName},
	)
	if err != nil {
		logrus.WithError(err).Errorf("Failed to list client intents for client %s", fullServerName)
		return nil
	}
	logrus.Infof("Enqueueing client intents %v for api server", intentsToServer.Items)

	intentsToReconcile = append(intentsToReconcile, intentsToServer.Items...)
	return intentsToReconcile
}

func (r *IntentsReconciler) mapProtectedServiceToClientIntents(_ context.Context, obj client.Object) []reconcile.Request {
	protectedService := obj.(*otterizev1alpha3.ProtectedService)
	logrus.Infof("Enqueueing client intents for protected services %s", protectedService.Name)

	intentsToReconcile := r.getIntentsToProtectedService(protectedService)
	return r.mapIntentsToRequests(intentsToReconcile)
}

func (r *IntentsReconciler) mapIntentsToRequests(intentsToReconcile []otterizev1alpha3.ClientIntents) []reconcile.Request {
	requests := make([]reconcile.Request, 0)
	for _, clientIntents := range intentsToReconcile {
		request := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      clientIntents.Name,
				Namespace: clientIntents.Namespace,
			},
		}
		requests = append(requests, request)
	}
	return requests
}

func (r *IntentsReconciler) getIntentsToProtectedService(protectedService *otterizev1alpha3.ProtectedService) []otterizev1alpha3.ClientIntents {
	intentsToReconcile := make([]otterizev1alpha3.ClientIntents, 0)
	fullServerName := fmt.Sprintf("%s.%s", protectedService.Spec.Name, protectedService.Namespace)
	var intentsToServer otterizev1alpha3.ClientIntentsList
	err := r.client.List(context.Background(),
		&intentsToServer,
		&client.MatchingFields{otterizev1alpha3.OtterizeTargetServerIndexField: fullServerName},
	)
	if err != nil {
		logrus.Errorf("Failed to list client intents for client %s: %v", fullServerName, err)
	}

	intentsToReconcile = append(intentsToReconcile, intentsToServer.Items...)
	return intentsToReconcile
}

// InitIntentsServerIndices indexes intents by target server name
// This is used in finalizers to determine whether a network policy should be removed from the target namespace
func (r *IntentsReconciler) InitIntentsServerIndices(mgr ctrl.Manager) error {
	err := mgr.GetCache().IndexField(
		context.Background(),
		&otterizev1alpha3.ClientIntents{},
		otterizev1alpha3.OtterizeTargetServerIndexField,
		func(object client.Object) []string {
			var res []string
			intents := object.(*otterizev1alpha3.ClientIntents)
			if intents.Spec == nil {
				return nil
			}

			for _, intent := range intents.GetCallsList() {
				if !intent.IsTargetServerKubernetesService() {
					res = append(res, intent.GetServerFullyQualifiedName(intents.Namespace))
				}
				fullyQualifiedSvcName, ok := intent.GetK8sServiceFullyQualifiedName(intents.Namespace)
				if ok {
					res = append(res, fullyQualifiedSvcName)
				}
			}

			return res
		})
	if err != nil {
		return err
	}

	err = mgr.GetCache().IndexField(
		context.Background(),
		&otterizev1alpha3.ClientIntents{},
		otterizev1alpha3.OtterizeFormattedTargetServerIndexField,
		func(object client.Object) []string {
			var res []string
			intents := object.(*otterizev1alpha3.ClientIntents)
			if intents.Spec == nil {
				return nil
			}

			for _, intent := range intents.GetCallsList() {
				if intent.Type == otterizev1alpha3.IntentTypeInternet {
					res = append(res, otterizev1alpha3.OtterizeInternetTargetName)
					continue
				}
				serverName := intent.GetTargetServerName()
				serverNamespace := intent.GetTargetServerNamespace(intents.Namespace)
				formattedServerName := otterizev1alpha3.GetFormattedOtterizeIdentity(serverName, serverNamespace)
				if !intent.IsTargetServerKubernetesService() {
					res = append(res, formattedServerName)
				} else {
					res = append(res, "svc:"+formattedServerName)
				}
			}

			return res
		})
	if err != nil {
		return err
	}

	return nil
}

// InitProtectedServiceIndexField indexes protected service resources by their service name
// This is used in finalizers to determine whether a network policy should be removed from the target namespace
func (r *IntentsReconciler) InitProtectedServiceIndexField(mgr ctrl.Manager) error {
	return protected_services.InitProtectedServiceIndexField(mgr)
}

func (r *IntentsReconciler) InitEndpointsPodNamesIndex(mgr ctrl.Manager) error {
	err := mgr.GetCache().IndexField(
		context.Background(),
		&corev1.Endpoints{},
		otterizev1alpha3.EndpointsPodNamesIndexField,
		func(object client.Object) []string {
			var res []string
			endpoints := object.(*corev1.Endpoints)
			addresses := make([]corev1.EndpointAddress, 0)
			for _, subset := range endpoints.Subsets {
				addresses = append(addresses, subset.Addresses...)
				addresses = append(addresses, subset.NotReadyAddresses...)
			}

			for _, address := range addresses {
				if address.TargetRef == nil || address.TargetRef.Kind != "Pod" {
					continue
				}

				res = append(res, address.TargetRef.Name)
			}

			return res
		})

	if err != nil {
		return err
	}

	return nil
}
