package webhook_traffic

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/automate_third_party_network_policy"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	policy automate_third_party_network_policy.Enum
}

func NewNetworkPolicyHandler(
	client client.Client,
	scheme *runtime.Scheme,
	policy automate_third_party_network_policy.Enum,
) *NetworkPolicyHandler {
	return &NetworkPolicyHandler{
		client: client,
		scheme: scheme,
		policy: policy,
	}
}

func (n *NetworkPolicyHandler) ReconcileAllValidatingWebhooksWebhooks(ctx context.Context) error {
	validatingWebhookConfigurationList := &admissionv1.ValidatingWebhookConfigurationList{}
	err := n.client.List(ctx, validatingWebhookConfigurationList)
	if err != nil {
		return errors.Wrap(err)
	}

	allClientConfigurations := make([]WebhookClientConfigurationWithMeta, 0)
	for _, webhookConfiguration := range validatingWebhookConfigurationList.Items {
		webhooksClientConfigurations := lo.Map(webhookConfiguration.Webhooks, func(webhook admissionv1.ValidatingWebhook, _ int) WebhookClientConfigurationWithMeta {
			return WebhookClientConfigurationWithMeta{clientConfiguration: webhook.ClientConfig, webhookName: webhookConfiguration.Name, webhook: &webhookConfiguration}
		})
		allClientConfigurations = append(allClientConfigurations, webhooksClientConfigurations...)
	}

	err = n.reconcileWebhooks(ctx, allClientConfigurations)
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

			// TODO: do we also need to create netpol for Otterize?
			netpol, err := n.buildNetworkPolicy(ctx, webhookClientConfig.webhookName, webhookClientConfig.clientConfiguration.Service, service)
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

func (n *NetworkPolicyHandler) buildNetworkPolicy(ctx context.Context, webhookName string, webhookService *admissionv1.ServiceReference, service *corev1.Service) (v1.NetworkPolicy, error) {
	policyName := fmt.Sprintf("webhook-%s-access-to-%s", strings.ToLower(webhookName), strings.ToLower(service.Name))

	controlPLaneIP, err := n.getControlPlaneIPs(ctx)
	if err != nil {
		return v1.NetworkPolicy{}, errors.Wrap(err)
	}

	rule := v1.NetworkPolicyIngressRule{}

	rule.From = append(rule.From, v1.NetworkPolicyPeer{
		IPBlock: &v1.IPBlock{
			// TODO: handle google cloud CIDR
			CIDR: fmt.Sprintf("%s/32", controlPLaneIP),
		},
	})

	newPolicy := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: service.Namespace,
			Labels: map[string]string{
				v2alpha1.OtterizeNetworkPolicyWebhooks: webhookName,
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

	if webhookService.Port != nil {
		newPolicy.Spec.Ingress[0].Ports = append(newPolicy.Spec.Ingress[0].Ports, v1.NetworkPolicyPort{
			Port:     lo.ToPtr(intstr.IntOrString{IntVal: *webhookService.Port, Type: intstr.Int}),
			Protocol: lo.ToPtr(corev1.ProtocolTCP),
		})
	}

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

func (n *NetworkPolicyHandler) getControlPlaneIPs(ctx context.Context) (string, error) {
	var svc corev1.Service
	err := n.client.Get(ctx, types.NamespacedName{Name: "kubernetes", Namespace: "default"}, &svc)
	if err != nil {
		return "", errors.Wrap(err)
	}

	// TODO: Do we also need to get the IPs of the service endpoints'?
	return svc.Spec.ClusterIP, nil
}

func (n *NetworkPolicyHandler) policiesAreEqual(policy *v1.NetworkPolicy, otherPolicy *v1.NetworkPolicy) bool {
	return reflect.DeepEqual(policy.Spec, otherPolicy.Spec) &&
		reflect.DeepEqual(policy.Labels, otherPolicy.Labels) &&
		reflect.DeepEqual(policy.Annotations, otherPolicy.Annotations)
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
	policyCopy.Annotations = newPolicy.Annotations
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
