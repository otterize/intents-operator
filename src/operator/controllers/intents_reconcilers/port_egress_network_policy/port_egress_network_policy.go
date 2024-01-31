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
	logrus.Infof("Reconciling network policies for service %s in namespace %s",
		ep.Service.Name, ep.Service.Namespace)

	networkPolicies := make([]types.NamespacedName, 0)
	for _, intent := range ep.Calls {
		if intent.Type != "" && intent.Type != otterizev1alpha3.IntentTypeHTTP && intent.Type != otterizev1alpha3.IntentTypeKafka {
			continue
		}
		if !intent.IsTargetServerKubernetesService() {
			continue
		}

		if len(r.RestrictToNamespaces) != 0 && !lo.Contains(r.RestrictToNamespaces, ep.Service.Namespace) {
			// Namespace is not in list of namespaces we're allowed to act in, so drop it.
			ep.ClientIntentsEventRecorder.RecordWarningEventf(consts.ReasonNamespaceNotAllowed, "ClientIntents are in namespace %s but namespace is not allowed by configuration", ep.Service.Namespace)
			continue
		}
		if !r.enforcementDefaultState {
			logrus.Infof("Enforcement is disabled globally and server is not explicitly protected, skipping network policy creation for server %s in namespace %s", intent.GetTargetServerName(), intent.GetTargetServerNamespace(ep.Service.Namespace))
			ep.ClientIntentsEventRecorder.RecordNormalEventf(consts.ReasonEnforcementDefaultOff, "Enforcement is disabled globally, network policy creation skipped", intent.Name)
			continue
		}
		if !r.enableNetworkPolicyCreation {
			logrus.Infof("Network policy creation is disabled, skipping network policy creation for server %s in namespace %s", intent.GetTargetServerName(), intent.GetTargetServerNamespace(ep.Service.Namespace))
			ep.ClientIntentsEventRecorder.RecordNormalEventf(consts.ReasonEgressNetworkPolicyCreationDisabled, "Network policy creation is disabled, creation skipped")
			continue
		}
		policy, created, err := r.handleNetworkPolicyCreation(ctx, ep, intent)
		if err != nil {
			ep.ClientIntentsEventRecorder.RecordWarningEventf(consts.ReasonCreatingEgressNetworkPoliciesFailed, "could not create network policies: %s", err.Error())
			return nil, errors.Wrap(err)
		}
		if created {
			networkPolicies = append(networkPolicies, types.NamespacedName{Name: policy.Name, Namespace: policy.Namespace})
		}
	}

	if len(networkPolicies) != 0 {
		callsCount := len(ep.Calls)
		ep.ClientIntentsEventRecorder.RecordNormalEventf(consts.ReasonCreatedEgressNetworkPolicies, "NetworkPolicy reconcile complete, reconciled %d servers", callsCount)
		telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesCreated, len(networkPolicies))
		prometheus.IncrementNetpolCreated(len(networkPolicies))

	}
	return networkPolicies, nil
}

func (r *PortEgressNetworkPolicyReconciler) handleNetworkPolicyCreation(
	ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy, intent otterizev1alpha3.Intent) (*v1.NetworkPolicy, bool, error) {

	existingPolicy := &v1.NetworkPolicy{}
	svc := corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: intent.GetTargetServerName(), Namespace: intent.GetTargetServerNamespace(ep.Service.Namespace)}, &svc)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return existingPolicy, false, nil
		}
		return nil, false, errors.Wrap(err)
	}
	policyName := fmt.Sprintf(otterizev1alpha3.OtterizeSvcEgressNetworkPolicyNameTemplate, intent.GetServerFullyQualifiedName(ep.Service.Namespace), ep.Service.Name)
	newPolicy, err := r.buildNetworkPolicyObjectForIntents(ctx, &svc, ep, intent, policyName)
	if err != nil {
		return nil, false, errors.Wrap(err)
	}
	err = r.Get(ctx, types.NamespacedName{
		Name:      policyName,
		Namespace: ep.Service.Namespace},
		existingPolicy)
	if err != nil && !k8serrors.IsNotFound(err) {
		r.RecordWarningEventf(existingPolicy, consts.ReasonGettingEgressNetworkPolicyFailed, "failed to get network policy: %s", err.Error())
		return nil, false, errors.Wrap(err)
	}

	if k8serrors.IsNotFound(err) {
		err = r.CreateNetworkPolicy(ctx, ep.Service.Namespace, intent, newPolicy)
		if err != nil {
			return nil, false, errors.Wrap(err)
		}
		return newPolicy, true, nil
	}

	err = r.UpdateExistingPolicy(ctx, existingPolicy, newPolicy, intent, ep.Service.Namespace)
	if err != nil {
		return nil, false, errors.Wrap(err)
	}
	return newPolicy, true, nil

}

func (r *PortEgressNetworkPolicyReconciler) UpdateExistingPolicy(ctx context.Context, existingPolicy *v1.NetworkPolicy, newPolicy *v1.NetworkPolicy, intent otterizev1alpha3.Intent, intentsObjNamespace string) error {
	if reflect.DeepEqual(existingPolicy.Spec, newPolicy.Spec) {
		return nil
	}

	policyCopy := existingPolicy.DeepCopy()
	policyCopy.Labels = newPolicy.Labels
	policyCopy.Annotations = newPolicy.Annotations
	policyCopy.Spec = newPolicy.Spec
	return errors.Wrap(r.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy)))

}

func (r *PortEgressNetworkPolicyReconciler) CreateNetworkPolicy(ctx context.Context, intentsObjNamespace string, intent otterizev1alpha3.Intent, newPolicy *v1.NetworkPolicy) error {
	logrus.Infof(
		"Creating network policy to enable access from namespace %s to %s", intentsObjNamespace, intent.Name)
	return r.Create(ctx, newPolicy)
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

// buildNetworkPolicyObjectForIntents builds the network policy that represents the intent from the parameter
func (r *PortEgressNetworkPolicyReconciler) buildNetworkPolicyObjectForIntents(ctx context.Context, svc *corev1.Service, ep effectivepolicy.ServiceEffectivePolicy, intent otterizev1alpha3.Intent, policyName string) (*v1.NetworkPolicy, error) {
	// The intent's target server made of name + namespace + hash
	formattedClient := otterizev1alpha3.GetFormattedOtterizeIdentity(ep.Service.Name, ep.Service.Namespace)
	formattedTargetServer := otterizev1alpha3.GetFormattedOtterizeIdentity(intent.GetTargetServerName(), intent.GetTargetServerNamespace(ep.Service.Namespace))
	clientPodSelector := r.buildPodLabelSelectorFromServiceEffectivePolicy(ep)
	var egressRule v1.NetworkPolicyEgressRule
	var err error
	if svc.Spec.Selector != nil {
		egressRule = getPodSelectorRule(svc, ep, intent)
	} else if intent.IsTargetTheKubernetesAPIServer(ep.Service.Namespace) {
		egressRule, err = r.getIPRuleFromEndpoint(ctx, svc)
		if err != nil {
			return nil, errors.Wrap(err)
		}
	} else {
		return nil, fmt.Errorf("service %s/%s has no selector", svc.Namespace, svc.Name)
	}

	netpol := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: ep.Service.Namespace,
			Annotations: map[string]string{
				otterizev1alpha3.OtterizeSvcEgressNetworkPolicyTargetService:          svc.Name,
				otterizev1alpha3.OtterizeSvcEgressNetworkPolicyTargetServiceNamespace: svc.Namespace,
			},
			Labels: map[string]string{
				otterizev1alpha3.OtterizeSvcEgressNetworkPolicy:       formattedClient,
				otterizev1alpha3.OtterizeSvcEgressNetworkPolicyTarget: formattedTargetServer,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			PodSelector: clientPodSelector,
			Egress:      []v1.NetworkPolicyEgressRule{egressRule},
		},
	}

	return netpol, nil
}

func getPodSelectorRule(svc *corev1.Service, ep effectivepolicy.ServiceEffectivePolicy, intent otterizev1alpha3.Intent) v1.NetworkPolicyEgressRule {
	svcPodSelector := metav1.LabelSelector{MatchLabels: svc.Spec.Selector}
	podSelectorEgressRule := v1.NetworkPolicyEgressRule{
		To: []v1.NetworkPolicyPeer{
			{
				PodSelector: &svcPodSelector,
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						otterizev1alpha3.KubernetesStandardNamespaceNameLabelKey: intent.GetTargetServerNamespace(ep.Service.Namespace),
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
	// The intent's target server made of name + namespace + hash
	formattedClient := otterizev1alpha3.GetFormattedOtterizeIdentity(ep.Service.Name, ep.Service.Namespace)

	return metav1.LabelSelector{
		MatchLabels: map[string]string{
			otterizev1alpha3.OtterizeClientLabelKey: formattedClient,
		},
	}
}
