package protected_service_reconcilers

import (
	"context"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PolicyCleanerReconciler reconciles a ProtectedService object
type PolicyCleanerReconciler struct {
	client.Client
	injectablerecorder.InjectableRecorder
	extNetpolHandler ExternalNepolHandler
}

func NewPolicyCleanerReconciler(client client.Client, extNetpolHandler ExternalNepolHandler) *PolicyCleanerReconciler {
	return &PolicyCleanerReconciler{
		Client:           client,
		extNetpolHandler: extNetpolHandler,
	}
}

func (r *PolicyCleanerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	selector, err := intents_reconcilers.MatchAccessNetworkPolicy()
	if err != nil {
		return ctrl.Result{}, err
	}

	namespace := req.Namespace
	policies := &v1.NetworkPolicyList{}
	err = r.List(ctx, policies, &client.ListOptions{Namespace: namespace, LabelSelector: selector})
	if err != nil {
		return ctrl.Result{}, err
	}

	if len(policies.Items) == 0 {
		return ctrl.Result{}, nil
	}

	var protectedServicesResources otterizev1alpha2.ProtectedServiceList
	err = r.List(ctx, &protectedServicesResources, &client.ListOptions{Namespace: namespace})
	if err != nil {
		return ctrl.Result{}, err
	}

	protectedServersByNamespace := sets.Set[string]{}
	for _, protectedService := range protectedServicesResources.Items {
		serverName := otterizev1alpha2.GetFormattedOtterizeIdentity(protectedService.Spec.Name, namespace)
		protectedServersByNamespace.Insert(serverName)
	}

	for _, networkPolicy := range policies.Items {
		serverName := networkPolicy.Labels[otterizev1alpha2.OtterizeNetworkPolicy]
		if !protectedServersByNamespace.Has(serverName) {
			err = r.removeNetworkPolicy(ctx, networkPolicy)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *PolicyCleanerReconciler) removeNetworkPolicy(ctx context.Context, networkPolicy v1.NetworkPolicy) error {
	err := r.extNetpolHandler.HandleBeforeAccessPolicyRemoval(ctx, &networkPolicy)
	if err != nil {
		return err
	}
	err = r.Delete(ctx, &networkPolicy)
	if err != nil {
		return err
	}
	return nil
}
