package protected_service_reconcilers

import (
	"context"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

type ManagerStarted func(ctx context.Context) bool

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

func (r *PolicyCleanerReconciler) RunInAllNamespacesOnce(managerStartedFunc ManagerStarted) {
	defaultDelayTime := viper.GetDuration(operatorconfig.RetryDelayTimeKey)
	go func() {
		for !managerStartedFunc(context.Background()) {
			time.Sleep(defaultDelayTime)
		}

		for {
			err := r.CleanAllNamespaces(context.Background())
			if err != nil {
				logrus.Errorf("Failed to clean all namespaces: %s", err.Error())
				time.Sleep(defaultDelayTime)
			} else {
				break
			}
		}
	}()
}

func (r *PolicyCleanerReconciler) CleanAllNamespaces(ctx context.Context) error {
	var namespaces corev1.NamespaceList
	err := r.List(ctx, &namespaces)
	if err != nil {
		return err
	}

	for _, namespace := range namespaces.Items {
		err = r.cleanPolicies(ctx, namespace.Name)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *PolicyCleanerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	namespace := req.Namespace
	err := r.cleanPolicies(ctx, namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *PolicyCleanerReconciler) cleanPolicies(ctx context.Context, namespace string) error {
	selector, err := intents_reconcilers.MatchAccessNetworkPolicy()
	if err != nil {
		return err
	}

	policies := &networkingv1.NetworkPolicyList{}
	err = r.List(ctx, policies, &client.ListOptions{Namespace: namespace, LabelSelector: selector})
	if err != nil {
		return err
	}

	if len(policies.Items) == 0 {
		return nil
	}

	var protectedServicesResources otterizev1alpha2.ProtectedServiceList
	err = r.List(ctx, &protectedServicesResources, &client.ListOptions{Namespace: namespace})
	if err != nil {
		return err
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
				return err
			}
		}
	}

	return nil
}

func (r *PolicyCleanerReconciler) removeNetworkPolicy(ctx context.Context, networkPolicy networkingv1.NetworkPolicy) error {
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
