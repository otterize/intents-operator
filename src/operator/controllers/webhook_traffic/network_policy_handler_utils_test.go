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
				Ports: nil,
				From:  nil,
			},
		},
		PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
	},
}

type NetworkPolicyBuilder struct {
	policy *v1.NetworkPolicy
}

type NetworkPolicyMatcher struct {
	ports                   []int32
	allowAllIncomingTraffic bool
}

func NewNetworkPolicyMatcher(ports []int32, allowAllIncomingTraffic bool) *NetworkPolicyMatcher {
	return &NetworkPolicyMatcher{
		ports:                   ports,
		allowAllIncomingTraffic: allowAllIncomingTraffic,
	}
}

func (m *NetworkPolicyMatcher) String() string {
	return fmt.Sprintf("%v", &ExpectedNetpol)
}

func (m *NetworkPolicyMatcher) Matches(other interface{}) bool {
	otherAsNetpol, ok := other.(*v1.NetworkPolicy)
	if !ok {
		return false
	}

	expectedNetpol := NewNetworkPolicyBuilder(ExpectedNetpol).
		WithPorts(m.ports).
		WithFromIPBlock(m.allowAllIncomingTraffic).
		Build()

	return otherAsNetpol.Namespace == TestNamespace &&
		otherAsNetpol.Name == expectedNetpol.Name &&
		reflect.DeepEqual(otherAsNetpol.Labels, expectedNetpol.Labels) &&
		reflect.DeepEqual(otherAsNetpol.Spec, expectedNetpol.Spec)
}

func NewNetworkPolicyBuilder(base v1.NetworkPolicy) *NetworkPolicyBuilder {
	return &NetworkPolicyBuilder{policy: base.DeepCopy()}
}

func (b *NetworkPolicyBuilder) WithPorts(ports []int32) *NetworkPolicyBuilder {
	b.policy.Spec.Ingress[0].Ports = lo.Map(ports, func(port int32, _ int) v1.NetworkPolicyPort {
		return v1.NetworkPolicyPort{
			Protocol: lo.ToPtr(corev1.ProtocolTCP),
			Port:     lo.ToPtr(intstr.IntOrString{Type: intstr.Int, IntVal: port}),
		}
	})
	return b
}

func (b *NetworkPolicyBuilder) WithFromIPBlock(allowAll bool) *NetworkPolicyBuilder {
	if allowAll {
		// leave .From empty
		return b
	}

	b.policy.Spec.Ingress[0].From = []v1.NetworkPolicyPeer{
		{
			IPBlock: &v1.IPBlock{
				CIDR: fmt.Sprintf("%s/32", TestControlPlaneIP),
			},
		},
	}
	return b
}

func (b *NetworkPolicyBuilder) Build() v1.NetworkPolicy {
	return *b.policy
}
