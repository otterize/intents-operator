package builders

import (
	"context"
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"net"
	"strings"
)

// The InternetEgressRulesBuilder creates network policies that allow egress traffic from pods.
type InternetEgressRulesBuilder struct {
	injectablerecorder.InjectableRecorder
}

func NewInternetEgressRulesBuilder() *InternetEgressRulesBuilder {
	return &InternetEgressRulesBuilder{}
}

func (r *InternetEgressRulesBuilder) buildEgressRules(ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyEgressRule, error) {
	rules := make([]v1.NetworkPolicyEgressRule, 0)

	// Get all intents to the internet
	intents := lo.Filter(ep.Calls, func(intent otterizev1alpha3.Intent, _ int) bool {
		return intent.Type == otterizev1alpha3.IntentTypeInternet
	})

	for _, intent := range intents {
		peers, ports, ok, err := r.buildRuleForIntent(intent, ep)
		if err != nil {
			return nil, errors.Wrap(err)
		}
		if !ok {
			continue
		}
		rules = append(rules, v1.NetworkPolicyEgressRule{
			To:    peers,
			Ports: ports,
		})
	}
	return rules, nil
}

func (r *InternetEgressRulesBuilder) buildRuleForIntent(intent otterizev1alpha3.Intent, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyPeer, []v1.NetworkPolicyPort, bool, error) {
	ips := make([]string, 0)
	ipsFromDns := r.getIpsForDNS(intent, ep)

	ips = append(ips, ipsFromDns...)
	ips = append(ips, intent.Internet.Ips...)

	if len(ips) == 0 {
		ep.ClientIntentsEventRecorder.RecordWarningEventf(consts.ReasonNetworkPolicyCreationFailedMissingIP, "no IPs found for internet intent %s", intent.Internet.Dns)
		return nil, nil, false, nil
	}

	peers, err := r.parseIps(ips)
	if err != nil {
		return nil, nil, false, errors.Wrap(err)
	}
	ports := r.parsePorts(intent)
	return peers, ports, true, nil
}

func (r *InternetEgressRulesBuilder) getIpsForDNS(intent otterizev1alpha3.Intent, ep effectivepolicy.ServiceEffectivePolicy) []string {
	ipsFromDns := make([]string, 0)
	if intent.Internet.Dns == "" {
		return ipsFromDns
	}
	dnsResolvedIps, found := lo.Find(ep.ClientIntentsStatus.ResolvedIPs, func(resolvedIPs otterizev1alpha3.ResolvedIPs) bool {
		return resolvedIPs.DNS == intent.Internet.Dns
	})

	if !found {
		ep.ClientIntentsEventRecorder.RecordWarningEventf(consts.ReasonIntentToUnresolvedDns, "could not find IP for DNS %s", intent.Internet.Dns)
		return ipsFromDns
	}

	ipsFromDns = dnsResolvedIps.IPs
	return ipsFromDns
}

func (r *InternetEgressRulesBuilder) parseIps(ips []string) ([]v1.NetworkPolicyPeer, error) {
	var cidrs []string
	for _, ip := range ips {
		var cidr string
		if !strings.Contains(ip, "/") {
			cidr = fmt.Sprintf("%s/32", ip)
		} else {
			cidr = ip
		}

		_, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, errors.Wrap(err)
		}

		cidrs = append(cidrs, cidr)
	}

	peers := make([]v1.NetworkPolicyPeer, 0)
	for _, cidr := range cidrs {
		peers = append(peers, v1.NetworkPolicyPeer{
			IPBlock: &v1.IPBlock{
				CIDR: cidr,
			},
		})
	}
	return peers, nil
}

func (r *InternetEgressRulesBuilder) parsePorts(intent otterizev1alpha3.Intent) []v1.NetworkPolicyPort {
	ports := make([]v1.NetworkPolicyPort, 0)
	for _, port := range intent.Internet.Ports {
		ports = append(ports, v1.NetworkPolicyPort{
			Port: &intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: int32(port),
			},
		})
	}
	return ports
}

func (r *InternetEgressRulesBuilder) Build(_ context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyEgressRule, error) {
	return r.buildEgressRules(ep)
}
