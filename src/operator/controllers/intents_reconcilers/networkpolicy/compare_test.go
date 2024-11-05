package networkpolicy

import (
	"gopkg.in/yaml.v3"
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
	cidr := "192.168.0.0/16"
	except1 := []string{"192.168.1.0/24", "192.168.2.0/24"}
	except2 := []string{"192.168.2.0/24", "192.168.1.0/24"} // Same values in different order

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
					{IPBlock: &networkingv1.IPBlock{CIDR: cidr, Except: except1}},
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
					{IPBlock: &networkingv1.IPBlock{CIDR: cidr, Except: except2}},
					{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"team": "database"}}},
					{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"role": "backend"}}},
				},
			},
		},
		PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
	}

	if !isNetworkPolicySpecEqual(baseSpec, baseSpec) {
		t.Error("Expected NetworkPolicySpecs to be equal")
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
		{
			name: "Same CIDR with Different Except Order",
			modify: func(spec *networkingv1.NetworkPolicySpec) {
				spec.Egress[0].To[2].IPBlock.Except = except2 // Reversed order of Except
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
			name: "Different PodSelector Label",
			modify: func(spec *networkingv1.NetworkPolicySpec) {
				spec.PodSelector.MatchLabels["app"] = "different"
			},
		},
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
				spec.Egress[0].Ports[0].Protocol = &udp
			},
		},
		{
			name: "Different Egress Port Number",
			modify: func(spec *networkingv1.NetworkPolicySpec) {
				spec.Egress[0].Ports[0].Port = &port443
			},
		},
		{
			name: "Different Egress To NamespaceSelector",
			modify: func(spec *networkingv1.NetworkPolicySpec) {
				spec.Egress[0].To[1].NamespaceSelector.MatchLabels["team"] = "frontend"
			},
		},
		{
			name: "Different PolicyTypes",
			modify: func(spec *networkingv1.NetworkPolicySpec) {
				spec.PolicyTypes = []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}
			},
		},
		{
			name: "Different CIDR",
			modify: func(spec *networkingv1.NetworkPolicySpec) {
				spec.Egress[0].To[2].IPBlock.CIDR = "1.1.1.1"
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

func TestPoliticoNetpol(t *testing.T) {
	netpolABuf := []byte(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  creationTimestamp: "2024-08-21T11:16:20Z"
  finalizers:
  - networking.k8s.aws/resources
  generation: 15949
  labels:
    intents.otterize.com/network-policy: howler-qared-e40d9d
  name: howler-access
  namespace: qared
  resourceVersion: "1462861842"
  uid: 50aa1534-baf3-4568-bd3e-77ec901a634c
spec:
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: qared
      podSelector:
        matchLabels:
          intents.otterize.com/access-howler-qared-e40d9d: "true"
  - from:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: qablue
      podSelector:
        matchLabels:
          intents.otterize.com/access-howler-qared-e40d9d: "true"
  podSelector:
    matchLabels:
      intents.otterize.com/service: howler-qared-e40d9d
  policyTypes:
  - Ingress`)

	netpolBBuf := []byte(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  creationTimestamp: "2024-08-21T11:16:20Z"
  finalizers:
  - networking.k8s.aws/resources
  generation: 15949
  labels:
    intents.otterize.com/network-policy: howler-qared-e40d9d
  name: howler-access
  namespace: qared
  resourceVersion: "1462861842"
  uid: 50aa1534-baf3-4568-bd3e-77ec901a634c
spec:
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: qablue
      podSelector:
        matchLabels:
          intents.otterize.com/access-howler-qared-e40d9d: "true"
  - from:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: qared
      podSelector:
        matchLabels:
          intents.otterize.com/access-howler-qared-e40d9d: "true"
  podSelector:
    matchLabels:
      intents.otterize.com/service: howler-qared-e40d9d
  policyTypes:
  - Ingress`)

	var netpolA networkingv1.NetworkPolicy
	var netpolB networkingv1.NetworkPolicy

	err := yaml.Unmarshal(netpolABuf, &netpolA)
	if err != nil {
		t.Error(err)
	}

	err = yaml.Unmarshal(netpolBBuf, &netpolB)
	if err != nil {
		t.Error(err)
	}

	if !isNetworkPolicySpecEqual(netpolA.Spec, netpolB.Spec) {
		t.Error("Expected NetworkPolicySpecs to be equal")
	}

	if !isNetworkPolicySpecEqual(netpolB.Spec, netpolA.Spec) {
		t.Error("Expected NetworkPolicySpecs to be equal")
	}
}

func TestNetworkPolicyRuleOrderIndependenceWithMultipleSelectors(t *testing.T) {
	// Common fields for test initialization
	tcp := v1.ProtocolTCP
	udp := v1.ProtocolUDP
	port80 := intstr.FromInt(80)
	port443 := intstr.FromInt(443)

	// Define two NetworkPolicySpecs with the same Ingress and Egress rules in different orders,
	// each with multiple PodSelector and NamespaceSelector entries in different orders
	spec1 := networkingv1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "test"},
		},
		Ingress: []networkingv1.NetworkPolicyIngressRule{
			{
				Ports: []networkingv1.NetworkPolicyPort{
					{Protocol: &tcp, Port: &port80},
				},
				From: []networkingv1.NetworkPolicyPeer{
					{
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"role": "frontend"},
						},
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"team": "backend"},
						},
					},
					{
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"role": "database"},
						},
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"team": "frontend"},
						},
					},
				},
			},
			{
				Ports: []networkingv1.NetworkPolicyPort{
					{Protocol: &udp, Port: &port443},
				},
				From: []networkingv1.NetworkPolicyPeer{
					{
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"role": "cache"},
						},
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"team": "data"},
						},
					},
				},
			},
		},
		Egress: []networkingv1.NetworkPolicyEgressRule{
			{
				Ports: []networkingv1.NetworkPolicyPort{
					{Protocol: &tcp, Port: &port443},
				},
				To: []networkingv1.NetworkPolicyPeer{
					{
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"role": "app"},
						},
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"team": "frontend"},
						},
					},
					{
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"role": "api"},
						},
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"team": "backend"},
						},
					},
				},
			},
			{
				Ports: []networkingv1.NetworkPolicyPort{
					{Protocol: &udp, Port: &port80},
				},
				To: []networkingv1.NetworkPolicyPeer{
					{
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"role": "service"},
						},
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"team": "dev"},
						},
					},
				},
			},
		},
	}

	// Initialize spec2 by copying spec1 and swapping elements
	spec2 := *spec1.DeepCopy()

	// Swap the order of Ingress rules
	spec2.Ingress[0], spec2.Ingress[1] = spec2.Ingress[1], spec2.Ingress[0]

	// Swap the order of From peers in the first Ingress rule
	spec2.Ingress[1].From[0], spec2.Ingress[1].From[0] = spec2.Ingress[1].From[0], spec2.Ingress[1].From[0]

	// Swap the order of Egress rules
	spec2.Egress[0], spec2.Egress[1] = spec2.Egress[1], spec2.Egress[0]

	// Sort and compare
	if !isNetworkPolicySpecEqual(spec1, spec2) {
		t.Error("Expected NetworkPolicySpecs to be equal regardless of Ingress and Egress rule order and Peer selectors")
	}

	// And in reverse
	if !isNetworkPolicySpecEqual(spec2, spec1) {
		t.Error("Expected NetworkPolicySpecs to be equal regardless of Ingress and Egress rule order and Peer selectors")
	}
}
