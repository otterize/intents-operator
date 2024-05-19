package builders

import (
	"context"
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/samber/lo"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The PortEgressRulesBuilder creates network policies that allow egress traffic from pods to specific ports,
// based on which Kubernetes service is specified in the intents.
type PortEgressRulesBuilder struct {
	client.Client
	injectablerecorder.InjectableRecorder
}

func NewPortEgressRulesBuilder(c client.Client) *PortEgressRulesBuilder {
	return &PortEgressRulesBuilder{Client: c}
}

func (r *PortEgressRulesBuilder) buildEgressRulesFromEffectivePolicy(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyEgressRule, error) {
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
			return nil, errors.Errorf("service %s/%s has no selector", svc.Namespace, svc.Name)
		}
		egressRules = append(egressRules, egressRule)
		if viper.GetBool(operatorconfig.EnableEgressAutoallowDNSTrafficKey) {
			// DNS
			egressRules = append(egressRules, v1.NetworkPolicyEgressRule{
				Ports: []v1.NetworkPolicyPort{
					{
						Protocol: lo.ToPtr(corev1.ProtocolUDP),
						Port:     lo.ToPtr(intstr.FromInt32(53)),
					},
				},
			})
		}
	}
	return egressRules, nil
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

	// Create a list of network policy ports
	networkPolicyPorts := make([]v1.NetworkPolicyPort, 0)
	for _, port := range svc.Spec.Ports {
		netpolPort := v1.NetworkPolicyPort{
			Port: lo.ToPtr(port.TargetPort),
		}
		if len(port.Protocol) != 0 {
			netpolPort.Protocol = lo.ToPtr(port.Protocol)
		}
		networkPolicyPorts = append(networkPolicyPorts, netpolPort)
	}

	podSelectorEgressRule.Ports = networkPolicyPorts

	return podSelectorEgressRule
}

func (r *PortEgressRulesBuilder) getIPRuleFromEndpoint(ctx context.Context, svc *corev1.Service) (v1.NetworkPolicyEgressRule, error) {
	ipAddresses := make([]string, 0)
	ports := make([]v1.NetworkPolicyPort, 0)

	ipAddresses = append(ipAddresses, svc.Spec.ClusterIP)
	var endpoint corev1.Endpoints
	err := r.Client.Get(ctx, types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, &endpoint)
	if err != nil {
		return v1.NetworkPolicyEgressRule{}, errors.Wrap(err)
	}

	if len(endpoint.Subsets) == 0 {
		return v1.NetworkPolicyEgressRule{}, errors.Errorf("no endpoints found for service %s/%s", svc.Namespace, svc.Name)
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
		return v1.NetworkPolicyEgressRule{}, errors.Errorf("no endpoints found for service %s/%s", svc.Namespace, svc.Name)
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

func (r *PortEgressRulesBuilder) Build(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyEgressRule, error) {
	return r.buildEgressRulesFromEffectivePolicy(ctx, ep)
}
