package protected_service_reconcilers

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EffectivePolicyReconcilerGroup interface {
	Reconcile(ctx context.Context) error
}

// PolicyCleanerReconciler reconciles a ProtectedService object
type PolicyCleanerReconciler struct {
	client.Client
	injectablerecorder.InjectableRecorder
	epReconcilerGroup EffectivePolicyReconcilerGroup
}

func NewPolicyCleanerReconciler(client client.Client, networkPolicyHandler EffectivePolicyReconcilerGroup) *PolicyCleanerReconciler {
	return &PolicyCleanerReconciler{
		Client:            client,
		epReconcilerGroup: networkPolicyHandler,
	}
}

func (r *PolicyCleanerReconciler) Reconcile(ctx context.Context, _ ctrl.Request) (ctrl.Result, error) {
	err := r.epReconcilerGroup.Reconcile(ctx)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	return ctrl.Result{}, nil
}
