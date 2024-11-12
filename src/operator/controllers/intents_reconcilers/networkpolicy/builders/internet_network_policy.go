package builders

import (
	"context"
	"fmt"
	"github.com/amit7itz/goset"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/samber/lo"
	"github.com/spf13/viper"
	"golang.org/x/exp/slices"
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
	intents := lo.Filter(ep.Calls, func(intent effectivepolicy.Call, _ int) bool {
		return intent.Internet != nil
	})

	if len(intents) == 0 {
		return make([]v1.NetworkPolicyEgressRule, 0), nil
	}

	for _, intent := range intents {
		peers, ports, foundResolvedDNSNames, err := r.buildRuleForIntent(intent.Target, ep)
		if err != nil {
			return nil, errors.Wrap(err)
		}
		if !foundResolvedDNSNames {
			ep.ClientIntentsEventRecorder.RecordNormalEventf(consts.ReasonInternetEgressNetworkPolicyCreationWaitingUnresolvedDNS, "Network policy not created for internet intents as no DNS names were resolved yet; once traffic is observed, a matching network policy will be created")
			continue
		}
		rules = append(rules, v1.NetworkPolicyEgressRule{
			To:    peers,
			Ports: ports,
		})
	}

	return rules, nil
}

func (r *InternetEgressRulesBuilder) buildRuleForIntent(intent otterizev2alpha1.Target, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyPeer, []v1.NetworkPolicyPort, bool, error) {
	ips := r.getIpsForDNS(intent, ep)
	for _, existingIp := range intent.Internet.Ips {
		ips[existingIp] = struct{}{}
	}

	if len(ips) == 0 {
		return nil, nil, false, nil
	}

	peers, err := getIPsAsPeers(ips, false)
	if err != nil {
		return nil, nil, false, errors.Wrap(err)
	}

	if viper.GetBool(operatorconfig.EnableGroupInternetIPsByCIDRKey) && len(peers) > viper.GetInt(operatorconfig.EnableGroupInternetIPsByCIDRPeersLimitKey) {
		peers, err = getIPsAsPeers(ips, true)
		if err != nil {
			return nil, nil, false, errors.Wrap(err)
		}
	}

	slices.SortFunc(peers, func(a, b v1.NetworkPolicyPeer) bool {
		return a.IPBlock.CIDR < b.IPBlock.CIDR
	})

	ports := make([]v1.NetworkPolicyPort, 0)
	for _, port := range intent.Internet.Ports {
		ports = append(ports, v1.NetworkPolicyPort{
			Port: &intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: int32(port),
			},
		})
	}
	return peers, ports, true, nil
}

func (r *InternetEgressRulesBuilder) getIpsForDNS(intent otterizev2alpha1.Target, ep effectivepolicy.ServiceEffectivePolicy) map[string]struct{} {
	ipsFromDns := make(map[string]struct{})

	for _, dns := range intent.Internet.Domains {
		dnsResolvedIps, found := lo.Find(ep.ClientIntentsStatus.ResolvedIPs, func(resolvedIPs otterizev2alpha1.ResolvedIPs) bool {
			return resolvedIPs.DNS == dns
		})

		if !found {
			continue
		}

		for _, ip := range dnsResolvedIps.IPs {
			ipsFromDns[ip] = struct{}{}
		}
	}

	return ipsFromDns
}

func (r *InternetEgressRulesBuilder) Build(_ context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyEgressRule, error) {
	return r.buildEgressRules(ep)
}

func getIPsAsPeers(ips map[string]struct{}, groupBySubnet bool) ([]v1.NetworkPolicyPeer, error) {
	peers := make([]v1.NetworkPolicyPeer, 0)
	cidrSet := goset.NewSet[string]()
	for ip := range ips {
		cidr, err := getCIDR(ip, groupBySubnet)
		if err != nil {
			return nil, errors.Wrap(err)
		}
		cidrSet.Add(cidr.String())
	}

	peers = lo.Map(cidrSet.Items(), func(cidrStr string, _ int) v1.NetworkPolicyPeer {
		return v1.NetworkPolicyPeer{
			IPBlock: &v1.IPBlock{
				CIDR: cidrStr,
			},
		}
	})

	return peers, nil
}

func getCIDR(ipStr string, groupBySubnet bool) (*net.IPNet, error) {
	cidr := ipStr
	if !strings.Contains(ipStr, "/") {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			return nil, errors.New(fmt.Sprintf("invalid IP: %s", ipStr))
		}
		isV6 := ip.To4() == nil

		if isV6 {
			// groupBySubnet currently not supported for ipv6
			cidr = fmt.Sprintf("%s/128", ip)
		} else {
			if groupBySubnet {
				cidr = fmt.Sprintf("%s/24", ip)
			} else {
				cidr = fmt.Sprintf("%s/32", ip)
			}
		}
	}

	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	return network, nil
}
