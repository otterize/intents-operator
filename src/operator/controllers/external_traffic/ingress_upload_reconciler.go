package external_traffic

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/samber/lo"
	v1 "k8s.io/api/networking/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

//+kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=get;list;watch
//+kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;update;patch;list;watch;delete;create

type IngressUploadReconciler struct {
	client.Client
	injectablerecorder.InjectableRecorder
	serviceUploader ServiceUploader
}

func NewIngressUploadReconciler(
	client client.Client,
	otterizeClient operator_cloud_client.CloudClient,
) *IngressUploadReconciler {
	serviceUploader := NewServiceUploader(client, otterizeClient)

	return &IngressUploadReconciler{
		Client:          client,
		serviceUploader: serviceUploader,
	}
}

func (r *IngressUploadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	recorder := mgr.GetEventRecorderFor("intents-operator")
	r.InjectRecorder(recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Ingress{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}

// Reconcile handles ingress creation, update and delete. In all of these cases, it resolves which services
// the ingress refers to, and sends a Reconcile request to the EndpointsReconciler.
// The EndpointsReconciler is responsible for determining which services and ingresses are related to an Endpoints resource
// and managing the network policies accordingly.
func (r *IngressUploadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	err := r.serviceUploader.UploadNamespaceServices(ctx, req.Namespace)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	return ctrl.Result{}, nil
}
