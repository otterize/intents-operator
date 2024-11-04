package networkpolicy

import (
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"testing"
)

func TestNetworkPolicySpecComparison(t *testing.T) {
	// Common fields for test initialization
	tcp := v1.ProtocolTCP
	udp := v1.ProtocolUDP
	port80 := intstr.FromInt(80)
	port443 := intstr.FromInt(443)
	port53 := intstr.FromInt(53)

	baseSpec := networkingv1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "test"},
		},
		Ingress: []networkingv1.NetworkPolicyIngressRule{
			{
				Ports: []networkingv1.NetworkPolicyPort{
					{Protocol: &tcp, Port: &port80},
					{Protocol: &udp, Port: &port443},
				},
				From: []networkingv1.NetworkPolicyPeer{
					{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"role": "frontend"}}},
					{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "backend"}}},
				},
			},
		},
		Egress: []networkingv1.NetworkPolicyEgressRule{
			{
				Ports: []networkingv1.NetworkPolicyPort{
					{Protocol: &udp, Port: &port53},
					{Protocol: &tcp, Port: &port80},
				},
				To: []networkingv1.NetworkPolicyPeer{
					{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "database"}}},
					{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"role": "backend"}}},
				},
			},
		},
		PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
	}

	// Helper function to clone the baseSpec with DeepCopy
	cloneSpec := func() networkingv1.NetworkPolicySpec {
		return networkingv1.NetworkPolicySpec{
			PodSelector: *baseSpec.PodSelector.DeepCopy(),
			Ingress:     deepCopyIngressRules(baseSpec.Ingress),
			Egress:      deepCopyEgressRules(baseSpec.Egress),
			PolicyTypes: append([]networkingv1.PolicyType{}, baseSpec.PolicyTypes...),
		}
	}

	// Test cases that should still be equal despite different ordering
	orderIndependentTests := []struct {
		name   string
		modify func(spec *networkingv1.NetworkPolicySpec)
	}{
		{
			name: "Different Ingress Ports Order",
			modify: func(spec *networkingv1.NetworkPolicySpec) {
				spec.Ingress[0].Ports[0], spec.Ingress[0].Ports[1] = spec.Ingress[0].Ports[1], spec.Ingress[0].Ports[0]
			},
		},
		{
			name: "Different Ingress From Order",
			modify: func(spec *networkingv1.NetworkPolicySpec) {
				spec.Ingress[0].From[0], spec.Ingress[0].From[1] = spec.Ingress[0].From[1], spec.Ingress[0].From[0]
			},
		},
		{
			name: "Different Egress Ports Order",
			modify: func(spec *networkingv1.NetworkPolicySpec) {
				spec.Egress[0].Ports[0], spec.Egress[0].Ports[1] = spec.Egress[0].Ports[1], spec.Egress[0].Ports[0]
			},
		},
		{
			name: "Different Egress To Order",
			modify: func(spec *networkingv1.NetworkPolicySpec) {
				spec.Egress[0].To[0], spec.Egress[0].To[1] = spec.Egress[0].To[1], spec.Egress[0].To[0]
			},
		},
		{
			name: "Different PolicyTypes Order",
			modify: func(spec *networkingv1.NetworkPolicySpec) {
				spec.PolicyTypes[0], spec.PolicyTypes[1] = spec.PolicyTypes[1], spec.PolicyTypes[0]
			},
		},
	}

	for _, test := range orderIndependentTests {
		t.Run(test.name, func(t *testing.T) {
			spec1 := cloneSpec()
			spec2 := cloneSpec()
			test.modify(&spec2)
			if !isNetworkPolicySpecEqual(spec1, spec2) {
				t.Errorf("Expected NetworkPolicySpecs to be equal despite %s", test.name)
			}
		})
	}

	// Test cases that should result in inequality
	inequalityTests := []struct {
		name   string
		modify func(spec *networkingv1.NetworkPolicySpec)
	}{
		{
			name: "Different Ingress Port Protocol",
			modify: func(spec *networkingv1.NetworkPolicySpec) {
				spec.Ingress[0].Ports[0].Protocol = &udp
			},
		},
		{
			name: "Different Ingress Port Number",
			modify: func(spec *networkingv1.NetworkPolicySpec) {
				spec.Ingress[0].Ports[0].Port = &port443
			},
		},
		{
			name: "Different Ingress From PodSelector",
			modify: func(spec *networkingv1.NetworkPolicySpec) {
				spec.Ingress[0].From[0].PodSelector.MatchLabels["role"] = "backend"
			},
		},
		{
			name: "Different Egress Port Protocol",
			modify: func(spec *networkingv1.NetworkPolicySpec) {
				spec.Egress[0].Ports[0].Protocol = &tcp
			},
		},
		{
			name: "Different Egress Port Number",
			modify: func(spec *networkingv1.NetworkPolicySpec) {
				spec.Egress[0].Ports[0].Port = &port80
			},
		},
		{
			name: "Different Egress To NamespaceSelector",
			modify: func(spec *networkingv1.NetworkPolicySpec) {
				spec.Egress[0].To[0].NamespaceSelector.MatchLabels["team"] = "frontend"
			},
		},
		{
			name: "Different PolicyTypes",
			modify: func(spec *networkingv1.NetworkPolicySpec) {
				spec.PolicyTypes = []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}
			},
		},
	}

	for _, test := range inequalityTests {
		t.Run(test.name, func(t *testing.T) {
			spec1 := cloneSpec()
			spec2 := cloneSpec()
			test.modify(&spec2)
			if isNetworkPolicySpecEqual(spec1, spec2) {
				t.Errorf("Expected NetworkPolicySpecs to be unequal due to %s", test.name)
			}
		})
	}
}

// Helper functions to deep copy Ingress and Egress rules for test isolation
func deepCopyIngressRules(rules []networkingv1.NetworkPolicyIngressRule) []networkingv1.NetworkPolicyIngressRule {
	copied := make([]networkingv1.NetworkPolicyIngressRule, len(rules))
	for i := range rules {
		copied[i] = *rules[i].DeepCopy()
	}
	return copied
}

func deepCopyEgressRules(rules []networkingv1.NetworkPolicyEgressRule) []networkingv1.NetworkPolicyEgressRule {
	copied := make([]networkingv1.NetworkPolicyEgressRule, len(rules))
	for i := range rules {
		copied[i] = *rules[i].DeepCopy()
	}
	return copied
}
