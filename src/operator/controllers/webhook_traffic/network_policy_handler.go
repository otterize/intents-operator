package webhook_traffic

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/automate_third_party_network_policy"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"net"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

const (
	ReasonCreatingWebhookTrafficNetpol        = "CreatingNetworkPolicyForWebhook"
	ReasonCreatingWebhookTrafficNetpolFailed  = "CreatingNetworkPolicyForWebhookFailed"
	ReasonCreatingWebhookTrafficNetpolSuccess = "CreatingNetworkPolicyForWebhookSucceeded"
	ReasonPatchingWebhookTrafficNetpol        = "PatchingNetworkPolicyForWebhook"
	ReasonPatchingWebhookTrafficNetpolFailed  = "PatchingNetworkPolicyForWebhookFailed"
	ReasonPatchingWebhookTrafficNetpolSuccess = "PatchingNetworkPolicyForWebhookSucceeded"
)

type WebhookClientConfigurationWithMeta struct {
	clientConfiguration admissionv1.WebhookClientConfig
	webhookName         string
	webhook             runtime.Object
}
type NetworkPolicyWithMeta struct {
	policy      *v1.NetworkPolicy
	serviceName string
	webhook     runtime.Object
}
type NetworkPolicyWithMetaByName map[string]*NetworkPolicyWithMeta
type NetworkPolicyByName map[string]*v1.NetworkPolicy

type NetworkPolicyHandler struct {
	client client.Client
	scheme *runtime.Scheme
	injectablerecorder.InjectableRecorder
	policy                       automate_third_party_network_policy.Enum
	controlPlaneCIDRPrefixLength int
	allowAllIncomingTraffic      bool
}

func NewNetworkPolicyHandler(
	client client.Client,
	scheme *runtime.Scheme,
	policy automate_third_party_network_policy.Enum,
	controlPlaneCIDRPrefixLength int,
	allowAllIncomingTraffic bool,
) *NetworkPolicyHandler {
	return &NetworkPolicyHandler{
		client:                       client,
		scheme:                       scheme,
		policy:                       policy,
		controlPlaneCIDRPrefixLength: controlPlaneCIDRPrefixLength,
		allowAllIncomingTraffic:      allowAllIncomingTraffic,
	}
}

func (n *NetworkPolicyHandler) HandleAll(ctx context.Context) error {
	validatingWebhooks, err := n.collectValidatingWebhooks(ctx)
	if err != nil {
		return errors.Wrap(err)
	}

	mutatingWebhooks, err := n.collectMutatingWebhooks(ctx)
	if err != nil {
		return errors.Wrap(err)
	}

	crdsWebhooks, err := n.collectCRDsWebhooks(ctx)
	if err != nil {
		return errors.Wrap(err)
	}

	allWebhooks := make([]WebhookClientConfigurationWithMeta, 0, len(validatingWebhooks)+len(mutatingWebhooks)+len(crdsWebhooks))
	allWebhooks = append(allWebhooks, validatingWebhooks...)
	allWebhooks = append(allWebhooks, mutatingWebhooks...)
	allWebhooks = append(allWebhooks, crdsWebhooks...)

	err = n.reconcileWebhooks(ctx, allWebhooks)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (n *NetworkPolicyHandler) reconcileWebhooks(ctx context.Context, webhooksClientConfigs []WebhookClientConfigurationWithMeta) error {
	reducedPolicies, err := n.reduceWebhooksNetpols(ctx, webhooksClientConfigs)
	if err != nil {
		return errors.Wrap(err)
	}

	existingPolicies, err := n.getExistingWebhooksNetpols(ctx)
	if err != nil {
		return errors.Wrap(err)
	}

	reducedPoliciesNames := lo.Keys(reducedPolicies)
	currentNetworkPoliciesName := lo.Keys(existingPolicies)

	policiesToAdd, policiesToDelete := lo.Difference(reducedPoliciesNames, currentNetworkPoliciesName)
	policiesToUpdate := lo.Intersect(reducedPoliciesNames, currentNetworkPoliciesName)

	err = n.handlePoliciesToAdd(ctx, policiesToAdd, reducedPolicies)
	if err != nil {
		return errors.Wrap(err)
	}

	err = n.handlePoliciesToDelete(ctx, policiesToDelete, existingPolicies)
	if err != nil {
		return errors.Wrap(err)
	}

	err = n.handlePoliciesToUpdate(ctx, policiesToUpdate, reducedPolicies)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (n *NetworkPolicyHandler) reduceWebhooksNetpols(ctx context.Context, webhooksClientConfigs []WebhookClientConfigurationWithMeta) (NetworkPolicyWithMetaByName, error) {
	if n.policy == automate_third_party_network_policy.Off {
		// If the configuration is off, we don't want to create any network policies
		return make(NetworkPolicyWithMetaByName), nil
	}

	policiesByName := make(NetworkPolicyWithMetaByName)
	for _, webhookClientConfig := range webhooksClientConfigs {
		if webhookClientConfig.clientConfiguration.Service != nil {
			service, serviceFound, err := n.getWebhookService(ctx, webhookClientConfig.clientConfiguration.Service)
			if err != nil {
				return make(NetworkPolicyWithMetaByName), errors.Wrap(err)
			}

			if !serviceFound {
				continue
			}

			if n.policy == automate_third_party_network_policy.IfBlockedByOtterize {
				blockedByOtterize, err := n.isServiceBlockedByOtterize(ctx, service)
				if err != nil {
					return make(NetworkPolicyWithMetaByName), errors.Wrap(err)
				}
				if !blockedByOtterize {
					continue
				}
			}

			// At this point we want to create the network policy, because the configuration is either set to "always" or
			// that it is set to "if blocked by otterize" and the service is blocked by otterize
			// TODO: do we also need to create netpol for Otterize?
			netpol, err := n.buildNetworkPolicy(ctx, webhookClientConfig.webhookName, service)
			if err != nil {
				return make(NetworkPolicyWithMetaByName), errors.Wrap(err)
			}

			existingNetpol, foundExistingNetpol := policiesByName[netpol.Name]
			if foundExistingNetpol {
				// merge existing netpol with the new one
				netpol.Spec.Ingress[0].Ports = append(netpol.Spec.Ingress[0].Ports, existingNetpol.policy.Spec.Ingress[0].Ports...)
			}

			policiesByName[netpol.Name] = &NetworkPolicyWithMeta{serviceName: webhookClientConfig.clientConfiguration.Service.Name, policy: &netpol, webhook: webhookClientConfig.webhook}
		}
	}

	// we want to make sure that the ports of the netpol will appear in the same order, for later policies comparison
	n.dedupAndSortPorts(policiesByName)

	return policiesByName, nil
}

func (n *NetworkPolicyHandler) isServiceBlockedByOtterize(ctx context.Context, service *corev1.Service) (bool, error) {
	endpoints := &corev1.Endpoints{}
	err := n.client.Get(ctx, types.NamespacedName{Namespace: service.Namespace, Name: service.Name}, endpoints)
	if err != nil {
		return false, errors.Wrap(err)
	}

	endpointsAddresses := n.getAddressesFromEndpoints(endpoints)

	for _, address := range endpointsAddresses {
		pod, err := n.getAffectedPod(ctx, address)
		if k8sErr := &(k8serrors.StatusError{}); errors.As(err, &k8sErr) {
			if k8serrors.IsNotFound(k8sErr) {
				continue
			}
		}

		if err != nil {
			return false, errors.Wrap(err)
		}

		// only act on pods affected by Otterize
		_, ok := pod.Labels[v2alpha1.OtterizeServiceLabelKey]
		if !ok {
			continue
		}
		netpolsAffectingThisWorkload := make([]v1.NetworkPolicy, 0)

		serviceId, err := serviceidresolver.NewResolver(n.client).ResolvePodToServiceIdentity(ctx, pod)
		if err != nil {
			return false, errors.Wrap(err)
		}
		netpolList := &v1.NetworkPolicyList{}

		// Get netpolsAffectingThisWorkload which was created by intents targeting this pod by its owner with "kind"
		err = n.client.List(ctx, netpolList, client.MatchingLabels{v2alpha1.OtterizeNetworkPolicy: serviceId.GetFormattedOtterizeIdentityWithKind()})
		if err != nil {
			return false, errors.Wrap(err)
		}
		netpolsAffectingThisWorkload = append(netpolsAffectingThisWorkload, netpolList.Items...)

		// Get netpolsAffectingThisWorkload which was created by intents targeting this pod by its owner without "kind"
		err = n.client.List(ctx, netpolList, client.MatchingLabels{v2alpha1.OtterizeNetworkPolicy: serviceId.GetFormattedOtterizeIdentityWithoutKind()})
		if err != nil {
			return false, errors.Wrap(err)
		}
		netpolsAffectingThisWorkload = append(netpolsAffectingThisWorkload, netpolList.Items...)

		// Get netpolsAffectingThisWorkload which was created by intents targeting this pod by its service
		err = n.client.List(ctx, netpolList, client.MatchingLabels{v2alpha1.OtterizeNetworkPolicy: (&serviceidentity.ServiceIdentity{Name: endpoints.Name, Namespace: endpoints.Namespace, Kind: serviceidentity.KindService}).GetFormattedOtterizeIdentityWithKind()})
		if err != nil {
			return false, errors.Wrap(err)
		}
		netpolsAffectingThisWorkload = append(netpolsAffectingThisWorkload, netpolList.Items...)

		blockedByOtterize := lo.SomeBy(netpolsAffectingThisWorkload, func(netpol v1.NetworkPolicy) bool {
			return lo.Contains(netpol.Spec.PolicyTypes, v1.PolicyTypeIngress)
		})

		if blockedByOtterize {
			return true, nil
		}
	}

	return false, nil
}

func (n *NetworkPolicyHandler) getAffectedPod(ctx context.Context, address corev1.EndpointAddress) (*corev1.Pod, error) {
	if address.TargetRef == nil || address.TargetRef.Kind != "Pod" {
		return nil, k8serrors.NewNotFound(corev1.Resource("Pod"), "not-a-pod")
	}

	pod := &corev1.Pod{}
	err := n.client.Get(ctx, types.NamespacedName{Name: address.TargetRef.Name, Namespace: address.TargetRef.Namespace}, pod)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	return pod, nil
}

func (n *NetworkPolicyHandler) getAddressesFromEndpoints(endpoints *corev1.Endpoints) []corev1.EndpointAddress {
	addresses := make([]corev1.EndpointAddress, 0)
	for _, subset := range endpoints.Subsets {
		addresses = append(addresses, subset.Addresses...)
		addresses = append(addresses, subset.NotReadyAddresses...)

	}
	return addresses
}

func (n *NetworkPolicyHandler) dedupAndSortPorts(policiesByNames NetworkPolicyWithMetaByName) {
	for _, netpol := range policiesByNames {
		uniquePorts := lo.UniqBy(netpol.policy.Spec.Ingress[0].Ports, func(a v1.NetworkPolicyPort) string {
			return a.Port.String()
		})
		slices.SortFunc(uniquePorts, func(a, b v1.NetworkPolicyPort) bool {
			return a.Port.IntVal < b.Port.IntVal
		})
		netpol.policy.Spec.Ingress[0].Ports = uniquePorts
	}
}

func (n *NetworkPolicyHandler) getWebhookService(ctx context.Context, webhookService *admissionv1.ServiceReference) (*corev1.Service, bool, error) {
	service := &corev1.Service{}
	err := n.client.Get(ctx, types.NamespacedName{Name: webhookService.Name, Namespace: webhookService.Namespace}, service)
	if k8serrors.IsNotFound(err) {
		// This is ok, the service might not exist yet, but the reconcile will be called when it does
		return nil, false, nil
	}

	if err != nil {
		return nil, false, errors.Wrap(err)
	}

	return service, true, nil
}

func (n *NetworkPolicyHandler) buildNetworkPolicy(ctx context.Context, webhookName string, service *corev1.Service) (v1.NetworkPolicy, error) {
	policyName := fmt.Sprintf("webhook-%s-access-to-%s", strings.ToLower(webhookName), strings.ToLower(service.Name))
	rule := v1.NetworkPolicyIngressRule{}

	if !n.allowAllIncomingTraffic {
		controlPlaneIPs, err := n.getControlPlaneIPsAsCIDR(ctx)
		if err != nil {
			return v1.NetworkPolicy{}, errors.Wrap(err)
		}

		fromControlPlaneIPs := lo.Map(controlPlaneIPs, func(controlPLaneIP string, _ int) v1.NetworkPolicyPeer {
			return v1.NetworkPolicyPeer{
				IPBlock: &v1.IPBlock{
					CIDR: controlPLaneIP,
				},
			}
		})

		rule.From = append(rule.From, fromControlPlaneIPs...)
	}

	labelValue := webhookName
	if len(labelValue) > 63 {
		labelValue = labelValue[:63]
	}

	newPolicy := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: service.Namespace,
			Labels: map[string]string{
				v2alpha1.OtterizeNetworkPolicyWebhooks: labelValue,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: service.Spec.Selector,
			},
			Ingress: []v1.NetworkPolicyIngressRule{
				rule,
			},
		},
	}

	netpolPorts := make([]v1.NetworkPolicyPort, 0, 2*len(service.Spec.Ports))

	for _, servicePort := range service.Spec.Ports {
		// Add the port
		netpolPorts = append(netpolPorts,
			v1.NetworkPolicyPort{
				Port:     lo.ToPtr(intstr.IntOrString{IntVal: servicePort.Port, Type: intstr.Int}),
				Protocol: lo.ToPtr(servicePort.Protocol),
			})
		// Add the target port
		if servicePort.TargetPort.IntVal != 0 || servicePort.TargetPort.StrVal != "" {
			netpolPorts = append(netpolPorts,
				v1.NetworkPolicyPort{
					Port:     lo.ToPtr(servicePort.TargetPort),
					Protocol: lo.ToPtr(servicePort.Protocol),
				})
		}
	}

	newPolicy.Spec.Ingress[0].Ports = netpolPorts

	return newPolicy, nil
}

func (n *NetworkPolicyHandler) getExistingWebhooksNetpols(ctx context.Context) (NetworkPolicyByName, error) {
	otterizeWebhookNetpolSelector, err := metav1.LabelSelectorAsSelector(
		&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      v2alpha1.OtterizeNetworkPolicyWebhooks,
				Operator: metav1.LabelSelectorOpExists,
			},
		},
		})

	if err != nil {
		return make(NetworkPolicyByName), errors.Wrap(err)
	}

	networkPoliciesList := &v1.NetworkPolicyList{}
	err = n.client.List(ctx, networkPoliciesList, client.MatchingLabelsSelector{Selector: otterizeWebhookNetpolSelector})
	if err != nil {
		return make(NetworkPolicyByName), errors.Wrap(err)
	}

	networkPolicies := make(NetworkPolicyByName)
	for _, netpol := range networkPoliciesList.Items {
		networkPolicies[netpol.Name] = &netpol
	}

	return networkPolicies, nil
}

func (n *NetworkPolicyHandler) getControlPlaneIPsAsCIDR(ctx context.Context) ([]string, error) {
	var svc corev1.Service
	err := n.client.Get(ctx, types.NamespacedName{Name: "kubernetes", Namespace: "default"}, &svc)
	if err != nil {
		return make([]string, 0), errors.Wrap(err)
	}

	addresses := make([]string, 0)
	for _, clusterIP := range svc.Spec.ClusterIPs {
		ip, isIP := n.ipAddressToCIDR(clusterIP)
		if isIP {
			addresses = append(addresses, ip)
		}
	}

	var endpoints corev1.Endpoints
	err = n.client.Get(ctx, types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, &endpoints)
	if err != nil {
		return make([]string, 0), errors.Wrap(err)
	}

	for _, subset := range endpoints.Subsets {
		for _, endpointAddress := range subset.Addresses {
			ip, isIP := n.ipAddressToCIDR(endpointAddress.IP)
			if isIP {
				addresses = append(addresses, ip)
			}
		}
	}

	return addresses, nil
}

func (n *NetworkPolicyHandler) ipAddressToCIDR(ipAddress string) (string, bool) {
	ip := net.ParseIP(ipAddress)
	if ip == nil {
		return "", false
	}

	if ip.To4() != nil {
		return fmt.Sprintf("%s/%d", ipAddress, n.controlPlaneCIDRPrefixLength), true
	}
	// The address is IPv6, we currently support configurable CIDR prefix length only for IPv4
	return fmt.Sprintf("%s/128", ipAddress), true

}

func (n *NetworkPolicyHandler) policiesAreEqual(policy *v1.NetworkPolicy, otherPolicy *v1.NetworkPolicy) bool {
	return reflect.DeepEqual(policy.Spec, otherPolicy.Spec) &&
		reflect.DeepEqual(policy.Labels, otherPolicy.Labels)
}

func (n *NetworkPolicyHandler) handlePoliciesToAdd(ctx context.Context, policiesNamesToAdd []string, policiesByName NetworkPolicyWithMetaByName) error {
	for _, policyName := range policiesNamesToAdd {
		serviceName := policiesByName[policyName].serviceName
		webhookRuntime := policiesByName[policyName].webhook
		n.RecordNormalEventf(policiesByName[policyName].webhook, ReasonCreatingWebhookTrafficNetpol, "Creating network policy for serivice %s", serviceName)

		err := n.client.Create(ctx, policiesByName[policyName].policy)
		if err != nil {
			if k8serrors.IsAlreadyExists(err) {
				existingPolicy := &v1.NetworkPolicy{}
				err = n.client.Get(ctx, types.NamespacedName{Name: policiesByName[policyName].policy.Name, Namespace: policiesByName[policyName].policy.Namespace}, existingPolicy)
				if err != nil {
					n.RecordWarningEventf(webhookRuntime, ReasonCreatingWebhookTrafficNetpolFailed, "Creating network policy for webhook for serivice %s failed with error: %s", serviceName, err.Error())
					return errors.Wrap(err) // We could not add and update, so just give up
				}
				return n.updatePolicy(ctx, webhookRuntime, existingPolicy, policiesByName[policyName].policy)
			}

			n.RecordWarningEventf(webhookRuntime, ReasonCreatingWebhookTrafficNetpolFailed, "Creating network policy for webhook for serivice %s failed with error: %s", serviceName, err.Error())
			return errors.Wrap(err)
		}

		n.RecordNormalEventf(webhookRuntime, ReasonCreatingWebhookTrafficNetpolSuccess, "Creating network policy for serivice %s succeed", serviceName)
	}

	return nil
}

func (n *NetworkPolicyHandler) handlePoliciesToDelete(ctx context.Context, policiesNamesToDelete []string, policiesByName NetworkPolicyByName) error {
	for _, policyName := range policiesNamesToDelete {
		err := n.client.Delete(ctx, policiesByName[policyName])
		if k8serrors.IsNotFound(err) {
			continue
		}

		if err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (n *NetworkPolicyHandler) handlePoliciesToUpdate(ctx context.Context, policiesNames []string, policiesByName NetworkPolicyWithMetaByName) error {
	for _, policyName := range policiesNames {
		webhookRuntime := policiesByName[policyName].webhook

		existingPolicy := &v1.NetworkPolicy{}
		err := n.client.Get(ctx, types.NamespacedName{Name: policyName, Namespace: policiesByName[policyName].policy.Namespace}, existingPolicy)

		// No matching network policy found to update, try to create one
		if k8serrors.IsNotFound(err) {
			err = n.handlePoliciesToAdd(ctx, []string{policyName}, policiesByName)
			if err != nil {
				return errors.Wrap(err)
			}
			continue
		}

		if err != nil {
			return errors.Wrap(err)
		}

		// Found existing matching policy, if it is identical to this one - do nothing
		if n.policiesAreEqual(existingPolicy, policiesByName[policyName].policy) {
			continue
		}

		err = n.updatePolicy(ctx, webhookRuntime, existingPolicy, policiesByName[policyName].policy)
		if err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (n *NetworkPolicyHandler) updatePolicy(ctx context.Context, webhookRuntime runtime.Object, existingPolicy *v1.NetworkPolicy, newPolicy *v1.NetworkPolicy) error {
	policyCopy := existingPolicy.DeepCopy()
	policyCopy.Labels = newPolicy.Labels
	policyCopy.Spec = newPolicy.Spec

	serviceName := policyCopy.Spec.PodSelector.MatchLabels["app"]
	n.RecordNormalEventf(webhookRuntime, ReasonPatchingWebhookTrafficNetpol, "Patching network policy for webhook for serivce %s", serviceName)

	err := n.client.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy))
	if err != nil {
		n.RecordWarningEventf(webhookRuntime, ReasonPatchingWebhookTrafficNetpolFailed, "Patching network policy for webhook for serivce %s failed with error: %s", serviceName, err.Error())
		return errors.Wrap(err)
	}

	n.RecordNormalEventf(webhookRuntime, ReasonPatchingWebhookTrafficNetpolSuccess, "Patching network policy for webhook for serivce %s succeed", serviceName)

	return nil
}

func (n *NetworkPolicyHandler) collectValidatingWebhooks(ctx context.Context) ([]WebhookClientConfigurationWithMeta, error) {
	validatingWebhookConfigurationList := &admissionv1.ValidatingWebhookConfigurationList{}
	err := n.client.List(ctx, validatingWebhookConfigurationList)
	if err != nil {
		return make([]WebhookClientConfigurationWithMeta, 0), errors.Wrap(err)
	}

	allClientConfigurations := make([]WebhookClientConfigurationWithMeta, 0)
	for _, webhookConfiguration := range validatingWebhookConfigurationList.Items {
		webhooksClientConfigurations := lo.Map(webhookConfiguration.Webhooks, func(webhook admissionv1.ValidatingWebhook, _ int) WebhookClientConfigurationWithMeta {
			return WebhookClientConfigurationWithMeta{clientConfiguration: webhook.ClientConfig, webhookName: webhookConfiguration.Name, webhook: &webhookConfiguration}
		})
		allClientConfigurations = append(allClientConfigurations, webhooksClientConfigurations...)
	}

	return allClientConfigurations, nil
}

func (n *NetworkPolicyHandler) collectMutatingWebhooks(ctx context.Context) ([]WebhookClientConfigurationWithMeta, error) {
	webhookConfigurationList := &admissionv1.MutatingWebhookConfigurationList{}
	err := n.client.List(ctx, webhookConfigurationList)
	if err != nil {
		return make([]WebhookClientConfigurationWithMeta, 0), errors.Wrap(err)
	}

	allClientConfigurations := make([]WebhookClientConfigurationWithMeta, 0)
	for _, webhookConfiguration := range webhookConfigurationList.Items {
		webhooksClientConfigurations := lo.Map(webhookConfiguration.Webhooks, func(webhook admissionv1.MutatingWebhook, _ int) WebhookClientConfigurationWithMeta {
			return WebhookClientConfigurationWithMeta{clientConfiguration: webhook.ClientConfig, webhookName: webhookConfiguration.Name, webhook: &webhookConfiguration}
		})
		allClientConfigurations = append(allClientConfigurations, webhooksClientConfigurations...)
	}

	return allClientConfigurations, nil
}

func (n *NetworkPolicyHandler) collectCRDsWebhooks(ctx context.Context) ([]WebhookClientConfigurationWithMeta, error) {
	crds := &apiextensionsv1.CustomResourceDefinitionList{}
	err := n.client.List(ctx, crds)
	if err != nil {
		return make([]WebhookClientConfigurationWithMeta, 0), errors.Wrap(err)
	}

	webhookdsCRDs := lo.Filter(crds.Items, func(crd apiextensionsv1.CustomResourceDefinition, _ int) bool {
		return crd.Spec.Conversion != nil && crd.Spec.Conversion.Webhook != nil && crd.Spec.Conversion.Webhook.ClientConfig != nil
	})

	allClientConfigurations := make([]WebhookClientConfigurationWithMeta, 0)
	for _, crd := range webhookdsCRDs {
		webhookConfig := WebhookClientConfigurationWithMeta{
			clientConfiguration: admissionv1.WebhookClientConfig{},
			webhookName:         crd.Name,
			webhook:             &crd,
		}

		if crd.Spec.Conversion.Webhook.ClientConfig.Service != nil {
			webhookConfig.clientConfiguration.Service =
				&admissionv1.ServiceReference{
					Name:      crd.Spec.Conversion.Webhook.ClientConfig.Service.Name,
					Namespace: crd.Spec.Conversion.Webhook.ClientConfig.Service.Namespace,
					Port:      crd.Spec.Conversion.Webhook.ClientConfig.Service.Port,
				}
		}

		allClientConfigurations = append(allClientConfigurations, webhookConfig)
	}

	return allClientConfigurations, nil
}
