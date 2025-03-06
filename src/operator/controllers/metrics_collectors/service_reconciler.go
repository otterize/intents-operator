package metrics_collectors

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch
//+kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;update;patch;list;watch;delete;create

type ServiceReconciler struct {
	client.Client
	netpolHandle *NetworkPolicyHandler
	injectablerecorder.InjectableRecorder
}

func NewServiceReconciler(client client.Client, netpolHandle *NetworkPolicyHandler) *ServiceReconciler {
	return &ServiceReconciler{
		Client:       client,
		netpolHandle: netpolHandle,
	}
}

func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	recorder := mgr.GetEventRecorderFor("intents-operator")
	r.InjectRecorder(recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}

func (r *ServiceReconciler) InjectRecorder(recorder record.EventRecorder) {
	r.Recorder = recorder
	r.netpolHandle.InjectRecorder(recorder)
}

func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	err := r.netpolHandle.HandleAllPodsInNamespace(ctx, req.Namespace)

	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	return ctrl.Result{}, nil
}
