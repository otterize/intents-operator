package builders

import (
	"context"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"strings"
)

type IngressDNSServerAutoAllowNetpolBuilder struct {
	injectablerecorder.InjectableRecorder
}

func NewIngressDNSServerAutoAllowNetpolBuilder() *IngressDNSServerAutoAllowNetpolBuilder {
	return &IngressDNSServerAutoAllowNetpolBuilder{}
}

// Add UDP allow port 53 if the target is a DNS server in kube-system. This covers 'coredns' and 'kube-dns'.
func (r *IngressDNSServerAutoAllowNetpolBuilder) buildIngressRulesFromServiceEffectivePolicy(ep effectivepolicy.ServiceEffectivePolicy) []v1.NetworkPolicyIngressRule {
	ingressRules := make([]v1.NetworkPolicyIngressRule, 0)

	// If the service is not called by any other service, or it's not a DNS server, skip.
	if len(ep.CalledBy) == 0 ||
		!strings.HasSuffix(ep.Service.Name, "dns") || ep.Service.Namespace != "kube-system" {
		return ingressRules
	}
	ingressRules = append(ingressRules, v1.NetworkPolicyIngressRule{
		Ports: []v1.NetworkPolicyPort{{
			Protocol: lo.ToPtr(corev1.ProtocolUDP),
			Port:     lo.ToPtr(intstr.FromInt32(53)),
		}},
	})
	return ingressRules
}

func (r *IngressDNSServerAutoAllowNetpolBuilder) Build(_ context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyIngressRule, error) {
	return r.buildIngressRulesFromServiceEffectivePolicy(ep), nil
}
