package builders

import (
	"context"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
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
		if call.IsTargetOutOfCluster() {
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
							otterizev2alpha1.OtterizeServiceLabelKey: targetServiceIdentity.GetFormattedOtterizeIdentityWithoutKind(),
						},
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							otterizev2alpha1.KubernetesStandardNamespaceNameLabelKey: targetServiceIdentity.Namespace,
						},
					},
				},
			},
		})

		// Workaround to make netpols work in AWS VPC CNI which requires a rule matching the service's selector exactly in order to allow traffic to ClusterIP.
		for _, svc := range call.ReferencingKubernetesServices {
			egressRules = append(egressRules, v1.NetworkPolicyEgressRule{
				To: []v1.NetworkPolicyPeer{
					{
						PodSelector: &metav1.LabelSelector{
							MatchLabels: svc.Spec.Selector,
						},
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								otterizev2alpha1.KubernetesStandardNamespaceNameLabelKey: targetServiceIdentity.Namespace,
							},
						},
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
