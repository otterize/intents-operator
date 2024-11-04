package networkpolicy

import (
	v1 "k8s.io/api/networking/v1"
	"reflect"
	"sort"
)

// isNetworkPolicySpecEqual compares two NetworkPolicySpec structs, ignoring the order of items in nested slices.
func isNetworkPolicySpecEqual(spec1, spec2 v1.NetworkPolicySpec) bool {
	// Compare PodSelector
	if !reflect.DeepEqual(spec1.PodSelector, spec2.PodSelector) {
		return false
	}

	// Sort and compare Ingress rules
	sortIngressRules(spec1.Ingress)
	sortIngressRules(spec2.Ingress)
	if !compareIngressRules(spec1.Ingress, spec2.Ingress) {
		return false
	}

	// Sort and compare Egress rules
	sortEgressRules(spec1.Egress)
	sortEgressRules(spec2.Egress)
	if !compareEgressRules(spec1.Egress, spec2.Egress) {
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

// compareIngressRules compares two slices of NetworkPolicyIngressRule, ignoring order within the From and Ports fields.
func compareIngressRules(rules1, rules2 []v1.NetworkPolicyIngressRule) bool {
	if len(rules1) != len(rules2) {
		return false
	}
	for i := range rules1 {
		sortNetworkPolicyPorts(rules1[i].Ports)
		sortNetworkPolicyPorts(rules2[i].Ports)
		if !reflect.DeepEqual(rules1[i].Ports, rules2[i].Ports) {
			return false
		}

		sortNetworkPolicyPeers(rules1[i].From)
		sortNetworkPolicyPeers(rules2[i].From)
		if !reflect.DeepEqual(rules1[i].From, rules2[i].From) {
			return false
		}
	}
	return true
}

// compareEgressRules compares two slices of NetworkPolicyEgressRule, ignoring order within the To and Ports fields.
func compareEgressRules(rules1, rules2 []v1.NetworkPolicyEgressRule) bool {
	if len(rules1) != len(rules2) {
		return false
	}
	for i := range rules1 {
		sortNetworkPolicyPorts(rules1[i].Ports)
		sortNetworkPolicyPorts(rules2[i].Ports)
		if !reflect.DeepEqual(rules1[i].Ports, rules2[i].Ports) {
			return false
		}

		sortNetworkPolicyPeers(rules1[i].To)
		sortNetworkPolicyPeers(rules2[i].To)
		if !reflect.DeepEqual(rules1[i].To, rules2[i].To) {
			return false
		}
	}
	return true
}

// Helper function to sort NetworkPolicyPorts by all fields deterministically
func sortNetworkPolicyPorts(ports []v1.NetworkPolicyPort) {
	sort.SliceStable(ports, func(i, j int) bool {
		// Compare Protocols (nil-safe)
		if ports[i].Protocol != ports[j].Protocol {
			if ports[i].Protocol == nil {
				return true
			}
			if ports[j].Protocol == nil {
				return false
			}
			if *ports[i].Protocol != *ports[j].Protocol {
				return *ports[i].Protocol < *ports[j].Protocol
			}
		}

		// Compare Ports (nil-safe)
		if ports[i].Port != ports[j].Port {
			if ports[i].Port == nil {
				return true
			}
			if ports[j].Port == nil {
				return false
			}
			if ports[i].Port.IntVal != ports[j].Port.IntVal {
				return ports[i].Port.IntVal < ports[j].Port.IntVal
			}
		}

		// Compare EndPorts (nil-safe)
		if ports[i].EndPort != ports[j].EndPort {
			if ports[i].EndPort == nil {
				return true
			}
			if ports[j].EndPort == nil {
				return false
			}
			return *ports[i].EndPort < *ports[j].EndPort
		}

		return false
	})
}

// Helper function to sort NetworkPolicyPeers by a deterministic order
func sortNetworkPolicyPeers(peers []v1.NetworkPolicyPeer) {
	sort.SliceStable(peers, func(i, j int) bool {
		// Compare PodSelectors (nil-safe)
		if peers[i].PodSelector != peers[j].PodSelector {
			if peers[i].PodSelector == nil {
				return true
			}
			if peers[j].PodSelector == nil {
				return false
			}
			return reflect.DeepEqual(peers[i].PodSelector, peers[j].PodSelector)
		}

		// Compare NamespaceSelectors (nil-safe)
		if peers[i].NamespaceSelector != peers[j].NamespaceSelector {
			if peers[i].NamespaceSelector == nil {
				return true
			}
			if peers[j].NamespaceSelector == nil {
				return false
			}
			return reflect.DeepEqual(peers[i].NamespaceSelector, peers[j].NamespaceSelector)
		}

		// Compare IPBlocks by CIDR and Except (nil-safe)
		if peers[i].IPBlock != peers[j].IPBlock {
			if peers[i].IPBlock == nil {
				return true
			}
			if peers[j].IPBlock == nil {
				return false
			}
			if peers[i].IPBlock.CIDR != peers[j].IPBlock.CIDR {
				return peers[i].IPBlock.CIDR < peers[j].IPBlock.CIDR
			}

			// Compare IPBlock Exceptions if CIDRs are equal
			sort.Strings(peers[i].IPBlock.Except)
			sort.Strings(peers[j].IPBlock.Except)
			return reflect.DeepEqual(peers[i].IPBlock.Except, peers[j].IPBlock.Except)
		}
		return false
	})
}

// Helper function to sort PolicyTypes
func sortPolicyTypes(types []v1.PolicyType) {
	sort.SliceStable(types, func(i, j int) bool {
		return types[i] < types[j]
	})
}

// Helper function to sort Ingress rules by a deterministic order
func sortIngressRules(rules []v1.NetworkPolicyIngressRule) {
	sort.SliceStable(rules, func(i, j int) bool {
		return len(rules[i].Ports) < len(rules[j].Ports)
	})
}

// Helper function to sort Egress rules by a deterministic order
func sortEgressRules(rules []v1.NetworkPolicyEgressRule) {
	sort.SliceStable(rules, func(i, j int) bool {
		return len(rules[i].Ports) < len(rules[j].Ports)
	})
}
