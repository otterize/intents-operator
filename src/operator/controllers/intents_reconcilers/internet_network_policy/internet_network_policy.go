package internet_network_policy

import (
	"context"
	goerrors "errors"
	"fmt"
	"github.com/amit7itz/goset"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/prometheus"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetrysender"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"net"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"slices"
	"strings"
)

// The InternetNetworkPolicyReconciler creates network policies that allow egress traffic from pods.
type InternetNetworkPolicyReconciler struct {
	client.Client
	Scheme                      *runtime.Scheme
	RestrictToNamespaces        []string
	enableNetworkPolicyCreation bool
	enforcementDefaultState     bool
	injectablerecorder.InjectableRecorder
}

func NewInternetNetworkPolicyReconciler(
	c client.Client,
	s *runtime.Scheme,
	restrictToNamespaces []string,
	enableNetworkPolicyCreation bool,
	enforcementDefaultState bool) *InternetNetworkPolicyReconciler {
	return &InternetNetworkPolicyReconciler{
		Client:                      c,
		Scheme:                      s,
		RestrictToNamespaces:        restrictToNamespaces,
		enableNetworkPolicyCreation: enableNetworkPolicyCreation,
		enforcementDefaultState:     enforcementDefaultState,
	}
}

func (r *InternetNetworkPolicyReconciler) ReconcileEffectivePolicies(ctx context.Context, eps []effectivepolicy.ServiceEffectivePolicy) (int, error) {
	currentPolicies := goset.NewSet[types.NamespacedName]()
	errorList := make([]error, 0)
	for _, ep := range eps {
		netpols, err := r.applyServiceEffectivePolicy(ctx, ep)
		if err != nil {
			errorList = append(errorList, errors.Wrap(err))
			continue
		}
		currentPolicies.Add(netpols...)
	}
	if len(errorList) > 0 {
		return 0, errors.Wrap(goerrors.Join(errorList...))
	}

	err := r.removeNetworkPoliciesThatShouldNotExist(ctx, currentPolicies)

	return currentPolicies.Len(), errors.Wrap(err)
}

func (r *InternetNetworkPolicyReconciler) handleNetworkPolicyCreation(
	ctx context.Context,
	ep effectivepolicy.ServiceEffectivePolicy,
) (v1.NetworkPolicy, error) {
	policyName := policyNameFor(ep.Service.Name)
	existingPolicy := &v1.NetworkPolicy{}
	newPolicy, err := r.buildNetworkPolicy(ep, policyName)
	if err != nil {
		return v1.NetworkPolicy{}, errors.Wrap(err)
	}

	err = r.Get(ctx, types.NamespacedName{
		Name:      policyName,
		Namespace: ep.Service.Namespace},
		existingPolicy)
	if err != nil && !k8serrors.IsNotFound(err) {
		r.RecordWarningEventf(existingPolicy, consts.ReasonGettingEgressNetworkPolicyFailed, "failed to get network policy: %s", err.Error())
		return v1.NetworkPolicy{}, errors.Wrap(err)
	}

	if k8serrors.IsNotFound(err) {
		err = r.CreateNetworkPolicy(ctx, newPolicy)
		if err != nil {
			return v1.NetworkPolicy{}, errors.Wrap(err)
		}
		return *newPolicy, nil
	}

	err = r.UpdateExistingPolicy(ctx, existingPolicy, newPolicy)
	if err != nil {
		return v1.NetworkPolicy{}, err
	}
	return *newPolicy, nil
}

func policyNameFor(clientName string) string {
	return fmt.Sprintf(otterizev1alpha3.OtterizeEgressNetworkPolicyNameTemplate, otterizev1alpha3.OtterizeInternetTargetName, clientName)
}

func (r *InternetNetworkPolicyReconciler) UpdateExistingPolicy(ctx context.Context, existingPolicy *v1.NetworkPolicy, newPolicy *v1.NetworkPolicy) error {
	if !reflect.DeepEqual(existingPolicy.Spec, newPolicy.Spec) {
		policyCopy := existingPolicy.DeepCopy()
		policyCopy.Labels = newPolicy.Labels
		policyCopy.Annotations = newPolicy.Annotations
		policyCopy.Spec = newPolicy.Spec

		err := r.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy))
		if err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (r *InternetNetworkPolicyReconciler) CreateNetworkPolicy(ctx context.Context, newPolicy *v1.NetworkPolicy) error {
	logrus.Infof("Creating internet network policy %s", newPolicy.Name)
	err := r.Create(ctx, newPolicy)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (r *InternetNetworkPolicyReconciler) removeNetworkPoliciesThatShouldNotExist(ctx context.Context, netpolNamesThatShouldExist *goset.Set[types.NamespacedName]) error {
	logrus.Info("Searching for orphaned network policies")
	networkPolicyList := &v1.NetworkPolicyList{}
	labelSelector := metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha3.OtterizeInternetNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
	}}
	selector, err := metav1.LabelSelectorAsSelector(&labelSelector)
	if err != nil {
		return errors.Wrap(err)
	}

	err = r.List(ctx, networkPolicyList, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		logrus.Infof("Error listing network policies: %s", err.Error())
		return errors.Wrap(err)
	}

	logrus.Infof("Selector: %s found %d network policies", selector.String(), len(networkPolicyList.Items))
	for _, networkPolicy := range networkPolicyList.Items {
		namespacedName := types.NamespacedName{Namespace: networkPolicy.Namespace, Name: networkPolicy.Name}
		if !netpolNamesThatShouldExist.Contains(namespacedName) {
			serverName := networkPolicy.Labels[otterizev1alpha3.OtterizeNetworkPolicy]
			logrus.Infof("Removing orphaned network policy: %s server %s ns %s", networkPolicy.Name, serverName, networkPolicy.Namespace)
			err = r.removeNetworkPolicy(ctx, networkPolicy)
			if err != nil {
				return errors.Wrap(err)
			}
		}
	}

	return nil
}

func (r *InternetNetworkPolicyReconciler) removeNetworkPolicy(ctx context.Context, networkPolicy v1.NetworkPolicy) error {
	err := r.Delete(ctx, &networkPolicy)
	if err != nil {
		return errors.Wrap(err)
	}
	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesDeleted, 1)
	prometheus.IncrementNetpolDeleted(1)

	return nil
}

func (r *InternetNetworkPolicyReconciler) buildNetworkPolicy(
	ep effectivepolicy.ServiceEffectivePolicy,
	policyName string,
) (*v1.NetworkPolicy, error) {
	// The intent's target server made of name + namespace + hash
	formattedClient := otterizev1alpha3.GetFormattedOtterizeIdentity(ep.Service.Name, ep.Service.Namespace)
	podSelector := r.buildPodLabelSelectorFromServiceEffectivePolicy(ep)

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

	return &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: ep.Service.Namespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeInternetNetworkPolicy: formattedClient,
			},
		},

		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			PodSelector: podSelector,
			Egress:      rules,
		},
	}, nil
}

func (r *InternetNetworkPolicyReconciler) buildRuleForIntent(intent otterizev1alpha3.Intent, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyPeer, []v1.NetworkPolicyPort, bool, error) {
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

func (r *InternetNetworkPolicyReconciler) getIpsForDNS(intent otterizev1alpha3.Intent, ep effectivepolicy.ServiceEffectivePolicy) []string {
	ipsFromDns := make([]string, 0)
	if intent.Internet.Dns == "" {
		return ipsFromDns
	}
	dnsResolvedIps, found := lo.Find(ep.Status.ResolvedIPs, func(resolvedIPs otterizev1alpha3.ResolvedIPs) bool {
		return resolvedIPs.DNS == intent.Internet.Dns
	})

	if !found {
		ep.ClientIntentsEventRecorder.RecordWarningEventf(consts.ReasonIntentToUnresolvedDns, "could not find IP for DNS %s", intent.Internet.Dns)
		return ipsFromDns
	}

	ipsFromDns = dnsResolvedIps.IPs
	return ipsFromDns
}

func (r *InternetNetworkPolicyReconciler) parseIps(ips []string) ([]v1.NetworkPolicyPeer, error) {
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

func (r *InternetNetworkPolicyReconciler) parsePorts(intent otterizev1alpha3.Intent) []v1.NetworkPolicyPort {
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

func (r *InternetNetworkPolicyReconciler) buildPodLabelSelectorFromServiceEffectivePolicy(ep effectivepolicy.ServiceEffectivePolicy) metav1.LabelSelector {
	// The intent's target server made of name + namespace + hash
	formattedClient := otterizev1alpha3.GetFormattedOtterizeIdentity(ep.Service.Name, ep.Service.Namespace)

	return metav1.LabelSelector{
		MatchLabels: map[string]string{
			otterizev1alpha3.OtterizeClientLabelKey: formattedClient,
		},
	}
}

// applyServiceEffectivePolicy - reconcile ingress netpols for a service. returns the list of policies' namespaced names
func (r *InternetNetworkPolicyReconciler) applyServiceEffectivePolicy(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]types.NamespacedName, error) {
	hasAnyInternetIntents := slices.ContainsFunc(ep.Calls, func(intent otterizev1alpha3.Intent) bool {
		return intent.Type == otterizev1alpha3.IntentTypeInternet
	})
	if !hasAnyInternetIntents {
		return make([]types.NamespacedName, 0), nil
	}

	logrus.Infof("Reconciling internet network policies for service %s in namespace %s",
		ep.Service.Name, ep.Service.Namespace)

	if len(r.RestrictToNamespaces) != 0 && !lo.Contains(r.RestrictToNamespaces, ep.Service.Namespace) {
		ep.ClientIntentsEventRecorder.RecordWarningEventf(consts.ReasonNamespaceNotAllowed, "ClientIntents are in namespace %s but namespace is not allowed by configuration", ep.Service.Namespace)
		return make([]types.NamespacedName, 0), nil
	}

	if !r.enforcementDefaultState {
		logrus.Infof("Enforcement is disabled globally skipping internet network policy creation for service %s in namespace %s", ep.Service.Name, ep.Service.Namespace)
		ep.ClientIntentsEventRecorder.RecordNormalEventf(consts.ReasonEnforcementDefaultOff, "Enforcement is disabled globally, internet network policy creation skipped")
		return make([]types.NamespacedName, 0), nil
	}
	if !r.enableNetworkPolicyCreation {
		logrus.Infof("Network policy creation is disabled, skipping internet network policy creation for service %s in namespace %s", ep.Service.Name, ep.Service.Namespace)
		ep.ClientIntentsEventRecorder.RecordNormalEventf(consts.ReasonEgressNetworkPolicyCreationDisabled, "Network policy creation is disabled, internet network policy creation skipped")
		return make([]types.NamespacedName, 0), nil
	}

	netpol, err := r.handleNetworkPolicyCreation(ctx, ep)
	if err != nil {
		ep.ClientIntentsEventRecorder.RecordWarningEventf(consts.ReasonCreatingEgressNetworkPoliciesFailed, "could not create network policies: %s", err.Error())
		return nil, errors.Wrap(err)
	}

	ep.ClientIntentsEventRecorder.RecordNormalEvent(consts.ReasonCreatedInternetEgressNetworkPolicies, "InternetNetworkPolicy reconcile complete")
	prometheus.IncrementNetpolCreated(1)
	return []types.NamespacedName{{Namespace: netpol.Namespace, Name: netpol.Name}}, nil
}
