package networkpolicy

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"testing"
)

func TestNetworkPolicySpecComparison(t *testing.T) {
	// Create a base NetworkPolicySpec for testing
	tcp := corev1.ProtocolTCP
	udp := corev1.ProtocolUDP
	port80 := intstr.FromInt(80)
	port443 := intstr.FromInt(443)
	port53 := intstr.FromInt(53)

	baseSpec := v1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "test"},
		},
		Ingress: []v1.NetworkPolicyIngressRule{
			{
				Ports: []v1.NetworkPolicyPort{
					{Protocol: &tcp, Port: &port80},
					{Protocol: &udp, Port: &port443},
				},
				From: []v1.NetworkPolicyPeer{
					{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"role": "frontend"}}},
					{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "backend"}}},
				},
			},
		},
		Egress: []v1.NetworkPolicyEgressRule{
			{
				Ports: []v1.NetworkPolicyPort{
					{Protocol: &udp, Port: &port53},
					{Protocol: &tcp, Port: &port80},
				},
				To: []v1.NetworkPolicyPeer{
					{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "database"}}},
					{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"role": "backend"}}},
				},
			},
		},
		PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress, v1.PolicyTypeEgress},
	}

	// Helper function to clone the baseSpec
	cloneSpec := func() v1.NetworkPolicySpec {
		return v1.NetworkPolicySpec{
			PodSelector: *baseSpec.PodSelector.DeepCopy(),
			Ingress:     deepCopyIngressRules(baseSpec.Ingress),
			Egress:      deepCopyEgressRules(baseSpec.Egress),
			PolicyTypes: append([]v1.PolicyType{}, baseSpec.PolicyTypes...),
		}
	}

	// Test identical specs pass
	spec1 := cloneSpec()
	spec2 := cloneSpec()
	if !isNetworkPolicySpecEqual(spec1, spec2) {
		t.Error("Expected identical NetworkPolicySpecs to be equal")
	}

	// Test each individual field and subfield
	tests := []struct {
		name   string
		modify func(spec *v1.NetworkPolicySpec)
	}{
		{
			name: "Different PodSelector",
			modify: func(spec *v1.NetworkPolicySpec) {
				spec.PodSelector.MatchLabels["app"] = "different"
			},
		},
		{
			name: "Different Ingress Port Protocol",
			modify: func(spec *v1.NetworkPolicySpec) {
				spec.Ingress[0].Ports[0].Protocol = &udp
			},
		},
		{
			name: "Different Ingress Port Number",
			modify: func(spec *v1.NetworkPolicySpec) {
				spec.Ingress[0].Ports[0].Port = &port443
			},
		},
		{
			name: "Different Ingress From PodSelector",
			modify: func(spec *v1.NetworkPolicySpec) {
				spec.Ingress[0].From[0].PodSelector.MatchLabels["role"] = "backend"
			},
		},
		{
			name: "Different Egress Port Protocol",
			modify: func(spec *v1.NetworkPolicySpec) {
				spec.Egress[0].Ports[0].Protocol = &tcp
			},
		},
		{
			name: "Different Egress Port Number",
			modify: func(spec *v1.NetworkPolicySpec) {
				spec.Egress[0].Ports[0].Port = &port80
			},
		},
		{
			name: "Different Egress To NamespaceSelector",
			modify: func(spec *v1.NetworkPolicySpec) {
				spec.Egress[0].To[0].NamespaceSelector.MatchLabels["team"] = "frontend"
			},
		},
		{
			name: "Different PolicyTypes",
			modify: func(spec *v1.NetworkPolicySpec) {
				spec.PolicyTypes = []v1.PolicyType{v1.PolicyTypeIngress}
			},
		},
		// Modifying order of Ingress Ports should cause inequality
		{
			name: "Different Ingress Ports Order",
			modify: func(spec *v1.NetworkPolicySpec) {
				spec.Ingress[0].Ports[0], spec.Ingress[0].Ports[1] = spec.Ingress[0].Ports[1], spec.Ingress[0].Ports[0]
			},
		},
		// Modifying order of Ingress From peers should cause inequality
		{
			name: "Different Ingress From Order",
			modify: func(spec *v1.NetworkPolicySpec) {
				spec.Ingress[0].From[0], spec.Ingress[0].From[1] = spec.Ingress[0].From[1], spec.Ingress[0].From[0]
			},
		},
		// Modifying order of Egress Ports should cause inequality
		{
			name: "Different Egress Ports Order",
			modify: func(spec *v1.NetworkPolicySpec) {
				spec.Egress[0].Ports[0], spec.Egress[0].Ports[1] = spec.Egress[0].Ports[1], spec.Egress[0].Ports[0]
			},
		},
		// Modifying order of Egress To peers should cause inequality
		{
			name: "Different Egress To Order",
			modify: func(spec *v1.NetworkPolicySpec) {
				spec.Egress[0].To[0], spec.Egress[0].To[1] = spec.Egress[0].To[1], spec.Egress[0].To[0]
			},
		},
		// Modifying order of PolicyTypes should cause inequality
		{
			name: "Different PolicyTypes Order",
			modify: func(spec *v1.NetworkPolicySpec) {
				spec.PolicyTypes[0], spec.PolicyTypes[1] = spec.PolicyTypes[1], spec.PolicyTypes[0]
			},
		},
	}

	// Run each test case
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec1 := cloneSpec()
			spec2 := cloneSpec()
			test.modify(&spec2)
			if isNetworkPolicySpecEqual(spec1, spec2) {
				t.Errorf("Expected NetworkPolicySpecs to be unequal: %s", test.name)
			}
		})
	}
}

// Helper functions to deep copy Ingress and Egress rules for test isolation
func deepCopyIngressRules(rules []v1.NetworkPolicyIngressRule) []v1.NetworkPolicyIngressRule {
	copied := make([]v1.NetworkPolicyIngressRule, len(rules))
	for i := range rules {
		copied[i] = *rules[i].DeepCopy()
	}
	return copied
}

func deepCopyEgressRules(rules []v1.NetworkPolicyEgressRule) []v1.NetworkPolicyEgressRule {
	copied := make([]v1.NetworkPolicyEgressRule, len(rules))
	for i := range rules {
		copied[i] = *rules[i].DeepCopy()
	}
	return copied
}
