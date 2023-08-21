package protected_service_reconcilers

import (
	"context"
	"github.com/otterize/intents-operator/src/operator/controllers/protected_service_reconcilers/consts"
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
}

func NewPolicyCleanerReconciler(client client.Client, networkPolicyHandler NetworkPolicyHandler) *PolicyCleanerReconciler {
	return &PolicyCleanerReconciler{
		Client:               client,
		networkPolicyHandler: networkPolicyHandler,
	}
}

func (r *PolicyCleanerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	err := WithFinalizer(ctx, r.Client, req, consts.PolicyCleanerReconcilerFinalizerName, func(ctx context.Context, req ctrl.Request) error {
		namespace := req.Namespace
		return r.networkPolicyHandler.CleanPoliciesFromUnprotectedServices(ctx, namespace)
	})
	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
