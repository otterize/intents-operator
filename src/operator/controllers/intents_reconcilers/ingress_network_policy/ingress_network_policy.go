package ingress_network_policy

import (
	"context"
	goerrors "errors"
	"fmt"
	"github.com/amit7itz/goset"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/protected_services"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/prometheus"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/allowexternaltraffic"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetrysender"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type externalNetpolHandler interface {
	HandlePodsByLabelSelector(ctx context.Context, namespace string, labelSelector labels.Selector) error
	HandleBeforeAccessPolicyRemoval(ctx context.Context, accessPolicy *v1.NetworkPolicy) error
}

type IngressNetpolEffectivePolicyReconciler struct {
	client.Client
	Scheme                      *runtime.Scheme
	extNetpolHandler            externalNetpolHandler
	RestrictToNamespaces        []string
	enableNetworkPolicyCreation bool
	enforcementDefaultState     bool
	allowExternalTraffic        allowexternaltraffic.Enum
	injectablerecorder.InjectableRecorder
}

func NewIngressNetpolEffectivePolicyReconciler(
	c client.Client,
	s *runtime.Scheme,
	extNetpolHandler externalNetpolHandler,
	restrictToNamespaces []string,
	enableNetworkPolicyCreation bool,
	enforcementDefaultState bool,
	allowExternalTraffic allowexternaltraffic.Enum,
) *IngressNetpolEffectivePolicyReconciler {
	return &IngressNetpolEffectivePolicyReconciler{
		Client:                      c,
		Scheme:                      s,
		extNetpolHandler:            extNetpolHandler,
		RestrictToNamespaces:        restrictToNamespaces,
		enableNetworkPolicyCreation: enableNetworkPolicyCreation,
		enforcementDefaultState:     enforcementDefaultState,
		allowExternalTraffic:        allowExternalTraffic,
	}
}

// ReconcileEffectivePolicies Gets current state of effective policies and returns number of network policies
func (r *IngressNetpolEffectivePolicyReconciler) ReconcileEffectivePolicies(ctx context.Context, eps []effectivepolicy.ServiceEffectivePolicy) (int, error) {
	currentPolicies := goset.NewSet[types.NamespacedName]()
	errorList := make([]error, 0)
	for _, ep := range eps {
		netpols, err := r.applyServiceEffectivePolicy(ctx, ep)
		if err != nil {
			errorList = append(errorList, errors.Wrap(err))
			continue
		}
		currentPolicies.Add(netpols...)
	}
	if len(errorList) > 0 {
		return 0, errors.Wrap(goerrors.Join(errorList...))
	}

	// remove policies that doesn't exist in the policy list
	err := r.removeNetworkPoliciesThatShouldNotExist(ctx, currentPolicies)
	if err != nil {
		return currentPolicies.Len(), errors.Wrap(err)
	}

	if currentPolicies.Len() != 0 {
		telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesCreated, currentPolicies.Len())
	}
	return currentPolicies.Len(), nil
}

// applyServiceEffectivePolicy - reconcile ingress netpols for a service. returns the list of policies' namespaced names
func (r *IngressNetpolEffectivePolicyReconciler) applyServiceEffectivePolicy(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]types.NamespacedName, error) {
	netpol, shouldCreate, err := r.buildNetworkPolicyFromServiceEffectivePolicy(ctx, ep)
	if err != nil || !shouldCreate {
		return make([]types.NamespacedName, 0), errors.Wrap(err)
	}
	existingPolicy := &v1.NetworkPolicy{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      netpol.Name,
		Namespace: ep.Service.Namespace},
		existingPolicy)
	if err != nil && !k8serrors.IsNotFound(err) {
		r.RecordWarningEventf(existingPolicy, consts.ReasonGettingNetworkPolicyFailed, "failed to get network policy: %s", err.Error())
		return make([]types.NamespacedName, 0), errors.Wrap(err)
	}
	if k8serrors.IsNotFound(err) {
		err = r.createNetworkPolicy(ctx, ep.Service.Name, netpol)
		if err != nil {
			return make([]types.NamespacedName, 0), errors.Wrap(err)
		}
		prometheus.IncrementNetpolCreated(1)
		ep.RecordOnClientsNormalEventf(consts.ReasonCreatedNetworkPolicies, "NetworkPolicy created for %s", ep.Service.Name)
		return []types.NamespacedName{{Name: netpol.Name, Namespace: netpol.Namespace}}, nil
	}
	changed, err := r.updateExistingPolicy(ctx, existingPolicy, netpol)
	if err != nil {
		return make([]types.NamespacedName, 0), errors.Wrap(err)
	}
	if changed {
		ep.RecordOnClientsNormalEventf(consts.ReasonCreatedNetworkPolicies, "NetworkPolicy updated for %s", ep.Service.Name)
	}
	return []types.NamespacedName{{Name: netpol.Name, Namespace: netpol.Namespace}}, nil

}

// A function that builds network policy a v1.NetworkPolicy from serviceEffectivePolicy,
// gets a ctx and returns also a bool represents if there were ingress rules and an error
func (r *IngressNetpolEffectivePolicyReconciler) buildNetworkPolicyFromServiceEffectivePolicy(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) (*v1.NetworkPolicy, bool, error) {
	shouldCreate, err := r.shouldCreatePolicy(ctx, ep)
	if err != nil || !shouldCreate {
		return nil, false, errors.Wrap(err)
	}
	ingressRules := r.buildIngressRulesFromServiceEffectivePolicy(ep)
	if len(ingressRules) == 0 {
		return nil, false, nil
	}
	podSelector := r.buildPodLabelSelectorFromServiceEffectivePolicy(ep)
	return &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(otterizev1alpha3.OtterizeNetworkPolicyNameTemplate, ep.Service.Name),
			Namespace: ep.Service.Namespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeNetworkPolicy: ep.Service.GetFormattedOtterizeIdentity(),
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: podSelector,
			Ingress:     ingressRules,
		},
	}, true, nil
}

// A function that builds pod label selector from serviceEffectivePolicy
func (r *IngressNetpolEffectivePolicyReconciler) buildPodLabelSelectorFromServiceEffectivePolicy(ep effectivepolicy.ServiceEffectivePolicy) metav1.LabelSelector {
	return metav1.LabelSelector{
		MatchLabels: map[string]string{
			otterizev1alpha3.OtterizeServerLabelKey: ep.Service.GetFormattedOtterizeIdentity(),
		},
	}
}

func (r *IngressNetpolEffectivePolicyReconciler) shouldCreatePolicy(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) (bool, error) {
	shouldCreatePolicy, err := protected_services.IsServerEnforcementEnabledDueToProtectionOrDefaultState(ctx, r.Client, ep.Service.Name, ep.Service.Namespace, r.enforcementDefaultState)
	if err != nil {
		return false, errors.Wrap(err)
	}
	if !shouldCreatePolicy {
		logrus.Infof("Enforcement is disabled globally and server is not explicitly protected, skipping network policy creation for server %s in namespace %s", ep.Service.Name, ep.Service.Namespace)
		ep.RecordOnClientsNormalEventf(consts.ReasonEnforcementDefaultOff, "Enforcement is disabled globally and called service '%s' is not explicitly protected using a ProtectedService resource, network policy creation skipped", ep.Service.Name)
		return false, nil
	}
	if !r.enableNetworkPolicyCreation {
		logrus.Infof("Network policy creation is disabled, skipping network policy creation for server %s in namespace %s", ep.Service.Name, ep.Service.Namespace)
		ep.RecordOnClientsNormalEvent(consts.ReasonNetworkPolicyCreationDisabled, "Network policy creation is disabled, creation skipped")
		return false, nil
	}
	if len(r.RestrictToNamespaces) != 0 && !lo.Contains(r.RestrictToNamespaces, ep.Service.Namespace) {
		// Namespace is not in list of namespaces we're allowed to act in, so drop it.
		ep.RecordOnClientsWarningEventf(consts.ReasonNamespaceNotAllowed, "namespace %s was specified in intent, but is not allowed by configuration", ep.Service.Namespace)
		return false, nil
	}
	logrus.Debugf("Server %s in namespace %s is in protected list: %t", ep.Service.Name, ep.Service.Namespace, shouldCreatePolicy)
	return true, nil
}

// a function that builds ingress rules from serviceEffectivePolicy
func (r *IngressNetpolEffectivePolicyReconciler) buildIngressRulesFromServiceEffectivePolicy(ep effectivepolicy.ServiceEffectivePolicy) []v1.NetworkPolicyIngressRule {
	ingressRules := make([]v1.NetworkPolicyIngressRule, 0)
	fromNamespaces := goset.NewSet[string]()
	for _, call := range ep.CalledBy {
		if call.IntendedCall.IsTargetOutOfCluster() {
			continue
		}
		if fromNamespaces.Contains(call.Service.Namespace) {
			continue
		}
		// We should add only one ingress rule per namespace
		fromNamespaces.Add(call.Service.Namespace)
		ingressRules = append(ingressRules, v1.NetworkPolicyIngressRule{
			From: []v1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							fmt.Sprintf(
								otterizev1alpha3.OtterizeAccessLabelKey, ep.Service.GetFormattedOtterizeIdentity()): "true",
						},
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							otterizev1alpha3.KubernetesStandardNamespaceNameLabelKey: call.Service.Namespace,
						},
					},
				},
			},
		})
	}
	return ingressRules
}

func (r *IngressNetpolEffectivePolicyReconciler) updateExistingPolicy(ctx context.Context, existingPolicy *v1.NetworkPolicy, newPolicy *v1.NetworkPolicy) (bool, error) {
	if !reflect.DeepEqual(existingPolicy.Spec, newPolicy.Spec) {
		policyCopy := existingPolicy.DeepCopy()
		policyCopy.Labels = newPolicy.Labels
		policyCopy.Annotations = newPolicy.Annotations
		policyCopy.Spec = newPolicy.Spec

		err := r.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy))
		if err != nil {
			return true, errors.Wrap(err)
		}
		return true, nil
	}

	return false, nil
}

func (r *IngressNetpolEffectivePolicyReconciler) createNetworkPolicy(ctx context.Context, serviceName string, newPolicy *v1.NetworkPolicy) error {
	logrus.Infof("Creating network policy to enable access to %s", serviceName)
	err := r.Create(ctx, newPolicy)
	if err != nil {
		return errors.Wrap(err)
	}

	return r.reconcileEndpointsForPolicy(ctx, newPolicy)
}

func (r *IngressNetpolEffectivePolicyReconciler) reconcileEndpointsForPolicy(ctx context.Context, newPolicy *v1.NetworkPolicy) error {
	selector, err := metav1.LabelSelectorAsSelector(&newPolicy.Spec.PodSelector)
	if err != nil {
		return errors.Wrap(err)
	}
	// Use the external netpolHandler to check if pods got affected and if so, if they need external allow policies
	return r.extNetpolHandler.HandlePodsByLabelSelector(ctx, newPolicy.Namespace, selector)
}

func (r *IngressNetpolEffectivePolicyReconciler) removeNetworkPoliciesThatShouldNotExist(ctx context.Context, netpolNamesThatShouldExist *goset.Set[types.NamespacedName]) error {
	logrus.Info("Searching for orphaned network policies")
	networkPolicyList := &v1.NetworkPolicyList{}
	selector, err := matchAccessNetworkPolicy()
	if err != nil {
		return errors.Wrap(err)
	}

	err = r.List(ctx, networkPolicyList, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		logrus.Infof("Error listing network policies: %s", err.Error())
		return errors.Wrap(err)
	}

	logrus.Infof("Selector: %s found %d network policies", selector.String(), len(networkPolicyList.Items))
	for _, networkPolicy := range networkPolicyList.Items {
		namespacedName := types.NamespacedName{Namespace: networkPolicy.Namespace, Name: networkPolicy.Name}
		if !netpolNamesThatShouldExist.Contains(namespacedName) {
			serverName := networkPolicy.Labels[otterizev1alpha3.OtterizeNetworkPolicy]
			logrus.Infof("Removing orphaned network policy: %s server %s ns %s", networkPolicy.Name, serverName, networkPolicy.Namespace)
			err = r.removeNetworkPolicy(ctx, networkPolicy)
			if err != nil {
				return errors.Wrap(err)
			}
		}
	}

	deletedCount := len(networkPolicyList.Items) - netpolNamesThatShouldExist.Len()
	if deletedCount > 0 {
		telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesDeleted, deletedCount)
		prometheus.IncrementNetpolDeleted(deletedCount)
	}

	return nil
}

func (r *IngressNetpolEffectivePolicyReconciler) removeNetworkPolicy(ctx context.Context, networkPolicy v1.NetworkPolicy) error {
	err := r.extNetpolHandler.HandleBeforeAccessPolicyRemoval(ctx, &networkPolicy)
	if err != nil {
		return errors.Wrap(err)
	}
	err = r.Delete(ctx, &networkPolicy)
	if err != nil {
		return errors.Wrap(err)
	}
	return nil
}

func matchAccessNetworkPolicy() (labels.Selector, error) {
	isOtterizeNetworkPolicy := metav1.LabelSelectorRequirement{
		Key:      otterizev1alpha3.OtterizeNetworkPolicy,
		Operator: metav1.LabelSelectorOpExists,
	}
	isNotExternalTrafficPolicy := metav1.LabelSelectorRequirement{
		Key:      otterizev1alpha3.OtterizeNetworkPolicyExternalTraffic,
		Operator: metav1.LabelSelectorOpDoesNotExist,
	}
	isNotDefaultDenyPolicy := metav1.LabelSelectorRequirement{
		Key:      otterizev1alpha3.OtterizeNetworkPolicyServiceDefaultDeny,
		Operator: metav1.LabelSelectorOpDoesNotExist,
	}
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		isOtterizeNetworkPolicy,
		isNotExternalTrafficPolicy,
		isNotDefaultDenyPolicy,
	}})
}
