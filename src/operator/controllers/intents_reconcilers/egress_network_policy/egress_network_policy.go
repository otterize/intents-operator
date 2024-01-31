package egress_network_policy

import (
	"context"
	goerrors "errors"
	"fmt"
	"github.com/amit7itz/goset"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/prometheus"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
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

// The EgressNetworkPolicyReconciler creates network policies that allow egress traffic from pods.
type EgressNetworkPolicyReconciler struct {
	client.Client
	Scheme                      *runtime.Scheme
	RestrictToNamespaces        []string
	enableNetworkPolicyCreation bool
	enforcementDefaultState     bool
	injectablerecorder.InjectableRecorder
}

func NewEgressNetworkPolicyReconciler(
	c client.Client,
	s *runtime.Scheme,
	restrictToNamespaces []string,
	enableNetworkPolicyCreation bool,
	enforcementDefaultState bool) *EgressNetworkPolicyReconciler {
	return &EgressNetworkPolicyReconciler{
		Client:                      c,
		Scheme:                      s,
		RestrictToNamespaces:        restrictToNamespaces,
		enableNetworkPolicyCreation: enableNetworkPolicyCreation,
		enforcementDefaultState:     enforcementDefaultState,
	}
}

func (r *EgressNetworkPolicyReconciler) applyEffectivePolicy(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]types.NamespacedName, error) {
	logrus.Infof("Reconciling network policies for service %s in namespace %s",
		ep.Service.Name, ep.Service.Namespace)
	networkPolicies := make([]types.NamespacedName, 0)

	if len(ep.Calls) == 0 {
		return networkPolicies, nil
	}
	if !r.enforcementDefaultState {
		logrus.Infof("Enforcement is disabled globally skipping egress network policy creation for service %s in namespace %s", ep.Service.Name, ep.Service.Namespace)
		ep.ClientIntentsEventRecorder.RecordNormalEventf(consts.ReasonEnforcementDefaultOff, "Enforcement is disabled globally, network policy creation skipped")
		return networkPolicies, nil
	}
	if !r.enableNetworkPolicyCreation {
		logrus.Infof("Network policy creation is disabled, skipping network policy creation for service %s in namespace %s", ep.Service.Name, ep.Service.Namespace)
		ep.ClientIntentsEventRecorder.RecordNormalEventf(consts.ReasonEgressNetworkPolicyCreationDisabled, "Network policy creation is disabled, creation skipped")
		return networkPolicies, nil
	}
	if len(r.RestrictToNamespaces) != 0 && !lo.Contains(r.RestrictToNamespaces, ep.Service.Namespace) {
		// Namespace is not in list of namespaces we're allowed to act in, so drop it.
		ep.ClientIntentsEventRecorder.RecordWarningEventf(consts.ReasonNamespaceNotAllowed, "ClientIntents are in namespace %s but namespace is not allowed by configuration", ep.Service.Namespace)
		return networkPolicies, nil
	}

	netpol, created, err := r.handleNetworkPolicyCreation(ctx, ep)
	if err != nil {
		ep.ClientIntentsEventRecorder.RecordWarningEventf(consts.ReasonCreatingEgressNetworkPoliciesFailed, "could not create network policies: %s", err.Error())
		return nil, errors.Wrap(err)
	}
	if created {
		networkPolicies = append(networkPolicies, types.NamespacedName{Name: netpol.Name, Namespace: netpol.Namespace})
		callsCount := len(ep.Calls)
		ep.ClientIntentsEventRecorder.RecordNormalEventf(consts.ReasonCreatedEgressNetworkPolicies, "NetworkPolicy reconcile complete, reconciled %d servers", callsCount)
		telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesCreated, len(networkPolicies))
		prometheus.IncrementNetpolCreated(len(networkPolicies))
	}
	return networkPolicies, nil
}

// a function that creates []NetworkPolicyEgressRule from ep
func (r *EgressNetworkPolicyReconciler) buildNetworkPolicyEgressRules(ep effectivepolicy.ServiceEffectivePolicy) []v1.NetworkPolicyEgressRule {
	egressRules := make([]v1.NetworkPolicyEgressRule, 0)
	for _, call := range ep.Calls {
		if call.Type != "" && call.Type != otterizev1alpha3.IntentTypeHTTP && call.Type != otterizev1alpha3.IntentTypeKafka {
			continue
		}
		if call.IsTargetServerKubernetesService() {
			continue
		}
		egressRules = append(egressRules, v1.NetworkPolicyEgressRule{
			To: []v1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							otterizev1alpha3.OtterizeServerLabelKey: otterizev1alpha3.GetFormattedOtterizeIdentity(call.GetTargetServerName(), call.GetTargetServerNamespace(ep.Service.Namespace)),
						},
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							otterizev1alpha3.KubernetesStandardNamespaceNameLabelKey: call.GetTargetServerNamespace(ep.Service.Namespace),
						},
					},
				},
			},
		})
	}
	return egressRules
}

func (r *EgressNetworkPolicyReconciler) ReconcileEffectivePolicies(ctx context.Context, eps []effectivepolicy.ServiceEffectivePolicy) (int, error) {
	currentPolicies := goset.NewSet[types.NamespacedName]()
	errorList := make([]error, 0)
	for _, ep := range eps {
		netpols, err := r.applyEffectivePolicy(ctx, ep)
		if err != nil {
			errorList = append(errorList, errors.Wrap(err))
			continue
		}
		currentPolicies.Add(netpols...)
	}
	if len(errorList) > 0 {
		return 0, errors.Wrap(goerrors.Join(errorList...))
	}

	err := r.removeNetworkPoliciesThatShouldNotExist(ctx, currentPolicies)
	if err != nil {
		return currentPolicies.Len(), errors.Wrap(err)
	}

	return currentPolicies.Len(), nil
}

func (r *EgressNetworkPolicyReconciler) handleNetworkPolicyCreation(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) (*v1.NetworkPolicy, bool, error) {
	existingPolicy := &v1.NetworkPolicy{}
	newPolicy, shouldCreate := r.buildNetworkPolicyObjectForEffectivePolicy(ep)
	if !shouldCreate {
		return nil, false, nil
	}
	err := r.Get(ctx, types.NamespacedName{
		Name:      newPolicy.Name,
		Namespace: newPolicy.Namespace},
		existingPolicy)
	if err != nil && !k8serrors.IsNotFound(err) {
		r.RecordWarningEventf(existingPolicy, consts.ReasonGettingEgressNetworkPolicyFailed, "failed to get network policy: %s", err.Error())
		return nil, false, errors.Wrap(err)
	}

	if k8serrors.IsNotFound(err) {
		logrus.Infof("Creating network policy to enable access from service %s", ep.Service.Name)
		err = r.Create(ctx, newPolicy)
		if err != nil {
			return nil, false, errors.Wrap(err)
		}
		return newPolicy, true, nil
	}

	err = r.UpdateExistingPolicy(ctx, existingPolicy, newPolicy)
	if err != nil {
		return nil, false, errors.Wrap(err)
	}
	return newPolicy, true, nil
}

func (r *EgressNetworkPolicyReconciler) UpdateExistingPolicy(ctx context.Context, existingPolicy *v1.NetworkPolicy, newPolicy *v1.NetworkPolicy) error {
	if !reflect.DeepEqual(existingPolicy.Spec, newPolicy.Spec) {
		policyCopy := existingPolicy.DeepCopy()
		policyCopy.Labels = newPolicy.Labels
		policyCopy.Annotations = newPolicy.Annotations
		policyCopy.Spec = newPolicy.Spec

		err := r.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy))
		if err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (r *EgressNetworkPolicyReconciler) removeNetworkPoliciesThatShouldNotExist(ctx context.Context, netpolNamesThatShouldExist *goset.Set[types.NamespacedName]) error {
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

func (r *EgressNetworkPolicyReconciler) removeNetworkPolicy(ctx context.Context, networkPolicy v1.NetworkPolicy) error {
	err := r.Delete(ctx, &networkPolicy)
	if err != nil && k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Wrap(err)
}

func matchAccessNetworkPolicy() (labels.Selector, error) {
	isOtterizeNetworkPolicy := metav1.LabelSelectorRequirement{
		Key:      otterizev1alpha3.OtterizeEgressNetworkPolicy,
		Operator: metav1.LabelSelectorOpExists,
	}
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		isOtterizeNetworkPolicy,
	}})
}

// buildNetworkPolicyObjectForEffectivePolicy builds the network policy that represents the effective policy from the parameter
func (r *EgressNetworkPolicyReconciler) buildNetworkPolicyObjectForEffectivePolicy(ep effectivepolicy.ServiceEffectivePolicy) (*v1.NetworkPolicy, bool) {
	egressRules := r.buildNetworkPolicyEgressRules(ep)
	if len(egressRules) == 0 {
		return nil, false
	}
	podSelector := r.buildPodLabelSelectorFromServiceEffectivePolicy(ep)
	return &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(otterizev1alpha3.OtterizeEgressNetworkPolicyNameTemplate, ep.Service.Name),
			Namespace: ep.Service.Namespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeEgressNetworkPolicy: otterizev1alpha3.GetFormattedOtterizeIdentity(ep.Service.Name, ep.Service.Namespace),
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			PodSelector: podSelector,
			Egress:      egressRules,
		},
	}, true
}

// A function that builds pod label selector from serviceEffectivePolicy
func (r *EgressNetworkPolicyReconciler) buildPodLabelSelectorFromServiceEffectivePolicy(ep effectivepolicy.ServiceEffectivePolicy) metav1.LabelSelector {
	// The intent's target server made of name + namespace + hash
	formattedClient := otterizev1alpha3.GetFormattedOtterizeIdentity(ep.Service.Name, ep.Service.Namespace)

	return metav1.LabelSelector{
		MatchLabels: map[string]string{
			otterizev1alpha3.OtterizeClientLabelKey: formattedClient,
		},
	}
}
