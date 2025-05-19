package webhook_traffic

import (
	"fmt"
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
)

var ExpectedNetpol = v1.NetworkPolicy{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: TestNamespace,
		Name:      "webhook-test-webhook-access-to-test-service",
		Labels:    map[string]string{v2alpha1.OtterizeNetworkPolicyWebhooks: TestWebhookName},
	},
	Spec: v1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{
			MatchLabels: map[string]string{"Taylor": "Swift"},
		},
		Ingress: []v1.NetworkPolicyIngressRule{
			{
				Ports: []v1.NetworkPolicyPort{{
					Protocol: lo.ToPtr(corev1.ProtocolTCP),
					Port:     lo.ToPtr(intstr.IntOrString{Type: intstr.Int, IntVal: TestServicePort}),
				}},
				From: []v1.NetworkPolicyPeer{
					{
						IPBlock: &v1.IPBlock{
							CIDR: fmt.Sprintf("%s/32", TestControlPlaneIP),
						},
					},
				},
			},
		},
		PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
	},
}

type NetworkPolicyMatcher struct {
}

func NewNetworkPolicyMatcher() *NetworkPolicyMatcher {
	return &NetworkPolicyMatcher{}
}

func (m *NetworkPolicyMatcher) String() string {
	return fmt.Sprintf("%v", &ExpectedNetpol)
}

func (m *NetworkPolicyMatcher) Matches(other interface{}) bool {
	otherAsNetpol, ok := other.(*v1.NetworkPolicy)
	if !ok {
		return false
	}

	return otherAsNetpol.Namespace == TestNamespace &&
		otherAsNetpol.Name == ExpectedNetpol.Name &&
		reflect.DeepEqual(otherAsNetpol.Labels, ExpectedNetpol.Labels) &&
		reflect.DeepEqual(otherAsNetpol.Spec, ExpectedNetpol.Spec)
}
