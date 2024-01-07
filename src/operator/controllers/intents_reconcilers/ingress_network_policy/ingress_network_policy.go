package ingress_network_policy

import (
	"context"
	goerrors "errors"
	"fmt"
	"github.com/amit7itz/goset"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/protected_services"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/prometheus"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/allowexternaltraffic"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetrysender"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type externalNetpolHandler interface {
	HandlePodsByLabelSelector(ctx context.Context, namespace string, labelSelector labels.Selector) error
	HandleBeforeAccessPolicyRemoval(ctx context.Context, accessPolicy *v1.NetworkPolicy) error
}

type KubernetesEvent struct {
	Reason  string
	Message string
}

type IngressNetpolEffectivePolicyReconciler struct {
	client.Client
	Scheme                      *runtime.Scheme
	extNetpolHandler            externalNetpolHandler
	RestrictToNamespaces        []string
	enableNetworkPolicyCreation bool
	enforcementDefaultState     bool
	allowExternalTraffic        allowexternaltraffic.Enum
	injectablerecorder.InjectableRecorder
}

func NewIngressNetpolEffectivePolicyReconciler(
	c client.Client,
	s *runtime.Scheme,
	extNetpolHandler externalNetpolHandler,
	restrictToNamespaces []string,
	enableNetworkPolicyCreation bool,
	enforcementDefaultState bool,
	allowExternalTraffic allowexternaltraffic.Enum,
) *IngressNetpolEffectivePolicyReconciler {
	return &IngressNetpolEffectivePolicyReconciler{
		Client:                      c,
		Scheme:                      s,
		extNetpolHandler:            extNetpolHandler,
		RestrictToNamespaces:        restrictToNamespaces,
		enableNetworkPolicyCreation: enableNetworkPolicyCreation,
		enforcementDefaultState:     enforcementDefaultState,
		allowExternalTraffic:        allowExternalTraffic,
	}
}

// ReconcileEffectivePolicies Gets current state of effective policies and returns number of network policies
func (r *IngressNetpolEffectivePolicyReconciler) ReconcileEffectivePolicies(ctx context.Context, eps []effectivepolicy.ServiceEffectivePolicy) (int, error) {
	currentPolicies := goset.NewSet[types.NamespacedName]()
	errorList := make([]error, 0)
	for _, ep := range eps {
		netpols, err := r.ApplyServiceEffectivePolicy(ctx, ep)
		if err != nil {
			errorList = append(errorList, errors.Wrap(err))
			continue
		}
		currentPolicies.Add(netpols...)
	}
	if len(errorList) > 0 {
		return 0, errors.Wrap(goerrors.Join(errorList...))
	}

	// remove policies that doesn't exist in the policy list
	err := r.removeNetworkPoliciesThatShouldNotExist(ctx, currentPolicies)
	if err != nil {
		return currentPolicies.Len(), errors.Wrap(err)
	}

	if currentPolicies.Len() != 0 {
		telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesCreated, currentPolicies.Len())
	}
	return currentPolicies.Len(), nil
}

// ApplyServiceEffectivePolicy - reconcile ingress netpols for a service. returns the list of policies' namespaced names
func (r *IngressNetpolEffectivePolicyReconciler) ApplyServiceEffectivePolicy(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]types.NamespacedName, error) {
	shouldCreatePolicy, err := protected_services.IsServerEnforcementEnabledDueToProtectionOrDefaultState(ctx, r.Client, ep.Service.Name, ep.Service.Namespace, r.enforcementDefaultState)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	networkPolicies := make([]types.NamespacedName, 0)
	for _, intentCall := range ep.CalledBy {
		if !shouldCreatePolicy {
			logrus.Infof("Enforcement is disabled globally and server is not explicitly protected, skipping network policy creation for server %s in namespace %s", ep.Service.Name, ep.Service.Namespace)
			intentCall.ObjectEventRecorder.RecordNormalEventf(consts.ReasonEnforcementDefaultOff, "Enforcement is disabled globally and called service '%s' is not explicitly protected using a ProtectedService resource, network policy creation skipped", ep.Service.Name)
			continue
		}
		if !r.enableNetworkPolicyCreation {
			logrus.Infof("Network policy creation is disabled, skipping network policy creation for server %s in namespace %s", ep.Service.Name, ep.Service.Namespace)
			intentCall.ObjectEventRecorder.RecordNormalEvent(consts.ReasonNetworkPolicyCreationDisabled, "Network policy creation is disabled, creation skipped")
			continue
		}
		if len(r.RestrictToNamespaces) != 0 && !lo.Contains(r.RestrictToNamespaces, ep.Service.Namespace) {
			// Namespace is not in list of namespaces we're allowed to act in, so drop it.
			intentCall.ObjectEventRecorder.RecordWarningEventf(consts.ReasonNamespaceNotAllowed, "namespace %s was specified in intent, but is not allowed by configuration", ep.Service.Namespace)
			continue
		}

		logrus.Debugf("Server %s in namespace %s is in protected list: %t", ep.Service.Name, ep.Service.Namespace, shouldCreatePolicy)

		policyName := fmt.Sprintf(otterizev1alpha3.OtterizeNetworkPolicyNameTemplate, intentCall.IntendedCall.GetTargetServerName(), intentCall.Service.Namespace)
		existingPolicy := &v1.NetworkPolicy{}
		newPolicy := r.buildNetworkPolicyObjectForIntent(intentCall.IntendedCall, policyName, intentCall.Service.Namespace)
		err = r.Get(ctx, types.NamespacedName{
			Name:      policyName,
			Namespace: ep.Service.Namespace},
			existingPolicy)
		if err != nil && !k8serrors.IsNotFound(err) {
			r.RecordWarningEventf(existingPolicy, consts.ReasonGettingNetworkPolicyFailed, "failed to get network policy: %s", err.Error())
			return networkPolicies, errors.Wrap(err)
		}

		if k8serrors.IsNotFound(err) {
			err = r.createNetworkPolicy(ctx, intentCall.Service.Namespace, intentCall.IntendedCall, newPolicy)
			if err != nil {
				return networkPolicies, errors.Wrap(err)
			}
			networkPolicies = append(networkPolicies, types.NamespacedName{Name: newPolicy.Name, Namespace: newPolicy.Namespace})
			prometheus.IncrementNetpolCreated(1)
			intentCall.ObjectEventRecorder.RecordNormalEventf(consts.ReasonCreatedNetworkPolicies, "NetworkPolicy created for %s", intentCall.IntendedCall.GetTargetServerName())
			continue
		}
		changed, err := r.updateExistingPolicy(ctx, existingPolicy, newPolicy)
		if err != nil {
			return networkPolicies, errors.Wrap(err)
		}
		if changed {
			intentCall.ObjectEventRecorder.RecordNormalEventf(consts.ReasonCreatedNetworkPolicies, "NetworkPolicy created for %s", intentCall.IntendedCall.GetTargetServerName())
		}
		networkPolicies = append(networkPolicies, types.NamespacedName{Name: newPolicy.Name, Namespace: newPolicy.Namespace})
	}
	return networkPolicies, nil
}

func (r *IngressNetpolEffectivePolicyReconciler) updateExistingPolicy(ctx context.Context, existingPolicy *v1.NetworkPolicy, newPolicy *v1.NetworkPolicy) (bool, error) {
	if !reflect.DeepEqual(existingPolicy.Spec, newPolicy.Spec) {
		policyCopy := existingPolicy.DeepCopy()
		policyCopy.Labels = newPolicy.Labels
		policyCopy.Annotations = newPolicy.Annotations
		policyCopy.Spec = newPolicy.Spec

		err := r.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy))
		if err != nil {
			return true, errors.Wrap(err)
		}
		return true, nil
	}

	return false, nil
}

func (r *IngressNetpolEffectivePolicyReconciler) createNetworkPolicy(ctx context.Context, intentsObjNamespace string, intent otterizev1alpha3.Intent, newPolicy *v1.NetworkPolicy) error {
	logrus.Infof(
		"Creating network policy to enable access from namespace %s to %s", intentsObjNamespace, intent.Name)
	err := r.Create(ctx, newPolicy)
	if err != nil {
		return errors.Wrap(err)
	}

	return r.reconcileEndpointsForPolicy(ctx, newPolicy)
}

func (r *IngressNetpolEffectivePolicyReconciler) reconcileEndpointsForPolicy(ctx context.Context, newPolicy *v1.NetworkPolicy) error {
	selector, err := metav1.LabelSelectorAsSelector(&newPolicy.Spec.PodSelector)
	if err != nil {
		return errors.Wrap(err)
	}
	// Use the external netpolHandler to check if pods got affected and if so, if they need external allow policies
	return r.extNetpolHandler.HandlePodsByLabelSelector(ctx, newPolicy.Namespace, selector)
}

func (r *IngressNetpolEffectivePolicyReconciler) removeNetworkPoliciesThatShouldNotExist(ctx context.Context, netpolNamesThatShouldExist *goset.Set[types.NamespacedName]) error {
	logrus.Info("Searching for orphaned network policies")
	networkPolicyList := &v1.NetworkPolicyList{}
	selector, err := matchAccessNetworkPolicy()
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

func (r *IngressNetpolEffectivePolicyReconciler) removeNetworkPolicy(ctx context.Context, networkPolicy v1.NetworkPolicy) error {
	err := r.extNetpolHandler.HandleBeforeAccessPolicyRemoval(ctx, &networkPolicy)
	if err != nil {
		return errors.Wrap(err)
	}
	err = r.Delete(ctx, &networkPolicy)
	if err != nil {
		return errors.Wrap(err)
	}
	return nil
}

func matchAccessNetworkPolicy() (labels.Selector, error) {
	isOtterizeNetworkPolicy := metav1.LabelSelectorRequirement{
		Key:      otterizev1alpha3.OtterizeNetworkPolicy,
		Operator: metav1.LabelSelectorOpExists,
	}
	isNotExternalTrafficPolicy := metav1.LabelSelectorRequirement{
		Key:      otterizev1alpha3.OtterizeNetworkPolicyExternalTraffic,
		Operator: metav1.LabelSelectorOpDoesNotExist,
	}
	isNotDefaultDenyPolicy := metav1.LabelSelectorRequirement{
		Key:      otterizev1alpha3.OtterizeNetworkPolicyServiceDefaultDeny,
		Operator: metav1.LabelSelectorOpDoesNotExist,
	}
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		isOtterizeNetworkPolicy,
		isNotExternalTrafficPolicy,
		isNotDefaultDenyPolicy,
	}})
}

// buildNetworkPolicyObjectForIntent builds the network policy that represents the intent from the parameter
func (r *IngressNetpolEffectivePolicyReconciler) buildNetworkPolicyObjectForIntent(
	intent otterizev1alpha3.Intent, policyName, intentsObjNamespace string) *v1.NetworkPolicy {
	targetNamespace := intent.GetTargetServerNamespace(intentsObjNamespace)
	// The intent's target server made of name + namespace + hash
	formattedTargetServer := otterizev1alpha3.GetFormattedOtterizeIdentity(intent.GetTargetServerName(), targetNamespace)
	podSelector := r.buildPodLabelSelectorFromIntent(intent, intentsObjNamespace)
	return &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: targetNamespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeNetworkPolicy: formattedTargetServer,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: podSelector,
			Ingress: []v1.NetworkPolicyIngressRule{
				{
					From: []v1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									fmt.Sprintf(
										otterizev1alpha3.OtterizeAccessLabelKey, formattedTargetServer): "true",
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									otterizev1alpha3.KubernetesStandardNamespaceNameLabelKey: intentsObjNamespace,
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *IngressNetpolEffectivePolicyReconciler) buildPodLabelSelectorFromIntent(intent otterizev1alpha3.Intent, intentsObjNamespace string) metav1.LabelSelector {
	targetNamespace := intent.GetTargetServerNamespace(intentsObjNamespace)
	// The intent's target server made of name + namespace + hash
	formattedTargetServer := otterizev1alpha3.GetFormattedOtterizeIdentity(intent.GetTargetServerName(), targetNamespace)

	return metav1.LabelSelector{
		MatchLabels: map[string]string{
			otterizev1alpha3.OtterizeServerLabelKey: formattedTargetServer,
		},
	}
}
