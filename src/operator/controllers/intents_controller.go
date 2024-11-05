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
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/access_annotation"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/database"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/protected_services"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	"github.com/otterize/intents-operator/src/operator/health"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/enforcement"
	"github.com/otterize/intents-operator/src/shared/reconcilergroup"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
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
	"time"
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
	enforcementConfig enforcement.Config,
	operatorPodName string,
	operatorPodNamespace string,
	additionalReconcilers ...reconcilergroup.ReconcilerWithEvents,
) *IntentsReconciler {

	serviceIdResolver := serviceidresolver.NewResolver(client)
	reconcilers := []reconcilergroup.ReconcilerWithEvents{
		intents_reconcilers.NewPodLabelReconciler(client, scheme),
		intents_reconcilers.NewKafkaACLReconciler(client, scheme, kafkaServerStore, enforcementConfig.EnableKafkaACL, kafkaacls.NewKafkaIntentsAdmin, enforcementConfig.EnforcementDefaultState, operatorPodName, operatorPodNamespace, serviceIdResolver, enforcementConfig.EnforcedNamespaces),
		intents_reconcilers.NewIstioPolicyReconciler(client, scheme, restrictToNamespaces, enforcementConfig.EnableIstioPolicy, enforcementConfig.EnforcementDefaultState, enforcementConfig.EnforcedNamespaces),
	}
	reconcilers = append(reconcilers, additionalReconcilers...)
	reconcilersGroup := reconcilergroup.NewGroup(
		"intents-reconciler",
		client,
		scheme,
		&otterizev2alpha1.ClientIntents{},
		otterizev2alpha1.ClientIntentsFinalizerName,
		intentsLegacyFinalizers,
		reconcilers...,
	)

	intentsReconciler := &IntentsReconciler{
		group:  reconcilersGroup,
		client: client,
	}

	if enforcementConfig.EnableDatabasePolicy {
		databaseReconciler := database.NewDatabaseReconciler(client, scheme)
		intentsReconciler.group.AddToGroup(databaseReconciler)
	}

	return intentsReconciler
}

//+kubebuilder:rbac:groups=k8s.otterize.com,resources=clientintents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=postgresqlserverconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=mysqlserverconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=clientintents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=clientintents/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;update;patch;list;watch
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;update;patch;list;watch;create
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get
//+kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;update;patch;list;watch;delete;create
//+kubebuilder:rbac:groups="admissionregistration.k8s.io",resources=validatingwebhookconfigurations,verbs=get;update;patch;list
//+kubebuilder:rbac:groups="apiextensions.k8s.io",resources=customresourcedefinitions,verbs=get;list;watch;update;create;patch
//+kubebuilder:rbac:groups="apiextensions.k8s.io",resources=customresourcedefinitions,verbs=get;list;watch;update;create;patch
// +kubebuilder:rbac:groups=iam.cnrm.cloud.google.com,resources=iampartialpolicies,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *IntentsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	intents := &otterizev2alpha1.ClientIntents{}

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

	health.UpdateLastReconcileStartTime()

	timeoutCtx, cancel := context.WithTimeoutCause(ctx, 60*time.Second, errors.Errorf("timeout while reconciling client intents %s", req.NamespacedName))
	defer cancel()

	result, err := r.group.Reconcile(timeoutCtx, req)
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

	// Only consider reconcile ended if no error and no requeue.
	if result.IsZero() {
		health.UpdateLastReconcileEndTime()
	}
	return result, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IntentsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&otterizev2alpha1.ClientIntents{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Watches(&otterizev2alpha1.ProtectedService{}, handler.EnqueueRequestsFromMapFunc(r.mapProtectedServiceToClientIntents)).
		Watches(&corev1.Endpoints{}, handler.EnqueueRequestsFromMapFunc(r.watchApiServerEndpoint)).
		Watches(&otterizev2alpha1.PostgreSQLServerConfig{}, handler.EnqueueRequestsFromMapFunc(r.mapPostgresInstanceNameToDatabaseIntents)).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(r.mapPodWithAccessAnnotationToClientIntents)).
		Complete(r)
	if err != nil {
		return errors.Wrap(err)
	}

	r.group.InjectRecorder(mgr.GetEventRecorderFor("intents-operator"))

	return nil
}

func (r *IntentsReconciler) mapPodWithAccessAnnotationToClientIntents(ctx context.Context, obj client.Object) []reconcile.Request {
	pod := obj.(*corev1.Pod)
	logrus.Debugf("Enqueueing client intents for pod %s", pod.Name)

	clients, ok, err := access_annotation.ParseAccessAnnotations(pod)
	if err != nil {
		// If parsing fails an error will be recorded by the pods reconciler so here we just log it
		logrus.WithError(err).Errorf("Failed to parse access annotations for pod %s", pod.Name)
		return nil
	}
	if !ok {
		return nil
	}

	requests := make([]reconcile.Request, 0)
	for _, clientService := range clients {
		intentsToReconcile := r.getIntentsToService(ctx, clientService.Name, clientService.Namespace)
		serviceRequests := r.mapIntentsToRequests(intentsToReconcile)

		requests = append(requests, serviceRequests...)
	}
	return requests
}

func (r *IntentsReconciler) watchApiServerEndpoint(ctx context.Context, obj client.Object) []reconcile.Request {
	if obj.GetNamespace() != otterizev2alpha1.KubernetesAPIServerNamespace || obj.GetName() != otterizev2alpha1.KubernetesAPIServerName {
		return nil
	}

	intentsToReconcile := r.getIntentsToAPIServerService(ctx)
	return r.mapIntentsToRequests(intentsToReconcile)
}

func (r *IntentsReconciler) getIntentsToAPIServerService(ctx context.Context) []otterizev2alpha1.ClientIntents {
	intentsToReconcile := make([]otterizev2alpha1.ClientIntents, 0)
	fullServerName := fmt.Sprintf("svc:%s.%s", otterizev2alpha1.KubernetesAPIServerName, otterizev2alpha1.KubernetesAPIServerNamespace)
	var intentsToServer otterizev2alpha1.ClientIntentsList
	err := r.client.List(
		ctx,
		&intentsToServer,
		&client.MatchingFields{otterizev2alpha1.OtterizeTargetServerIndexField: fullServerName},
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
	protectedService := obj.(*otterizev2alpha1.ProtectedService)
	logrus.Debugf("Enqueueing client intents for protected services %s", protectedService.Name)

	intentsToReconcile := r.getIntentsToService(ctx, protectedService.Spec.Name, protectedService.Namespace)
	return r.mapIntentsToRequests(intentsToReconcile)
}

func (r *IntentsReconciler) mapPostgresInstanceNameToDatabaseIntents(_ context.Context, obj client.Object) []reconcile.Request {
	pgServerConf := obj.(*otterizev2alpha1.PostgreSQLServerConfig)
	logrus.Infof("Enqueueing client intents for PostgreSQLServerConfig change %s", pgServerConf.Name)

	intentsToReconcile := r.getIntentsToPostgresInstance(pgServerConf)

	requests := make([]reconcile.Request, 0)
	for _, clientIntents := range intentsToReconcile {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      clientIntents.Name,
				Namespace: clientIntents.Namespace,
			},
		})
	}
	return requests
}

func (r *IntentsReconciler) getIntentsToPostgresInstance(pgServerConf *otterizev2alpha1.PostgreSQLServerConfig) []otterizev2alpha1.ClientIntents {
	intentsList := otterizev2alpha1.ClientIntentsList{}
	dbInstanceName := pgServerConf.Name
	err := r.client.List(context.Background(),
		&intentsList,
		&client.MatchingFields{otterizev2alpha1.OtterizeTargetServerIndexField: dbInstanceName},
	)
	if err != nil {
		logrus.Errorf("Failed to list client intents targeting %s: %v", dbInstanceName, err)
	}

	return intentsList.Items
}

func (r *IntentsReconciler) mapIntentsToRequests(intentsToReconcile []otterizev2alpha1.ClientIntents) []reconcile.Request {
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

func (r *IntentsReconciler) getIntentsToService(ctx context.Context, serviceName string, namespace string) []otterizev2alpha1.ClientIntents {
	intentsToReconcile := make([]otterizev2alpha1.ClientIntents, 0)
	fullServerName := fmt.Sprintf("%s.%s", serviceName, namespace)
	var intentsToServer otterizev2alpha1.ClientIntentsList
	err := r.client.List(ctx,
		&intentsToServer,
		&client.MatchingFields{otterizev2alpha1.OtterizeTargetServerIndexField: fullServerName},
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
		&otterizev2alpha1.ClientIntents{},
		otterizev2alpha1.OtterizeTargetServerIndexField,
		func(object client.Object) []string {
			var res []string
			intents := object.(*otterizev2alpha1.ClientIntents)
			if intents.Spec == nil {
				return nil
			}

			for _, intent := range intents.GetTargetList() {
				if !intent.IsTargetServerKubernetesService() {
					res = append(res, intent.GetServerFullyQualifiedName(intents.Namespace))
				}
				if intent.SQL != nil {
					res = append(res, intent.GetTargetServerName())
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
		&otterizev2alpha1.ClientIntents{},
		otterizev2alpha1.OtterizeFormattedTargetServerIndexField,
		func(object client.Object) []string {
			var res []string
			intents := object.(*otterizev2alpha1.ClientIntents)
			if intents.Spec == nil {
				return nil
			}

			for _, intent := range intents.GetTargetList() {
				if intent.Internet != nil {
					res = append(res, otterizev2alpha1.OtterizeInternetTargetName)
					continue
				}
				service := intent.ToServiceIdentity(intents.Namespace)
				res = append(res, service.GetFormattedOtterizeIdentityWithKind())
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
		otterizev2alpha1.EndpointsPodNamesIndexField,
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
