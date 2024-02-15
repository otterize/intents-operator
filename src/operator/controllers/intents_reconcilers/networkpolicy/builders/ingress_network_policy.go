package builders

import (
	"context"
	"fmt"
	"github.com/amit7itz/goset"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
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

func (r *IngressNetpolBuilder) Build(_ context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyIngressRule, error) {
	return r.buildIngressRulesFromServiceEffectivePolicy(ep), nil
}
