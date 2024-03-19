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

type IngressReconciler struct {
	client.Client
	extNetpolHandler *NetworkPolicyHandler
	injectablerecorder.InjectableRecorder
	serviceUploader ServiceUploader
}

func NewIngressReconciler(
	client client.Client,
	extNetpolHandler *NetworkPolicyHandler,
	otterizeClient operator_cloud_client.CloudClient,
) *IngressReconciler {
	serviceUploader := NewServiceUploader(client, otterizeClient)

	return &IngressReconciler{
		Client:           client,
		extNetpolHandler: extNetpolHandler,
		serviceUploader:  serviceUploader,
	}
}

func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
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
func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	err := r.extNetpolHandler.HandleAllPods(ctx)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	err = r.serviceUploader.UploadNamespaceServices(ctx, req.Namespace)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	return ctrl.Result{}, nil
}
