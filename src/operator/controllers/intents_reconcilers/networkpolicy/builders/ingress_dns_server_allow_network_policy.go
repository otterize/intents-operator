package builders

import (
	"context"
	"github.com/amit7itz/goset"
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
	fromNamespaces := goset.NewSet[string]()
	for _, call := range ep.CalledBy {
		if call.IntendedCall.IsTargetOutOfCluster() {
			continue
		}
		if call.IntendedCall.IsTargetServerKubernetesService() {
			continue
		}
		// We use the same from.podSelector for every namespace,
		// therefore there is no need for more than one ingress rule per namespace
		if fromNamespaces.Contains(call.Service.Namespace) {
			continue
		}

		if !strings.HasSuffix(call.IntendedCall.Name, "dns") || call.IntendedCall.GetTargetServerNamespace(call.Service.Namespace) != "kube-system" {
			continue
		}
		fromNamespaces.Add(call.Service.Namespace)
		ingressRules = append(ingressRules, v1.NetworkPolicyIngressRule{
			Ports: []v1.NetworkPolicyPort{{
				Protocol: lo.ToPtr(corev1.ProtocolUDP),
				Port:     lo.ToPtr(intstr.FromInt32(53)),
			}},
		})
	}
	return ingressRules
}

func (r *IngressDNSServerAutoAllowNetpolBuilder) Build(_ context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyIngressRule, error) {
	return r.buildIngressRulesFromServiceEffectivePolicy(ep), nil
}
