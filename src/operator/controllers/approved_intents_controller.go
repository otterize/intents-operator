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
	"github.com/otterize/intents-operator/src/operator/mirrorevents"
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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

// ApprovedIntentsReconciler reconciles a Intents object
type ApprovedIntentsReconciler struct {
	group  *reconcilergroup.Group
	client client.Client
}

func NewApprovedIntentsReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	kafkaServerStore kafkaacls.ServersStore,
	restrictToNamespaces []string,
	enforcementConfig enforcement.Config,
	operatorPodName string,
	operatorPodNamespace string,
	additionalReconcilers ...reconcilergroup.ReconcilerWithEvents,
) *ApprovedIntentsReconciler {

	serviceIdResolver := serviceidresolver.NewResolver(client)
	reconcilers := []reconcilergroup.ReconcilerWithEvents{
		intents_reconcilers.NewPodLabelReconciler(client, scheme),
		intents_reconcilers.NewKafkaACLReconciler(client, scheme, kafkaServerStore, enforcementConfig.EnableKafkaACL, kafkaacls.NewKafkaIntentsAdmin, enforcementConfig.EnforcementDefaultState, operatorPodName, operatorPodNamespace, serviceIdResolver, enforcementConfig.EnforcedNamespaces),
		intents_reconcilers.NewIstioPolicyReconciler(client, scheme, restrictToNamespaces, enforcementConfig.EnableIstioPolicy, enforcementConfig.EnforcementDefaultState, enforcementConfig.EnforcedNamespaces),
	}
	reconcilers = append(reconcilers, additionalReconcilers...)
	reconcilersGroup := reconcilergroup.NewGroup(
		"approved-intents-reconciler",
		client,
		scheme,
		&otterizev2alpha1.ApprovedClientIntents{},
		otterizev2alpha1.ClientIntentsFinalizerName,
		intentsLegacyFinalizers,
		reconcilers...,
	)
	reconcilersGroup.AddPreRemoveFinalizerHook(removeFinalizerFromCorrespondingClientIntents)

	approvedIntentsReconciler := &ApprovedIntentsReconciler{
		group:  reconcilersGroup,
		client: client,
	}

	if enforcementConfig.EnableDatabasePolicy {
		databaseReconciler := database.NewDatabaseReconciler(client, scheme)
		approvedIntentsReconciler.group.AddToGroup(databaseReconciler)
	}

	return approvedIntentsReconciler
}

func removeFinalizerFromCorrespondingClientIntents(ctx context.Context, client client.Client, resource client.Object) error {
	approvedIntents := resource.(*otterizev2alpha1.ApprovedClientIntents)
	// get corresponding client intents
	clientIntents := &otterizev2alpha1.ClientIntents{}
	err := client.Get(ctx, types.NamespacedName{Name: approvedIntents.Name, Namespace: approvedIntents.Namespace}, clientIntents)
	if err != nil && k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Wrap(err)
	}

	// check if client intents has the finalizer
	if !controllerutil.ContainsFinalizer(clientIntents, otterizev2alpha1.ClientIntentsFinalizerName) {
		return nil
	}

	// remove finalizer from client intents
	controllerutil.RemoveFinalizer(clientIntents, otterizev2alpha1.ClientIntentsFinalizerName)
	err = client.Update(ctx, clientIntents)
	if err != nil {
		return errors.Errorf("failed to remove clientIntents finalizer: %w", err)
	}
	return nil
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ApprovedIntentsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	health.UpdateLastReconcileStartTime()

	approvedClientIntents := &otterizev2alpha1.ApprovedClientIntents{}

	err := r.client.Get(ctx, req.NamespacedName, approvedClientIntents)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrap(err)
	}

	if approvedClientIntents.Status.UpToDate != false && approvedClientIntents.Status.ObservedGeneration != approvedClientIntents.Generation {
		intentsCopy := approvedClientIntents.DeepCopy()
		intentsCopy.Status.UpToDate = false
		if err := r.client.Status().Patch(ctx, intentsCopy, client.MergeFrom(approvedClientIntents)); err != nil {
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

	if approvedClientIntents.DeletionTimestamp == nil {
		intentsCopy := approvedClientIntents.DeepCopy()
		intentsCopy.Status.UpToDate = true
		intentsCopy.Status.ObservedGeneration = intentsCopy.Generation
		if err := r.client.Status().Patch(ctx, intentsCopy, client.MergeFrom(approvedClientIntents)); err != nil {
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
func (r *ApprovedIntentsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&otterizev2alpha1.ApprovedClientIntents{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Watches(&otterizev2alpha1.ProtectedService{}, handler.EnqueueRequestsFromMapFunc(r.mapProtectedServiceToClientIntents)).
		Watches(&corev1.Endpoints{}, handler.EnqueueRequestsFromMapFunc(r.watchApiServerEndpoint)).
		Watches(&otterizev2alpha1.PostgreSQLServerConfig{}, handler.EnqueueRequestsFromMapFunc(r.mapPostgresInstanceNameToDatabaseIntents)).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(r.mapPodWithAccessAnnotationToClientIntents)).
		Complete(r)
	if err != nil {
		return errors.Wrap(err)
	}

	r.group.InjectRecorder(mirrorevents.GetMirrorToClientIntentsEventRecorderFor(mgr, "intents-operator"))

	return nil
}

func (r *ApprovedIntentsReconciler) mapPodWithAccessAnnotationToClientIntents(ctx context.Context, obj client.Object) []reconcile.Request {
	pod := obj.(*corev1.Pod)
	logrus.Debugf("Enqueueing approved client intents for pod %s", pod.Name)

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

func (r *ApprovedIntentsReconciler) watchApiServerEndpoint(ctx context.Context, obj client.Object) []reconcile.Request {
	if obj.GetNamespace() != otterizev2alpha1.KubernetesAPIServerNamespace || obj.GetName() != otterizev2alpha1.KubernetesAPIServerName {
		return nil
	}

	intentsToReconcile := r.getIntentsToAPIServerService(ctx)
	return r.mapIntentsToRequests(intentsToReconcile)
}

func (r *ApprovedIntentsReconciler) getIntentsToAPIServerService(ctx context.Context) []otterizev2alpha1.ApprovedClientIntents {
	intentsToReconcile := make([]otterizev2alpha1.ApprovedClientIntents, 0)
	fullServerName := fmt.Sprintf("svc:%s.%s", otterizev2alpha1.KubernetesAPIServerName, otterizev2alpha1.KubernetesAPIServerNamespace)
	var intentsToServer otterizev2alpha1.ApprovedClientIntentsList
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

func (r *ApprovedIntentsReconciler) mapProtectedServiceToClientIntents(ctx context.Context, obj client.Object) []reconcile.Request {
	protectedService := obj.(*otterizev2alpha1.ProtectedService)
	logrus.Debugf("Enqueueing approved client intents for protected services %s", protectedService.Name)

	intentsToReconcile := r.getIntentsToService(ctx, protectedService.Spec.Name, protectedService.Namespace)
	return r.mapIntentsToRequests(intentsToReconcile)
}

func (r *ApprovedIntentsReconciler) mapPostgresInstanceNameToDatabaseIntents(_ context.Context, obj client.Object) []reconcile.Request {
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

func (r *ApprovedIntentsReconciler) getIntentsToPostgresInstance(pgServerConf *otterizev2alpha1.PostgreSQLServerConfig) []otterizev2alpha1.ApprovedClientIntents {
	intentsList := otterizev2alpha1.ApprovedClientIntentsList{}
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

func (r *ApprovedIntentsReconciler) mapIntentsToRequests(intentsToReconcile []otterizev2alpha1.ApprovedClientIntents) []reconcile.Request {
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

func (r *ApprovedIntentsReconciler) getIntentsToService(ctx context.Context, serviceName string, namespace string) []otterizev2alpha1.ApprovedClientIntents {
	intentsToReconcile := make([]otterizev2alpha1.ApprovedClientIntents, 0)
	fullServerName := fmt.Sprintf("%s.%s", serviceName, namespace)
	var intentsToServer otterizev2alpha1.ApprovedClientIntentsList
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

// InitIntentsServerIndices indexes ApprovedClientIntents by target server name
// This is used in finalizers to determine whether a network policy should be removed from the target namespace
func (r *ApprovedIntentsReconciler) InitIntentsServerIndices(mgr ctrl.Manager) error {
	err := mgr.GetCache().IndexField(
		context.Background(),
		&otterizev2alpha1.ApprovedClientIntents{},
		otterizev2alpha1.OtterizeTargetServerIndexField,
		func(object client.Object) []string {
			var res []string
			intents := object.(*otterizev2alpha1.ApprovedClientIntents)
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
		&otterizev2alpha1.ApprovedClientIntents{},
		otterizev2alpha1.OtterizeFormattedTargetServerIndexField,
		func(object client.Object) []string {
			var res []string
			intents := object.(*otterizev2alpha1.ApprovedClientIntents)
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
func (r *ApprovedIntentsReconciler) InitProtectedServiceIndexField(mgr ctrl.Manager) error {
	return protected_services.InitProtectedServiceIndexField(mgr)
}

func (r *ApprovedIntentsReconciler) InitEndpointsPodNamesIndex(mgr ctrl.Manager) error {
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
