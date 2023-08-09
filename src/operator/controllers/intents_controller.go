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
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/operator/controllers/external_traffic"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/exp"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/otterize/intents-operator/src/shared/reconcilergroup"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type EnforcementConfig struct {
	EnforcementDefaultState  bool
	EnableNetworkPolicy      bool
	EnableKafkaACL           bool
	EnableIstioPolicy        bool
	EnableDatabaseReconciler bool
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
	externalNetpolHandler *external_traffic.NetworkPolicyHandler,
	restrictToNamespaces []string,
	enforcementConfig EnforcementConfig,
	externalNetworkPoliciesCreatedEvenIfNoIntents bool,
	otterizeClient operator_cloud_client.CloudClient,
	operatorPodName string,
	operatorPodNamespace string) *IntentsReconciler {
	reconcilersGroup := reconcilergroup.NewGroup("intents-reconciler", client, scheme,
		intents_reconcilers.NewCRDValidatorReconciler(client, scheme),
		intents_reconcilers.NewPodLabelReconciler(client, scheme),
		intents_reconcilers.NewNetworkPolicyReconciler(client, scheme, externalNetpolHandler, restrictToNamespaces, enforcementConfig.EnableNetworkPolicy, enforcementConfig.EnforcementDefaultState, externalNetworkPoliciesCreatedEvenIfNoIntents),
		intents_reconcilers.NewKafkaACLReconciler(client, scheme, kafkaServerStore, enforcementConfig.EnableKafkaACL, kafkaacls.NewKafkaIntentsAdmin, enforcementConfig.EnforcementDefaultState, operatorPodName, operatorPodNamespace, serviceidresolver.NewResolver(client)),
		intents_reconcilers.NewIstioPolicyReconciler(client, scheme, restrictToNamespaces, enforcementConfig.EnableIstioPolicy, enforcementConfig.EnforcementDefaultState),
	)

	intentsReconciler := &IntentsReconciler{
		group:  reconcilersGroup,
		client: client,
	}

	if otterizeClient != nil {
		otterizeCloudReconciler := intents_reconcilers.NewOtterizeCloudReconciler(client, scheme, otterizeClient)
		intentsReconciler.group.AddToGroup(otterizeCloudReconciler)
	}

	if enforcementConfig.EnableDatabaseReconciler {
		databaseReconciler := exp.NewDatabaseReconciler(client, scheme, otterizeClient)
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
//+kubebuilder:rbac:groups="apiextensions.k8s.io",resources=customresourcedefinitions,verbs=get;list;watch;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *IntentsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.group.Reconcile(ctx, req)
}

// SetupWithManager sets up the controller with the Manager.
func (r *IntentsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&otterizev1alpha2.ClientIntents{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Watches(&source.Kind{Type: &otterizev1alpha2.ProtectedService{}}, handler.EnqueueRequestsFromMapFunc(r.mapProtectedServiceToClientIntents)).
		Complete(r)
	if err != nil {
		return err
	}

	r.group.InjectRecorder(mgr.GetEventRecorderFor("intents-operator"))

	return nil
}

func (r *IntentsReconciler) mapProtectedServiceToClientIntents(obj client.Object) []reconcile.Request {
	protectedService := obj.(*otterizev1alpha2.ProtectedService)
	logrus.Infof("Enqueueing client intents for protected services %s", protectedService.Name)

	intentsToReconcile := r.getIntentsToProtectedService(protectedService)
	return r.mapIntentsToRequests(intentsToReconcile)
}

func (r *IntentsReconciler) mapIntentsToRequests(intentsToReconcile []otterizev1alpha2.ClientIntents) []reconcile.Request {
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

func (r *IntentsReconciler) getIntentsToProtectedService(protectedService *otterizev1alpha2.ProtectedService) []otterizev1alpha2.ClientIntents {
	intentsToReconcile := make([]otterizev1alpha2.ClientIntents, 0)
	fullServerName := fmt.Sprintf("%s.%s", protectedService.Spec.Name, protectedService.Namespace)
	var intentsToServer otterizev1alpha2.ClientIntentsList
	err := r.client.List(context.Background(),
		&intentsToServer,
		&client.MatchingFields{otterizev1alpha2.OtterizeTargetServerIndexField: fullServerName},
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
		&otterizev1alpha2.ClientIntents{},
		otterizev1alpha2.OtterizeTargetServerIndexField,
		func(object client.Object) []string {
			var res []string
			intents := object.(*otterizev1alpha2.ClientIntents)
			if intents.Spec == nil {
				return nil
			}

			for _, intent := range intents.GetCallsList() {
				res = append(res, intent.GetServerFullyQualifiedName(intents.Namespace))
			}

			return res
		})
	if err != nil {
		return err
	}

	err = mgr.GetCache().IndexField(
		context.Background(),
		&otterizev1alpha2.ClientIntents{},
		otterizev1alpha2.OtterizeFormattedTargetServerIndexField,
		func(object client.Object) []string {
			var res []string
			intents := object.(*otterizev1alpha2.ClientIntents)
			if intents.Spec == nil {
				return nil
			}

			for _, intent := range intents.GetCallsList() {
				serverName := intent.GetServerName()
				serverNamespace := intent.GetServerNamespace(intents.Namespace)
				formattedServerName := otterizev1alpha2.GetFormattedOtterizeIdentity(serverName, serverNamespace)
				res = append(res, formattedServerName)
			}

			return res
		})
	if err != nil {
		return err
	}

	return nil
}

func (r *IntentsReconciler) InitEndpointsPodNamesIndex(mgr ctrl.Manager) error {
	err := mgr.GetCache().IndexField(
		context.Background(),
		&corev1.Endpoints{},
		otterizev1alpha2.EndpointsPodNamesIndexField,
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
