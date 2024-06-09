package builders

import (
	"context"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// The DNSEgressNetworkPolicyBuilder creates network policies that allow egress traffic from pods.
type DNSEgressNetworkPolicyBuilder struct {
	injectablerecorder.InjectableRecorder
}

func NewDNSEgressNetworkPolicyBuilder() *DNSEgressNetworkPolicyBuilder {
	return &DNSEgressNetworkPolicyBuilder{}
}

// a function that creates []NetworkPolicyEgressRule from ep
func (r *DNSEgressNetworkPolicyBuilder) buildNetworkPolicyEgressRules(ep effectivepolicy.ServiceEffectivePolicy) []v1.NetworkPolicyEgressRule {
	if lo.NoneBy(ep.Calls, func(call otterizev2alpha1.Target) bool {
		return call.IsTargetInCluster()
	}) {
		return make([]v1.NetworkPolicyEgressRule, 0)
	}
	egressRules := make([]v1.NetworkPolicyEgressRule, 0)

	// DNS
	egressRules = append(egressRules, v1.NetworkPolicyEgressRule{
		Ports: []v1.NetworkPolicyPort{
			{
				Protocol: lo.ToPtr(corev1.ProtocolUDP),
				Port:     lo.ToPtr(intstr.FromInt32(53)),
			},
		},
	})
	return egressRules
}

func (r *DNSEgressNetworkPolicyBuilder) Build(_ context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyEgressRule, error) {
	return r.buildNetworkPolicyEgressRules(ep), nil
}
