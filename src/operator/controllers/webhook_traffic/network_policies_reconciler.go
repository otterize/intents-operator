package webhook_traffic

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=networkpolicies,verbs=get;list;watch

type NetworkPoliciesReconciler struct {
	client.Client
	injectablerecorder.InjectableRecorder
	handler *NetworkPolicyHandler
}

func NewNetworkPoliciesReconciler(client client.Client, handler *NetworkPolicyHandler) *NetworkPoliciesReconciler {
	return &NetworkPoliciesReconciler{
		Client:  client,
		handler: handler,
	}
}

func (r *NetworkPoliciesReconciler) SetupWithManager(mgr ctrl.Manager) error {
	recorder := mgr.GetEventRecorderFor("intents-operator")
	r.InjectRecorder(recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.NetworkPolicy{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}

func (r *NetworkPoliciesReconciler) InjectRecorder(recorder record.EventRecorder) {
	r.Recorder = recorder
}

func (r *NetworkPoliciesReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	err := r.handler.ReconcileAll(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
