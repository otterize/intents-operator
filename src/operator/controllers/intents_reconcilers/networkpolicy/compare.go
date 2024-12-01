package networkpolicy

import (
	"github.com/go-test/deep"
	"github.com/otterize/intents-operator/src/shared/errors"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"sort"
	"strings"
)

// PAY ATTENTION: deepEqual is sensitive the differance between nil and empty slice
// therefore, we marshal and unmarshal to nullify empty slices of the new policy
func marshalUnmarshalNetpol(netpol *networkingv1.NetworkPolicy) (networkingv1.NetworkPolicy, error) {
	data, err := netpol.Marshal()
	if err != nil {
		return networkingv1.NetworkPolicy{}, errors.Wrap(err)
	}
	newNetpol := networkingv1.NetworkPolicy{}
	err = newNetpol.Unmarshal(data)
	if err != nil {
		return networkingv1.NetworkPolicy{}, errors.Wrap(err)
	}
	return newNetpol, nil
}

// isNetworkPolicySpecEqual compares two NetworkPolicySpec structs, ignoring the order of items in nested slices.
func isNetworkPolicySpecEqual(spec1, spec2 networkingv1.NetworkPolicySpec) bool {

	// Compare PodSelector
	if !reflect.DeepEqual(spec1.PodSelector, spec2.PodSelector) {
		return false
	}

	// Sort and compare Ingress rules
	sortIngressRules(spec1.Ingress)
	sortIngressRules(spec2.Ingress)
	if diffs := deep.Equal(spec1.Ingress, spec2.Ingress); len(diffs) != 0 {
		println(diffs)
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

// Sorts Ingress rules in a consistent order
func sortIngressRules(rules []networkingv1.NetworkPolicyIngressRule) {
	for i := range rules {
		// Sort the Ports and From fields within each rule
		sortNetworkPolicyPorts(rules[i].Ports)
		sortNetworkPolicyPeers(rules[i].From)
	}

	// Sort the rules based on Ports and From fields in a consistent order
	sort.SliceStable(rules, func(i, j int) bool {
		// Compare Ports
		for k := 0; k < len(rules[i].Ports) && k < len(rules[j].Ports); k++ {
			if !portsEqual(rules[i].Ports[k], rules[j].Ports[k]) {
				return lessPorts(rules[i].Ports[k], rules[j].Ports[k])
			}
		}
		if len(rules[i].Ports) != len(rules[j].Ports) {
			return len(rules[i].Ports) < len(rules[j].Ports)
		}

		// Compare From peers
		for k := 0; k < len(rules[i].From) && k < len(rules[j].From); k++ {
			if !peersEqual(rules[i].From[k], rules[j].From[k]) {
				return lessPeers(rules[i].From[k], rules[j].From[k])
			}
		}
		return len(rules[i].From) < len(rules[j].From)
	})
}

// Sorts Egress rules in a consistent order
func sortEgressRules(rules []networkingv1.NetworkPolicyEgressRule) {
	for i := range rules {
		// Sort the Ports and To fields within each rule
		sortNetworkPolicyPorts(rules[i].Ports)
		sortNetworkPolicyPeers(rules[i].To)
	}

	// Sort the rules based on Ports and To fields in a consistent order
	sort.SliceStable(rules, func(i, j int) bool {
		// Compare Ports
		for k := 0; k < len(rules[i].Ports) && k < len(rules[j].Ports); k++ {
			if !portsEqual(rules[i].Ports[k], rules[j].Ports[k]) {
				return lessPorts(rules[i].Ports[k], rules[j].Ports[k])
			}
		}
		if len(rules[i].Ports) != len(rules[j].Ports) {
			return len(rules[i].Ports) < len(rules[j].Ports)
		}

		// Compare To peers
		for k := 0; k < len(rules[i].To) && k < len(rules[j].To); k++ {
			if !peersEqual(rules[i].To[k], rules[j].To[k]) {
				return lessPeers(rules[i].To[k], rules[j].To[k])
			}
		}
		return len(rules[i].To) < len(rules[j].To)
	})
}

// Helper function to check if two NetworkPolicyPort objects are equal
func portsEqual(port1, port2 networkingv1.NetworkPolicyPort) bool {
	return reflect.DeepEqual(port1, port2)
}

// Helper function to determine consistent ordering between two NetworkPolicyPort objects
func lessPorts(port1, port2 networkingv1.NetworkPolicyPort) bool {
	if port1.Protocol != nil && port2.Protocol != nil {
		if *port1.Protocol != *port2.Protocol {
			return *port1.Protocol < *port2.Protocol
		}
	} else if port1.Protocol != port2.Protocol {
		return port1.Protocol != nil
	}

	if port1.Port != nil && port2.Port != nil {
		return port1.Port.IntVal < port2.Port.IntVal
	}
	return port1.Port != nil
}

// Helper function to check if two NetworkPolicyPeer objects are equal
func peersEqual(peer1, peer2 networkingv1.NetworkPolicyPeer) bool {
	return reflect.DeepEqual(peer1, peer2)
}

// Helper function to determine consistent ordering between two NetworkPolicyPeer objects
func lessPeers(peer1, peer2 networkingv1.NetworkPolicyPeer) bool {
	if peer1.PodSelector != nil && peer2.PodSelector != nil {
		if !reflect.DeepEqual(peer1.PodSelector, peer2.PodSelector) {
			return peer1.PodSelector.String() < peer2.PodSelector.String()
		}
	} else if peer1.PodSelector != peer2.PodSelector {
		return peer1.PodSelector != nil
	}

	if peer1.NamespaceSelector != nil && peer2.NamespaceSelector != nil {
		if !reflect.DeepEqual(peer1.NamespaceSelector, peer2.NamespaceSelector) {
			return peer1.NamespaceSelector.String() < peer2.NamespaceSelector.String()
		}
	} else if peer1.NamespaceSelector != peer2.NamespaceSelector {
		return peer1.NamespaceSelector != nil
	}

	if peer1.IPBlock != nil && peer2.IPBlock != nil {
		if peer1.IPBlock.CIDR != peer2.IPBlock.CIDR {
			return peer1.IPBlock.CIDR < peer2.IPBlock.CIDR
		}
		// Sort by Except list if CIDRs are equal
		return lessExceptList(peer1.IPBlock.Except, peer2.IPBlock.Except)
	}
	return peer1.IPBlock != nil
}

// Helper function to determine consistent ordering between two lists of Except CIDRs
func lessExceptList(except1, except2 []string) bool {
	sort.Strings(except1)
	sort.Strings(except2)
	for i := 0; i < len(except1) && i < len(except2); i++ {
		if except1[i] != except2[i] {
			return except1[i] < except2[i]
		}
	}
	return len(except1) < len(except2)
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

func sortNetworkPolicyPeers(peers []networkingv1.NetworkPolicyPeer) {
	sort.SliceStable(peers, func(i, j int) bool {
		return lessPeer(peers[i], peers[j])
	})
}

// Helper function to determine consistent ordering between two NetworkPolicyPeer objects
func lessPeer(peer1, peer2 networkingv1.NetworkPolicyPeer) bool {
	// Compare PodSelector
	if peer1.PodSelector != nil && peer2.PodSelector != nil {
		if !selectorsEqual(*peer1.PodSelector, *peer2.PodSelector) {
			return selectorToString(*peer1.PodSelector) < selectorToString(*peer2.PodSelector)
		}
	} else if peer1.PodSelector != peer2.PodSelector {
		return peer1.PodSelector != nil
	}

	// Compare NamespaceSelector
	if peer1.NamespaceSelector != nil && peer2.NamespaceSelector != nil {
		if !selectorsEqual(*peer1.NamespaceSelector, *peer2.NamespaceSelector) {
			return selectorToString(*peer1.NamespaceSelector) < selectorToString(*peer2.NamespaceSelector)
		}
	} else if peer1.NamespaceSelector != peer2.NamespaceSelector {
		return peer1.NamespaceSelector != nil
	}

	// Compare IPBlock
	if peer1.IPBlock != nil && peer2.IPBlock != nil {
		if peer1.IPBlock.CIDR != peer2.IPBlock.CIDR {
			return peer1.IPBlock.CIDR < peer2.IPBlock.CIDR
		}
		// Sort by Except list if CIDRs are equal
		return lessExceptList(peer1.IPBlock.Except, peer2.IPBlock.Except)
	}
	return peer1.IPBlock != nil
}

// Helper function to check if two LabelSelectors are equal
func selectorsEqual(sel1, sel2 metav1.LabelSelector) bool {
	return selectorToString(sel1) == selectorToString(sel2)
}

// Helper function to convert a LabelSelector to a string for consistent comparison
func selectorToString(selector metav1.LabelSelector) string {
	var sb strings.Builder
	sb.WriteString("MatchLabels:")
	if selector.MatchLabels != nil {
		keys := make([]string, 0, len(selector.MatchLabels))
		for k := range selector.MatchLabels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString(k + "=" + selector.MatchLabels[k] + ",")
		}
	}
	sb.WriteString("MatchExpressions:")
	for _, expr := range selector.MatchExpressions {
		sb.WriteString(expr.Key + "=" + string(expr.Operator) + "[")
		sort.Strings(expr.Values)
		for _, v := range expr.Values {
			sb.WriteString(v + ",")
		}
		sb.WriteString("],")
	}
	return sb.String()
}

// Helper function to sort PolicyTypes
func sortPolicyTypes(types []networkingv1.PolicyType) {
	sort.SliceStable(types, func(i, j int) bool {
		return types[i] < types[j]
	})
}
