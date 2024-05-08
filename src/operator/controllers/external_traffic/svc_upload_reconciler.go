package external_traffic

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
//+kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;update;patch;list;watch;delete;create

type ServiceUploadReconciler struct {
	client.Client
	injectablerecorder.InjectableRecorder
	serviceUploader ServiceUploader
}

func NewServiceUploadReconciler(client client.Client, otterizeClient operator_cloud_client.CloudClient) *ServiceUploadReconciler {
	serviceUploader := NewServiceUploader(client, otterizeClient)
	return &ServiceUploadReconciler{
		Client:          client,
		serviceUploader: serviceUploader,
	}
}

func (r *ServiceUploadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	recorder := mgr.GetEventRecorderFor("intents-operator")
	r.InjectRecorder(recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Endpoints{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}

func (r *ServiceUploadReconciler) InjectRecorder(recorder record.EventRecorder) {
	r.Recorder = recorder
}

func (r *ServiceUploadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	endpoints := corev1.Endpoints{}

	err := r.Client.Get(ctx, req.NamespacedName, &endpoints)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(client.IgnoreNotFound(err))
	}

	// If no endpoints have pods, there is nothing we can do. We will get called again when pods are added.
	if lo.NoneBy(endpoints.Subsets, func(subset corev1.EndpointSubset) bool {
		return len(subset.Addresses) == 0
	}) {
		return ctrl.Result{}, nil
	}

	svc := corev1.Service{}
	err = r.Client.Get(ctx, req.NamespacedName, &svc)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(client.IgnoreNotFound(err))
	}

	err = r.serviceUploader.UploadNamespaceServices(ctx, svc)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	return ctrl.Result{}, nil
}
