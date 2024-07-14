package builders

import (
	"context"
	"fmt"
	"github.com/amit7itz/goset"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type IngressNetpolBuilder struct {
	injectablerecorder.InjectableRecorder
}

func NewIngressNetpolBuilder() *IngressNetpolBuilder {
	return &IngressNetpolBuilder{}
}

// a function that builds ingress rules from serviceEffectivePolicy
func (r *IngressNetpolBuilder) buildIngressRulesFromServiceEffectivePolicy(ep effectivepolicy.ServiceEffectivePolicy) []v1.NetworkPolicyIngressRule {
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
		fromNamespaces.Add(call.Service.Namespace)
		ingressRules = append(ingressRules, v1.NetworkPolicyIngressRule{
			From: []v1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							fmt.Sprintf(
								otterizev2alpha1.OtterizeAccessLabelKey, ep.Service.GetFormattedOtterizeIdentityWithKind()): "true",
						},
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							otterizev2alpha1.KubernetesStandardNamespaceNameLabelKey: call.Service.Namespace,
						},
					},
				},
			},
		})
	}
	return ingressRules
}

func (r *IngressNetpolBuilder) Build(_ context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyIngressRule, error) {
	return r.buildIngressRulesFromServiceEffectivePolicy(ep), nil
}
