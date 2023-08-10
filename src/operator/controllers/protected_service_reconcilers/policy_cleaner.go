package protected_service_reconcilers

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NetworkPolicyHandler interface {
	CleanPoliciesFromUnprotectedServices(ctx context.Context, namespace string) error
}

// PolicyCleanerReconciler reconciles a ProtectedService object
type PolicyCleanerReconciler struct {
	client.Client
	injectablerecorder.InjectableRecorder
	networkPolicyHandler NetworkPolicyHandler
	extNetpolHandler     ExternalNepolHandler
}

func NewPolicyCleanerReconciler(client client.Client, networkPolicyHandler NetworkPolicyHandler) *PolicyCleanerReconciler {
	return &PolicyCleanerReconciler{
		Client:               client,
		networkPolicyHandler: networkPolicyHandler,
	}
}

func (r *PolicyCleanerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	namespace := req.Namespace
	err := r.networkPolicyHandler.CleanPoliciesFromUnprotectedServices(ctx, namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
