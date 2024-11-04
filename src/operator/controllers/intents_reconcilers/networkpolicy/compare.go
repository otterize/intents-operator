package networkpolicy

import (
	"reflect"
	"sort"

	v1 "k8s.io/api/networking/v1"
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
func sortIngressRules(rules []v1.NetworkPolicyIngressRule) {
	sort.SliceStable(rules, func(i, j int) bool {
		// Sort and compare Ports first
		sortNetworkPolicyPorts(rules[i].Ports)
		sortNetworkPolicyPorts(rules[j].Ports)

		if !reflect.DeepEqual(rules[i].Ports, rules[j].Ports) {
			return comparePorts(rules[i].Ports, rules[j].Ports)
		}

		// If Ports are equal, compare based on From field
		sortNetworkPolicyPeers(rules[i].From)
		sortNetworkPolicyPeers(rules[j].From)

		return comparePeers(rules[i].From, rules[j].From)
	})
}

// Helper function to sort Egress rules by all relevant fields deterministically
func sortEgressRules(rules []v1.NetworkPolicyEgressRule) {
	sort.SliceStable(rules, func(i, j int) bool {
		// Sort and compare Ports first
		sortNetworkPolicyPorts(rules[i].Ports)
		sortNetworkPolicyPorts(rules[j].Ports)

		if !reflect.DeepEqual(rules[i].Ports, rules[j].Ports) {
			return comparePorts(rules[i].Ports, rules[j].Ports)
		}

		// If Ports are equal, compare based on To field
		sortNetworkPolicyPeers(rules[i].To)
		sortNetworkPolicyPeers(rules[j].To)

		return comparePeers(rules[i].To, rules[j].To)
	})
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

// Helper function to compare two slices of NetworkPolicyPort deterministically
func comparePorts(ports1, ports2 []v1.NetworkPolicyPort) bool {
	for k := range ports1 {
		if ports1[k].Protocol != nil && ports2[k].Protocol != nil {
			if *ports1[k].Protocol != *ports2[k].Protocol {
				return *ports1[k].Protocol < *ports2[k].Protocol
			}
		} else if ports1[k].Protocol != ports2[k].Protocol {
			return ports1[k].Protocol != nil
		}

		if ports1[k].Port != nil && ports2[k].Port != nil {
			if ports1[k].Port.IntVal != ports2[k].Port.IntVal {
				return ports1[k].Port.IntVal < ports2[k].Port.IntVal
			}
		} else if ports1[k].Port != ports2[k].Port {
			return ports1[k].Port != nil
		}

		if ports1[k].EndPort != nil && ports2[k].EndPort != nil {
			if *ports1[k].EndPort != *ports2[k].EndPort {
				return *ports1[k].EndPort < *ports2[k].EndPort
			}
		} else if ports1[k].EndPort != ports2[k].EndPort {
			return ports1[k].EndPort != nil
		}
	}
	return len(ports1) < len(ports2)
}

// Helper function to compare two slices of NetworkPolicyPeer deterministically
func comparePeers(peers1, peers2 []v1.NetworkPolicyPeer) bool {
	for k := range peers1 {
		if peers1[k].PodSelector != nil && peers2[k].PodSelector != nil {
			if !reflect.DeepEqual(peers1[k].PodSelector, peers2[k].PodSelector) {
				return reflect.DeepEqual(peers1[k].PodSelector, peers2[k].PodSelector)
			}
		} else if peers1[k].PodSelector != peers2[k].PodSelector {
			return peers1[k].PodSelector != nil
		}

		if peers1[k].NamespaceSelector != nil && peers2[k].NamespaceSelector != nil {
			if !reflect.DeepEqual(peers1[k].NamespaceSelector, peers2[k].NamespaceSelector) {
				return reflect.DeepEqual(peers1[k].NamespaceSelector, peers2[k].NamespaceSelector)
			}
		} else if peers1[k].NamespaceSelector != peers2[k].NamespaceSelector {
			return peers1[k].NamespaceSelector != nil
		}

		if peers1[k].IPBlock != nil && peers2[k].IPBlock != nil {
			if peers1[k].IPBlock.CIDR != peers2[k].IPBlock.CIDR {
				return peers1[k].IPBlock.CIDR < peers2[k].IPBlock.CIDR
			}
			sort.Strings(peers1[k].IPBlock.Except)
			sort.Strings(peers2[k].IPBlock.Except)
			return reflect.DeepEqual(peers1[k].IPBlock.Except, peers2[k].IPBlock.Except)
		} else if peers1[k].IPBlock != peers2[k].IPBlock {
			return peers1[k].IPBlock != nil
		}
	}
	return len(peers1) < len(peers2)
}

// Helper function to sort PolicyTypes
func sortPolicyTypes(types []v1.PolicyType) {
	sort.SliceStable(types, func(i, j int) bool {
		return types[i] < types[j]
	})
}
