package builders

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/samber/lo"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// The EgressNetworkPolicyBuilder creates network policies that allow egress traffic from pods.
type EgressNetworkPolicyBuilder struct {
	injectablerecorder.InjectableRecorder
}

func NewEgressNetworkPolicyBuilder() *EgressNetworkPolicyBuilder {
	return &EgressNetworkPolicyBuilder{}
}

// a function that creates []NetworkPolicyEgressRule from ep
func (r *EgressNetworkPolicyBuilder) buildNetworkPolicyEgressRules(ep effectivepolicy.ServiceEffectivePolicy) []v1.NetworkPolicyEgressRule {
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
							otterizev1alpha3.OtterizeServiceLabelKey: otterizev1alpha3.GetFormattedOtterizeIdentity(call.GetTargetServerName(), call.GetTargetServerNamespace(ep.Service.Namespace)),
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
	return egressRules
}

func (r *EgressNetworkPolicyBuilder) Build(_ context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyEgressRule, error) {
	return r.buildNetworkPolicyEgressRules(ep), nil
}
