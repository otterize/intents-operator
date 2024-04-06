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
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/protected_services"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/otterize/intents-operator/src/shared/reconcilergroup"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
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
	EnableGCPPolicy                      bool
	EnableAzurePolicy                    bool
}

// IntentsReconciler reconciles a Intents object
type IntentsReconciler struct {
	group  *reconcilergroup.Group
	client client.Client
}

func NewIntentsReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	kafkaServerStore kafkaacls.ServersStore,
	restrictToNamespaces []string,
	enforcementConfig EnforcementConfig,
	otterizeClient operator_cloud_client.CloudClient,
	operatorPodName string,
	operatorPodNamespace string,
	additionalReconcilers ...reconcilergroup.ReconcilerWithEvents,
) *IntentsReconciler {

	serviceIdResolver := serviceidresolver.NewResolver(client)
	reconcilers := []reconcilergroup.ReconcilerWithEvents{
		intents_reconcilers.NewPodLabelReconciler(client, scheme),
		intents_reconcilers.NewKafkaACLReconciler(client, scheme, kafkaServerStore, enforcementConfig.EnableKafkaACL, kafkaacls.NewKafkaIntentsAdmin, enforcementConfig.EnforcementDefaultState, operatorPodName, operatorPodNamespace, serviceIdResolver),
		intents_reconcilers.NewIstioPolicyReconciler(client, scheme, restrictToNamespaces, enforcementConfig.EnableIstioPolicy, enforcementConfig.EnforcementDefaultState),
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

	intentsReconciler := &IntentsReconciler{
		group:  reconcilersGroup,
		client: client,
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

	return intentsReconciler
}

//+kubebuilder:rbac:groups=k8s.otterize.com,resources=clientintents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=clientintents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=clientintents/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;update;patch;list;watch
//+kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;update;patch;list;watch;delete;create
//+kubebuilder:rbac:groups="admissionregistration.k8s.io",resources=validatingwebhookconfigurations,verbs=get;update;patch;list
//+kubebuilder:rbac:groups="apiextensions.k8s.io",resources=customresourcedefinitions,verbs=get;list;watch;update;create;patch
// +kubebuilder:rbac:groups=iam.cnrm.cloud.google.com,resources=iampartialpolicies,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *IntentsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	intents := &otterizev1alpha3.ClientIntents{}

	err := r.client.Get(ctx, req.NamespacedName, intents)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrap(err)
	}

	if intents.Status.UpToDate != false && intents.Status.ObservedGeneration != intents.Generation {
		intentsCopy := intents.DeepCopy()
		intentsCopy.Status.UpToDate = false
		if err := r.client.Status().Patch(ctx, intentsCopy, client.MergeFrom(intents)); err != nil {
			return ctrl.Result{}, errors.Wrap(err)
		}
		// we have to finish this reconcile loop here so that the group has a fresh copy of the intents
		// and that we don't trigger an infinite loop
		return ctrl.Result{}, nil
	}

	result, err := r.group.Reconcile(ctx, req)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	if intents.DeletionTimestamp == nil {
		intentsCopy := intents.DeepCopy()
		intentsCopy.Status.UpToDate = true
		intentsCopy.Status.ObservedGeneration = intentsCopy.Generation
		if err := r.client.Status().Patch(ctx, intentsCopy, client.MergeFrom(intents)); err != nil {
			return ctrl.Result{}, errors.Wrap(err)
		}
	}

	return result, nil
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
		return errors.Wrap(err)
	}

	r.group.InjectRecorder(mgr.GetEventRecorderFor("intents-operator"))

	return nil
}

func (r *IntentsReconciler) watchApiServerEndpoint(ctx context.Context, obj client.Object) []reconcile.Request {
	if obj.GetNamespace() != otterizev1alpha3.KubernetesAPIServerNamespace || obj.GetName() != otterizev1alpha3.KubernetesAPIServerName {
		return nil
	}

	intentsToReconcile := r.getIntentsToAPIServerService(ctx)
	return r.mapIntentsToRequests(intentsToReconcile)
}

func (r *IntentsReconciler) getIntentsToAPIServerService(ctx context.Context) []otterizev1alpha3.ClientIntents {
	intentsToReconcile := make([]otterizev1alpha3.ClientIntents, 0)
	fullServerName := fmt.Sprintf("svc:%s.%s", otterizev1alpha3.KubernetesAPIServerName, otterizev1alpha3.KubernetesAPIServerNamespace)
	var intentsToServer otterizev1alpha3.ClientIntentsList
	err := r.client.List(
		ctx,
		&intentsToServer,
		&client.MatchingFields{otterizev1alpha3.OtterizeTargetServerIndexField: fullServerName},
	)
	if err != nil {
		logrus.WithError(err).Errorf("Failed to list client intents for client %s", fullServerName)
		return nil
	}
	logrus.Debugf("Enqueueing client intents %v for api server", intentsToServer.Items)

	intentsToReconcile = append(intentsToReconcile, intentsToServer.Items...)
	return intentsToReconcile
}

func (r *IntentsReconciler) mapProtectedServiceToClientIntents(ctx context.Context, obj client.Object) []reconcile.Request {
	protectedService := obj.(*otterizev1alpha3.ProtectedService)
	logrus.Debugf("Enqueueing client intents for protected services %s", protectedService.Name)

	intentsToReconcile := r.getIntentsToProtectedService(ctx, protectedService)
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

func (r *IntentsReconciler) getIntentsToProtectedService(ctx context.Context, protectedService *otterizev1alpha3.ProtectedService) []otterizev1alpha3.ClientIntents {
	intentsToReconcile := make([]otterizev1alpha3.ClientIntents, 0)
	fullServerName := fmt.Sprintf("%s.%s", protectedService.Spec.Name, protectedService.Namespace)
	var intentsToServer otterizev1alpha3.ClientIntentsList
	err := r.client.List(ctx,
		&intentsToServer,
		&client.MatchingFields{otterizev1alpha3.OtterizeTargetServerIndexField: fullServerName},
	)
	if err != nil {
		logrus.Errorf("Failed to list client intents for client %s: %v", fullServerName, err)
		// Intentionally no return - we are not able to return errors in this flow currently
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
		return errors.Wrap(err)
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
				service := serviceidentity.NewFromIntent(intent, intents.Namespace)
				res = append(res, service.GetFormattedOtterizeIdentity())
			}

			return res
		})
	if err != nil {
		return errors.Wrap(err)
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
		return errors.Wrap(err)
	}

	return nil
}
