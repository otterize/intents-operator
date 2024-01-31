package port_egress_network_policy

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
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The PortEgressNetworkPolicyReconciler creates network policies that allow egress traffic from pods to specific ports,
// based on which Kubernetes service is specified in the intents.
type PortEgressNetworkPolicyReconciler struct {
	client.Client
	Scheme                      *runtime.Scheme
	RestrictToNamespaces        []string
	enableNetworkPolicyCreation bool
	enforcementDefaultState     bool
	injectablerecorder.InjectableRecorder
}

func NewPortEgressNetworkPolicyReconciler(
	c client.Client,
	s *runtime.Scheme,
	restrictToNamespaces []string,
	enableNetworkPolicyCreation bool,
	enforcementDefaultState bool) *PortEgressNetworkPolicyReconciler {
	return &PortEgressNetworkPolicyReconciler{
		Client:                      c,
		Scheme:                      s,
		RestrictToNamespaces:        restrictToNamespaces,
		enableNetworkPolicyCreation: enableNetworkPolicyCreation,
		enforcementDefaultState:     enforcementDefaultState,
	}
}

func (r *PortEgressNetworkPolicyReconciler) ReconcileEffectivePolicies(ctx context.Context, eps []effectivepolicy.ServiceEffectivePolicy) (int, error) {
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
	return currentPolicies.Len(), errors.Wrap(err)
}

func (r *PortEgressNetworkPolicyReconciler) applyServiceEffectivePolicy(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]types.NamespacedName, error) {
	networkPolicies := make([]types.NamespacedName, 0)
	logrus.Infof("Reconciling network policies for service %s in namespace %s", ep.Service.Name, ep.Service.Namespace)
	if len(r.RestrictToNamespaces) != 0 && !lo.Contains(r.RestrictToNamespaces, ep.Service.Namespace) {
		// Namespace is not in list of namespaces we're allowed to act in, so drop it.
		ep.ClientIntentsEventRecorder.RecordWarningEventf(consts.ReasonNamespaceNotAllowed, "ClientIntents are in namespace %s but namespace is not allowed by configuration", ep.Service.Namespace)
		return networkPolicies, nil
	}
	if !r.enforcementDefaultState {
		logrus.Infof("Enforcement is disabled globally and server is not explicitly protected, skipping network policy creation for service %s in namespace %s", ep.Service.Name, ep.Service.Namespace)
		ep.ClientIntentsEventRecorder.RecordNormalEvent(consts.ReasonEnforcementDefaultOff, "Enforcement is disabled globally, network policy creation skipped")
		return networkPolicies, nil
	}
	if !r.enableNetworkPolicyCreation {
		logrus.Infof("Network policy creation is disabled, skipping network policy creation for server %s in namespace %s", ep.Service.Name, ep.Service.Namespace)
		ep.ClientIntentsEventRecorder.RecordNormalEventf(consts.ReasonEgressNetworkPolicyCreationDisabled, "Network policy creation is disabled, creation skipped")
		return networkPolicies, nil
	}
	policy, created, err := r.handleNetworkPolicyCreation(ctx, ep)
	if err != nil {
		ep.ClientIntentsEventRecorder.RecordWarningEventf(consts.ReasonCreatingEgressNetworkPoliciesFailed, "could not create network policies: %s", err.Error())
		return nil, errors.Wrap(err)
	}
	if created {
		callsCount := len(ep.Calls)
		ep.ClientIntentsEventRecorder.RecordNormalEventf(consts.ReasonCreatedEgressNetworkPolicies, "NetworkPolicy reconcile complete, reconciled %d servers", callsCount)
		telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesCreated, len(networkPolicies))
		prometheus.IncrementNetpolCreated(len(networkPolicies))
		networkPolicies = append(networkPolicies, types.NamespacedName{Name: policy.Name, Namespace: policy.Namespace})
	}
	return networkPolicies, nil
}

func (r *PortEgressNetworkPolicyReconciler) handleNetworkPolicyCreation(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) (*v1.NetworkPolicy, bool, error) {
	existingPolicy := &v1.NetworkPolicy{}
	newPolicy, shouldCreate, err := r.buildNetworkPolicyFromEffectivePolicy(ctx, ep)
	if err != nil {
		return nil, false, errors.Wrap(err)
	}
	if !shouldCreate {
		return nil, false, nil
	}
	err = r.Get(ctx, types.NamespacedName{
		Name:      newPolicy.Name,
		Namespace: newPolicy.Namespace},
		existingPolicy)
	if err != nil && !k8serrors.IsNotFound(err) {
		r.RecordWarningEventf(existingPolicy, consts.ReasonGettingEgressNetworkPolicyFailed, "failed to get network policy: %s", err.Error())
		return nil, false, errors.Wrap(err)
	}

	if k8serrors.IsNotFound(err) {
		logrus.Infof(
			"Creating network policy to enable access from service %s namespace: %s", ep.Service.Name, ep.Service.Namespace)
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

func (r *PortEgressNetworkPolicyReconciler) UpdateExistingPolicy(ctx context.Context, existingPolicy *v1.NetworkPolicy, newPolicy *v1.NetworkPolicy) error {
	if reflect.DeepEqual(existingPolicy.Spec, newPolicy.Spec) {
		return nil
	}

	policyCopy := existingPolicy.DeepCopy()
	policyCopy.Labels = newPolicy.Labels
	policyCopy.Annotations = newPolicy.Annotations
	policyCopy.Spec = newPolicy.Spec
	return errors.Wrap(r.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy)))

}

func (r *PortEgressNetworkPolicyReconciler) removeNetworkPoliciesThatShouldNotExist(ctx context.Context, netpolNamesThatShouldExist *goset.Set[types.NamespacedName]) error {
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
	deleted := 0
	for _, networkPolicy := range networkPolicyList.Items {
		namespacedName := types.NamespacedName{Namespace: networkPolicy.Namespace, Name: networkPolicy.Name}
		if !netpolNamesThatShouldExist.Contains(namespacedName) {
			serverName := networkPolicy.Labels[otterizev1alpha3.OtterizeSvcEgressNetworkPolicy]
			logrus.Infof("Removing orphaned network policy: %s server %s ns %s", networkPolicy.Name, serverName, networkPolicy.Namespace)
			err = r.removeNetworkPolicy(ctx, networkPolicy)
			if err != nil {
				return errors.Wrap(err)
			}
			deleted += 1
		}
	}

	if deleted > 0 {
		telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesDeleted, deleted)
		prometheus.IncrementNetpolCreated(deleted)
	}

	return nil
}

func (r *PortEgressNetworkPolicyReconciler) removeNetworkPolicy(ctx context.Context, networkPolicy v1.NetworkPolicy) error {
	return r.Delete(ctx, &networkPolicy)
}

func matchAccessNetworkPolicy() (labels.Selector, error) {
	isOtterizeNetworkPolicy := metav1.LabelSelectorRequirement{
		Key:      otterizev1alpha3.OtterizeSvcEgressNetworkPolicy,
		Operator: metav1.LabelSelectorOpExists,
	}
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		isOtterizeNetworkPolicy,
	}})
}

func (r *PortEgressNetworkPolicyReconciler) buildEgressRulesFromEffectivePolicy(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyEgressRule, error) {
	egressRules := make([]v1.NetworkPolicyEgressRule, 0)
	for _, intent := range ep.Calls {
		if intent.Type != "" && intent.Type != otterizev1alpha3.IntentTypeHTTP && intent.Type != otterizev1alpha3.IntentTypeKafka {
			continue
		}
		if !intent.IsTargetServerKubernetesService() {
			continue
		}
		svc := corev1.Service{}
		err := r.Get(ctx, types.NamespacedName{Name: intent.GetTargetServerName(), Namespace: intent.GetTargetServerNamespace(ep.Service.Namespace)}, &svc)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				continue
			}
			return nil, errors.Wrap(err)
		}
		var egressRule v1.NetworkPolicyEgressRule
		if svc.Spec.Selector != nil {
			egressRule = getEgressRuleBasedOnServicePodSelector(&svc)
		} else if intent.IsTargetTheKubernetesAPIServer(ep.Service.Namespace) {
			egressRule, err = r.getIPRuleFromEndpoint(ctx, &svc)
			if err != nil {
				return nil, errors.Wrap(err)
			}
		} else {
			return nil, fmt.Errorf("service %s/%s has no selector", svc.Namespace, svc.Name)
		}
		egressRules = append(egressRules, egressRule)
	}
	return egressRules, nil
}

// a function that builds networkPolicy from the effective policy. returns netpol, created, error
func (r *PortEgressNetworkPolicyReconciler) buildNetworkPolicyFromEffectivePolicy(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) (*v1.NetworkPolicy, bool, error) {
	clientPodSelector := r.buildPodLabelSelectorFromServiceEffectivePolicy(ep)
	egressRules, err := r.buildEgressRulesFromEffectivePolicy(ctx, ep)
	if err != nil {
		return nil, false, errors.Wrap(err)
	}
	if len(egressRules) == 0 {
		return nil, false, nil
	}

	return &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(otterizev1alpha3.OtterizeSvcEgressNetworkPolicyNameTemplate, ep.Service.Name),
			Namespace: ep.Service.Namespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeSvcEgressNetworkPolicy: ep.Service.GetFormattedOtterizeIdentity(),
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			PodSelector: clientPodSelector,
			Egress:      egressRules,
		},
	}, true, nil
}

// getEgressRuleBasedOnServicePodSelector returns a network policy egress rule that allows traffic to pods selected by the service
func getEgressRuleBasedOnServicePodSelector(svc *corev1.Service) v1.NetworkPolicyEgressRule {
	svcPodSelector := metav1.LabelSelector{MatchLabels: svc.Spec.Selector}
	podSelectorEgressRule := v1.NetworkPolicyEgressRule{
		To: []v1.NetworkPolicyPeer{
			{
				PodSelector: &svcPodSelector,
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						otterizev1alpha3.KubernetesStandardNamespaceNameLabelKey: svc.Namespace,
					},
				},
			},
		},
	}

	portToProtocol := make(map[int]corev1.Protocol)
	// Gather all target ports (target ports in the pod the service proxies to)
	for _, port := range svc.Spec.Ports {
		if port.TargetPort.StrVal != "" {
			continue
		}
		portToProtocol[port.TargetPort.IntValue()] = port.Protocol
	}

	networkPolicyPorts := make([]v1.NetworkPolicyPort, 0)
	// Create a list of network policy ports
	for port, protocol := range portToProtocol {
		netpolPort := v1.NetworkPolicyPort{
			Port: &intstr.IntOrString{IntVal: int32(port)},
		}
		if len(protocol) != 0 {
			netpolPort.Protocol = lo.ToPtr(protocol)
		}
		networkPolicyPorts = append(networkPolicyPorts, netpolPort)
	}

	podSelectorEgressRule.Ports = networkPolicyPorts

	return podSelectorEgressRule
}

func (r *PortEgressNetworkPolicyReconciler) getIPRuleFromEndpoint(ctx context.Context, svc *corev1.Service) (v1.NetworkPolicyEgressRule, error) {
	ipAddresses := make([]string, 0)
	ports := make([]v1.NetworkPolicyPort, 0)

	var endpoint corev1.Endpoints
	err := r.Client.Get(ctx, types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, &endpoint)
	if err != nil {
		return v1.NetworkPolicyEgressRule{}, errors.Wrap(err)
	}

	if len(endpoint.Subsets) == 0 {
		return v1.NetworkPolicyEgressRule{}, fmt.Errorf("no endpoints found for service %s/%s", svc.Namespace, svc.Name)
	}

	for _, subset := range endpoint.Subsets {
		for _, address := range subset.Addresses {
			ipAddresses = append(ipAddresses, address.IP)
		}
		for _, port := range subset.Ports {
			ports = append(ports, v1.NetworkPolicyPort{
				Port:     &intstr.IntOrString{IntVal: port.Port},
				Protocol: lo.ToPtr(port.Protocol),
			})
		}
	}

	if len(ipAddresses) == 0 {
		return v1.NetworkPolicyEgressRule{}, fmt.Errorf("no endpoints found for service %s/%s", svc.Namespace, svc.Name)
	}

	podSelectorEgressRule := v1.NetworkPolicyEgressRule{}
	for _, ip := range ipAddresses {
		podSelectorEgressRule.To = append(podSelectorEgressRule.To, v1.NetworkPolicyPeer{
			IPBlock: &v1.IPBlock{
				CIDR:   fmt.Sprintf("%s/32", ip),
				Except: nil,
			},
		})
	}

	if len(ports) > 0 {
		podSelectorEgressRule.Ports = ports
	}

	return podSelectorEgressRule, nil
}

func (r *PortEgressNetworkPolicyReconciler) buildPodLabelSelectorFromServiceEffectivePolicy(ep effectivepolicy.ServiceEffectivePolicy) metav1.LabelSelector {
	return metav1.LabelSelector{
		MatchLabels: map[string]string{
			otterizev1alpha3.OtterizeClientLabelKey: ep.Service.GetFormattedOtterizeIdentity(),
		},
	}
}
