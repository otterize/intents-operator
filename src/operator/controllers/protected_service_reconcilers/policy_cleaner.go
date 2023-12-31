package protected_service_reconcilers

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EffectivePolicySyncer interface {
	Sync(ctx context.Context) []error
}

// PolicyCleanerReconciler reconciles a ProtectedService object
type PolicyCleanerReconciler struct {
	client.Client
	injectablerecorder.InjectableRecorder
	epSyncer EffectivePolicySyncer
}

func NewPolicyCleanerReconciler(client client.Client, networkPolicyHandler EffectivePolicySyncer) *PolicyCleanerReconciler {
	return &PolicyCleanerReconciler{
		Client:   client,
		epSyncer: networkPolicyHandler,
	}
}

func (r *PolicyCleanerReconciler) Reconcile(ctx context.Context, _ ctrl.Request) (ctrl.Result, error) {
	err := r.epSyncer.Sync(ctx)
	if len(err) > 0 {
		return ctrl.Result{}, err[0]
	}

	return ctrl.Result{}, nil
}
