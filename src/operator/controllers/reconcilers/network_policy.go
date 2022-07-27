package reconcilers

import (
	"context"
	otterizev1alpha1 "github.com/otterize/intents-operator/shared/api/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NetworkPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *NetworkPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	intents := &otterizev1alpha1.Intents{}
	err := r.Get(ctx, req.NamespacedName, intents)
	if k8serrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	if intents.Spec == nil {
		// No intents for namespace, move on
		return ctrl.Result{}, nil
	}

	// TODO: Generate Network policy here by intents

	return ctrl.Result{}, nil
}

func cleanupOtterizeNetworkPolicies() error {

	return nil
}
