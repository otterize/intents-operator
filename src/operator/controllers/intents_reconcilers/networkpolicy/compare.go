package networkpolicy

import (
	networkingv1 "k8s.io/api/networking/v1"
	"reflect"
	"sort"
)

// isNetworkPolicySpecEqual compares two NetworkPolicySpec structs, ignoring the order of items in nested slices.
func isNetworkPolicySpecEqual(spec1, spec2 networkingv1.NetworkPolicySpec) bool {
	// Compare PodSelector
	if !reflect.DeepEqual(spec1.PodSelector, spec2.PodSelector) {
		return false
	}

	// Sort and compare Ingress rules
	sortIngressRules(spec1.Ingress)
	sortIngressRules(spec2.Ingress)
	if !reflect.DeepEqual(spec1.Ingress, spec2.Ingress) {
		return false
	}

	// Sort and compare Egress rules
	sortEgressRules(spec1.Egress)
	sortEgressRules(spec2.Egress)
	if !reflect.DeepEqual(spec1.Egress, spec2.Egress) {
		return false
	}

	// Sort and compare PolicyTypes
	sortPolicyTypes(spec1.PolicyTypes)
	sortPolicyTypes(spec2.PolicyTypes)
	if !reflect.DeepEqual(spec1.PolicyTypes, spec2.PolicyTypes) {
		return false
	}

	return true
}

// Helper function to sort Ingress rules by all relevant fields deterministically
func sortIngressRules(rules []networkingv1.NetworkPolicyIngressRule) {
	for i := range rules {
		sortNetworkPolicyPorts(rules[i].Ports)
		sortNetworkPolicyPeers(rules[i].From)
	}
	sort.SliceStable(rules, func(i, j int) bool {
		return reflect.DeepEqual(rules[i], rules[j])
	})
}

// Helper function to sort Egress rules by all relevant fields deterministically
func sortEgressRules(rules []networkingv1.NetworkPolicyEgressRule) {
	for i := range rules {
		sortNetworkPolicyPorts(rules[i].Ports)
		sortNetworkPolicyPeers(rules[i].To)
	}
	sort.SliceStable(rules, func(i, j int) bool {
		return reflect.DeepEqual(rules[i], rules[j])
	})
}

// Helper function to sort NetworkPolicyPorts by all fields deterministically
func sortNetworkPolicyPorts(ports []networkingv1.NetworkPolicyPort) {
	sort.SliceStable(ports, func(i, j int) bool {
		if ports[i].Protocol != nil && ports[j].Protocol != nil {
			if *ports[i].Protocol != *ports[j].Protocol {
				return *ports[i].Protocol < *ports[j].Protocol
			}
		} else if ports[i].Protocol != ports[j].Protocol {
			return ports[i].Protocol != nil
		}

		if ports[i].Port != nil && ports[j].Port != nil {
			if ports[i].Port.IntVal != ports[j].Port.IntVal {
				return ports[i].Port.IntVal < ports[j].Port.IntVal
			}
		} else if ports[i].Port != ports[j].Port {
			return ports[i].Port != nil
		}

		if ports[i].EndPort != nil && ports[j].EndPort != nil {
			return *ports[i].EndPort < *ports[j].EndPort
		}
		return ports[i].EndPort != nil
	})
}

// Helper function to sort NetworkPolicyPeers by a deterministic order
func sortNetworkPolicyPeers(peers []networkingv1.NetworkPolicyPeer) {
	sort.SliceStable(peers, func(i, j int) bool {
		if peers[i].PodSelector != nil && peers[j].PodSelector != nil {
			if !reflect.DeepEqual(peers[i].PodSelector, peers[j].PodSelector) {
				return reflect.DeepEqual(peers[i].PodSelector, peers[j].PodSelector)
			}
		} else if peers[i].PodSelector != peers[j].PodSelector {
			return peers[i].PodSelector != nil
		}

		if peers[i].NamespaceSelector != nil && peers[j].NamespaceSelector != nil {
			if !reflect.DeepEqual(peers[i].NamespaceSelector, peers[j].NamespaceSelector) {
				return reflect.DeepEqual(peers[i].NamespaceSelector, peers[j].NamespaceSelector)
			}
		} else if peers[i].NamespaceSelector != peers[j].NamespaceSelector {
			return peers[i].NamespaceSelector != nil
		}

		if peers[i].IPBlock != nil && peers[j].IPBlock != nil {
			if peers[i].IPBlock.CIDR != peers[j].IPBlock.CIDR {
				return peers[i].IPBlock.CIDR < peers[j].IPBlock.CIDR
			}

			sort.Strings(peers[i].IPBlock.Except)
			sort.Strings(peers[j].IPBlock.Except)
			return reflect.DeepEqual(peers[i].IPBlock.Except, peers[j].IPBlock.Except)
		}
		return peers[i].IPBlock != nil
	})
}

// Helper function to sort PolicyTypes
func sortPolicyTypes(types []networkingv1.PolicyType) {
	sort.SliceStable(types, func(i, j int) bool {
		return types[i] < types[j]
	})
}
