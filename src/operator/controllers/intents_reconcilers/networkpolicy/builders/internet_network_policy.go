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
	"github.com/sirupsen/logrus"
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

	if len(intents) == 0 {
		return make([]v1.NetworkPolicyEgressRule, 0), nil
	}

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
	if len(rules) == 0 {
		logrus.Info("cannot create rules for internet network policy")
	}
	return rules, nil
}

func (r *InternetEgressRulesBuilder) buildRuleForIntent(intent otterizev1alpha3.Intent, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyPeer, []v1.NetworkPolicyPort, bool, error) {
	ips := make([]string, 0)
	ipsFromDns := r.getIpsForDNS(intent, ep)
	ips = append(ips, ipsFromDns...)
	ips = append(ips, intent.Internet.Ips...)

	if len(ips) == 0 {
		dnsNames := lo.Reduce(intent.Internet.Domains, func(names, dns string, _ int) string {
			return fmt.Sprintf("%s %s", names, dns)
		}, "")

		ep.ClientIntentsEventRecorder.RecordWarningEventf(consts.ReasonNetworkPolicyCreationFailedMissingIP, "no IPs found for internet intent %s", dnsNames)
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

	for _, dns := range intent.Internet.Domains {
		dnsResolvedIps, found := lo.Find(ep.ClientIntentsStatus.ResolvedIPs, func(resolvedIPs otterizev1alpha3.ResolvedIPs) bool {
			return resolvedIPs.DNS == dns
		})

		if !found {
			ep.ClientIntentsEventRecorder.RecordWarningEventf(consts.ReasonIntentToUnresolvedDns, "could not find IP for DNS %s", dns)
			continue
		}

		ipsFromDns = append(ipsFromDns, dnsResolvedIps.IPs...)
	}

	return ipsFromDns
}
func (r *InternetEgressRulesBuilder) parseIps(ips []string) ([]v1.NetworkPolicyPeer, error) {
	var cidrs []string
	for _, ipStr := range ips {
		cidr, err := getCIDR(ipStr)
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

func getCIDR(ipStr string) (string, error) {
	cidr := ipStr
	if !strings.Contains(ipStr, "/") {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			return "", errors.New(fmt.Sprintf("invalid IP: %s", ipStr))
		}
		isV6 := ip.To4() == nil

		if isV6 {
			cidr = fmt.Sprintf("%s/128", ip)
		} else {
			cidr = fmt.Sprintf("%s/32", ip)
		}
	}

	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", errors.Wrap(err)
	}
	return cidr, nil
}
