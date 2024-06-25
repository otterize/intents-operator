package builders

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		targetServiceIdentity := call.ToServiceIdentity(ep.Service.Namespace)
		egressRules = append(egressRules, v1.NetworkPolicyEgressRule{
			To: []v1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							otterizev1alpha3.OtterizeServiceLabelKey: targetServiceIdentity.GetFormattedOtterizeIdentityWithoutKind(),
						},
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							otterizev1alpha3.KubernetesStandardNamespaceNameLabelKey: targetServiceIdentity.Namespace,
						},
					},
				},
			},
		})
	}
	return egressRules
}

func (r *EgressNetworkPolicyBuilder) Build(_ context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyEgressRule, error) {
	return r.buildNetworkPolicyEgressRules(ep), nil
}
