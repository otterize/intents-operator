package networkpolicy

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
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetrysender"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type EgressRuleBuilder interface {
	Build(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyEgressRule, error)
	InjectRecorder(recorder record.EventRecorder)
}

type IngressRuleBuilder interface {
	Build(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyIngressRule, error)
	InjectRecorder(recorder record.EventRecorder)
}

type ExternalNetpolHandler interface {
	HandlePodsByLabelSelector(ctx context.Context, namespace string, labelSelector labels.Selector) error
	HandleBeforeAccessPolicyRemoval(ctx context.Context, accessPolicy *v1.NetworkPolicy) error
}

type Reconciler struct {
	client.Client
	Scheme                      *runtime.Scheme
	RestrictToNamespaces        []string
	EnableNetworkPolicyCreation bool
	EnforcementDefaultState     bool
	injectablerecorder.InjectableRecorder
	egressRuleBuilders  []EgressRuleBuilder
	ingressRuleBuilders []IngressRuleBuilder
	extNetpolHandler    ExternalNetpolHandler
}

func NewReconciler(
	c client.Client,
	s *runtime.Scheme,
	externalNetpolHandler ExternalNetpolHandler,
	restrictToNamespaces []string,
	enableNetworkPolicyCreation bool,
	enforcementDefaultState bool,
	ingressBuilders []IngressRuleBuilder,
	egressBuilders []EgressRuleBuilder) *Reconciler {

	return &Reconciler{
		Client:                      c,
		Scheme:                      s,
		RestrictToNamespaces:        restrictToNamespaces,
		EnableNetworkPolicyCreation: enableNetworkPolicyCreation,
		EnforcementDefaultState:     enforcementDefaultState,
		egressRuleBuilders:          egressBuilders,
		ingressRuleBuilders:         ingressBuilders,
		extNetpolHandler:            externalNetpolHandler,
	}
}

func (r *Reconciler) AddEgressRuleBuilder(builder EgressRuleBuilder) {
	r.egressRuleBuilders = append(r.egressRuleBuilders, builder)
}

func (r *Reconciler) AddIngressRuleBuilder(builder IngressRuleBuilder) {
	r.ingressRuleBuilders = append(r.ingressRuleBuilders, builder)
}

func (r *Reconciler) InjectRecorder(recorder record.EventRecorder) {
	r.Recorder = recorder
	for _, builder := range r.egressRuleBuilders {
		builder.InjectRecorder(recorder)
	}
	for _, builder := range r.ingressRuleBuilders {
		builder.InjectRecorder(recorder)
	}
}

// ReconcileEffectivePolicies Gets current state of effective policies and returns number of network policies
func (r *Reconciler) ReconcileEffectivePolicies(ctx context.Context, eps []effectivepolicy.ServiceEffectivePolicy) (int, []error) {
	currentPolicies := goset.NewSet[types.NamespacedName]()
	errorList := make([]error, 0)
	for _, ep := range eps {
		netpol, created, err := r.applyServiceEffectivePolicy(ctx, ep)
		if err != nil {
			errorList = append(errorList, errors.Wrap(err))
			continue
		}
		if created {
			currentPolicies.Add(netpol)
		}
	}
	if len(errorList) > 0 {
		return 0, errorList
	}

	// remove policies that doesn't exist in the policy list
	err := r.removeNetworkPoliciesThatShouldNotExist(ctx, currentPolicies)
	if err != nil {
		return currentPolicies.Len(), []error{errors.Wrap(err)}
	}

	if currentPolicies.Len() != 0 {
		telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesCreated, currentPolicies.Len())
	}
	return currentPolicies.Len(), nil
}

func (r *Reconciler) applyServiceEffectivePolicy(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) (types.NamespacedName, bool, error) {
	if !r.EnableNetworkPolicyCreation {
		logrus.Infof("Network policy creation is disabled, skipping network policy creation for service %s in namespace %s", ep.Service.Name, ep.Service.Namespace)
		if len(ep.Calls) > 0 && len(r.egressRuleBuilders) > 0 {
			ep.ClientIntentsEventRecorder.RecordNormalEventf(consts.ReasonEgressNetworkPolicyCreationDisabled, "Network policy creation is disabled, creation skipped")
		}
		if len(ep.CalledBy) > 0 && len(r.ingressRuleBuilders) > 0 {
			ep.RecordOnClientsNormalEventf(consts.ReasonNetworkPolicyCreationDisabled, "Network policy creation is disabled, creation skipped")
		}
		return types.NamespacedName{}, false, nil
	}
	netpol, shouldCreate, err := r.buildNetworkPolicy(ctx, ep)
	if err != nil {
		r.recordCreateFailedError(ep, err)
		return types.NamespacedName{}, false, errors.Wrap(err)
	}
	if !shouldCreate {
		return types.NamespacedName{}, shouldCreate, errors.Wrap(err)
	}
	existingPolicy := &v1.NetworkPolicy{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      netpol.Name,
		Namespace: ep.Service.Namespace},
		existingPolicy)
	if err != nil && !k8serrors.IsNotFound(err) {
		r.RecordWarningEventf(existingPolicy, consts.ReasonGettingNetworkPolicyFailed, "failed to get network policy: %s", err.Error())
		return types.NamespacedName{}, false, errors.Wrap(err)
	}
	if k8serrors.IsNotFound(err) {
		err = r.createNetworkPolicy(ctx, ep, netpol)
		if err != nil {
			r.recordCreateFailedError(ep, err)
			return types.NamespacedName{}, false, errors.Wrap(err)
		}
		return types.NamespacedName{Name: netpol.Name, Namespace: netpol.Namespace}, true, nil
	}

	err = r.updateExistingPolicy(ctx, ep, existingPolicy, netpol)

	if err != nil {
		r.recordCreateFailedError(ep, err)
		return types.NamespacedName{}, false, errors.Wrap(err)
	}

	return types.NamespacedName{Name: netpol.Name, Namespace: netpol.Namespace}, true, nil

}

func (r *Reconciler) recordCreateFailedError(ep effectivepolicy.ServiceEffectivePolicy, err error) {
	if len(ep.Calls) > 0 && len(r.egressRuleBuilders) > 0 {
		ep.ClientIntentsEventRecorder.RecordWarningEventf(consts.ReasonCreatingEgressNetworkPoliciesFailed, "NetworkPolicy creation failed: %s", err.Error())
	}
	if len(ep.CalledBy) > 0 && len(r.ingressRuleBuilders) > 0 {
		ep.RecordOnClientsWarningEventf(consts.ReasonCreatingNetworkPoliciesFailed, "NetworkPolicy creation failed: %s", err.Error())
	}
}

func (r *Reconciler) buildEgressRules(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyEgressRule, bool, error) {
	rules := make([]v1.NetworkPolicyEgressRule, 0)
	if len(ep.Calls) == 0 || len(r.egressRuleBuilders) == 0 {
		return rules, false, nil
	}
	if !r.EnforcementDefaultState {
		logrus.Infof("Enforcement is disabled globally skipping egress network policy creation for service %s in namespace %s", ep.Service.Name, ep.Service.Namespace)
		ep.ClientIntentsEventRecorder.RecordNormalEventf(consts.ReasonEnforcementDefaultOff, "Enforcement is disabled globally, network policy creation skipped")
		return rules, false, nil
	}
	if len(r.RestrictToNamespaces) != 0 && !lo.Contains(r.RestrictToNamespaces, ep.Service.Namespace) {
		// Namespace is not in list of namespaces we're allowed to act in, so drop it.
		ep.ClientIntentsEventRecorder.RecordWarningEventf(consts.ReasonNamespaceNotAllowed, "ClientIntents are in namespace %s but namespace is not allowed by configuration", ep.Service.Namespace)
		return rules, false, nil
	}
	errorList := make([]error, 0)
	for _, builder := range r.egressRuleBuilders {
		rule, err := builder.Build(ctx, ep)
		if err != nil {
			errorList = append(errorList, errors.Wrap(err))
			continue
		}
		rules = append(rules, rule...)
	}
	return rules, len(rules) > 0, errors.Wrap(goerrors.Join(errorList...))
}

func (r *Reconciler) buildIngressRules(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyIngressRule, bool, error) {
	rules := make([]v1.NetworkPolicyIngressRule, 0)
	if len(ep.CalledBy) == 0 || len(r.ingressRuleBuilders) == 0 {
		return rules, false, nil
	}
	shouldCreatePolicy, err := protected_services.IsServerEnforcementEnabledDueToProtectionOrDefaultState(ctx, r.Client, ep.Service.Name, ep.Service.Namespace, r.EnforcementDefaultState)
	if err != nil {
		return rules, false, errors.Wrap(err)
	}
	if !shouldCreatePolicy {
		logrus.Infof("Enforcement is disabled globally and server is not explicitly protected, skipping network policy creation for server %s in namespace %s", ep.Service.Name, ep.Service.Namespace)
		ep.RecordOnClientsNormalEventf(consts.ReasonEnforcementDefaultOff, "Enforcement is disabled globally and called service '%s' is not explicitly protected using a ProtectedService resource, network policy creation skipped", ep.Service.Name)
		return rules, false, nil
	}
	if len(r.RestrictToNamespaces) != 0 && !lo.Contains(r.RestrictToNamespaces, ep.Service.Namespace) {
		// Namespace is not in list of namespaces we're allowed to act in, so drop it.
		ep.RecordOnClientsWarningEventf(consts.ReasonNamespaceNotAllowed, "namespace %s was specified in intent, but is not allowed by configuration", ep.Service.Namespace)
		return rules, false, nil
	}
	errorList := make([]error, 0)
	for _, builder := range r.ingressRuleBuilders {
		rule, err := builder.Build(ctx, ep)
		if err != nil {
			errorList = append(errorList, errors.Wrap(err))
			continue
		}
		rules = append(rules, rule...)
	}
	return rules, len(rules) > 0, errors.Wrap(goerrors.Join(errorList...))
}

// A function that builds pod label selector from serviceEffectivePolicy
func (r *Reconciler) buildPodLabelSelectorFromServiceEffectivePolicy(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) (metav1.LabelSelector, bool, error) {
	if ep.Service.Kind == serviceidentity.KindService {
		svc := corev1.Service{}
		err := r.Get(ctx, types.NamespacedName{Name: ep.Service.Name, Namespace: ep.Service.Namespace}, &svc)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return metav1.LabelSelector{}, false, nil
			}
			return metav1.LabelSelector{}, false, errors.Wrap(err)
		}
		if svc.Spec.Selector == nil {
			return metav1.LabelSelector{}, false, fmt.Errorf("service %s/%s has no selector", svc.Namespace, svc.Name)
		}
		return metav1.LabelSelector{MatchLabels: svc.Spec.Selector}, true, nil
	}

	return metav1.LabelSelector{
		MatchLabels: map[string]string{
			otterizev1alpha3.OtterizeServiceLabelKey: ep.Service.GetFormattedOtterizeIdentity(),
		},
	}, true, nil
}

func (r *Reconciler) setNetworkPolicyOwnerReferenceIfNeeded(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy, netpol *v1.NetworkPolicy) error {
	if ep.Service.Kind != serviceidentity.KindService {
		return nil
	}
	svc := corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: ep.Service.Name, Namespace: ep.Service.Namespace}, &svc)
	if err != nil {
		return errors.Wrap(err)
	}
	return errors.Wrap(controllerutil.SetOwnerReference(&svc, netpol, r.Scheme))
}

func (r *Reconciler) buildNetworkPolicy(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) (*v1.NetworkPolicy, bool, error) {
	policyTypes := make([]v1.PolicyType, 0)
	egressRules, shouldCreateEgress, err := r.buildEgressRules(ctx, ep)
	if err != nil {
		return nil, false, errors.Wrap(err)
	}
	if shouldCreateEgress {
		policyTypes = append(policyTypes, v1.PolicyTypeEgress)
	}

	ingressRules, shouldCreateIngress, err := r.buildIngressRules(ctx, ep)
	if err != nil {
		return nil, false, errors.Wrap(err)
	}
	if shouldCreateIngress {
		policyTypes = append(policyTypes, v1.PolicyTypeIngress)
	}

	if !shouldCreateIngress && !shouldCreateEgress {
		return nil, false, nil
	}

	podSelector, shouldCreate, err := r.buildPodLabelSelectorFromServiceEffectivePolicy(ctx, ep)
	if err != nil {
		return nil, false, errors.Wrap(err)
	}
	if !shouldCreate {
		return nil, false, nil
	}

	policy := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(otterizev1alpha3.OtterizeSingleNetworkPolicyNameTemplate, ep.Service.GetNameWithKind()),
			Namespace: ep.Service.Namespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeNetworkPolicy: ep.Service.GetFormattedOtterizeIdentity(),
			},
		},
		Spec: v1.NetworkPolicySpec{
			PodSelector: podSelector,
			PolicyTypes: policyTypes,
			Egress:      egressRules,
			Ingress:     ingressRules,
		},
	}

	err = r.setNetworkPolicyOwnerReferenceIfNeeded(ctx, ep, &policy)
	if err != nil {
		return nil, false, errors.Wrap(err)
	}

	return &policy, true, nil
}

func (r *Reconciler) createNetworkPolicy(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy, netpol *v1.NetworkPolicy) error {
	err := r.Create(ctx, netpol)
	if err != nil {
		return errors.Wrap(err)
	}
	prometheus.IncrementNetpolCreated(1)
	if len(netpol.Spec.Ingress) > 0 {
		ep.RecordOnClientsNormalEventf(consts.ReasonCreatedNetworkPolicies, "NetworkPolicy created for %s", ep.Service.Name)
	}
	if len(netpol.Spec.Egress) > 0 {
		ep.ClientIntentsEventRecorder.RecordNormalEventf(consts.ReasonCreatedEgressNetworkPolicies, "Egress NetworkPolicy created for %s", ep.Service.Name)
	}
	return errors.Wrap(r.reconcileEndpointsForPolicy(ctx, netpol))
}

func (r *Reconciler) updateExistingPolicy(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy, existingPolicy *v1.NetworkPolicy, newPolicy *v1.NetworkPolicy) error {
	if reflect.DeepEqual(existingPolicy.Spec, newPolicy.Spec) {
		return nil
	}

	policyCopy := existingPolicy.DeepCopy()
	policyCopy.Labels = newPolicy.Labels
	policyCopy.Annotations = newPolicy.Annotations
	policyCopy.Spec = newPolicy.Spec

	err := r.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy))
	if err != nil {
		return errors.Wrap(err)
	}

	if len(newPolicy.Spec.Ingress) > 0 {
		ep.RecordOnClientsNormalEventf(consts.ReasonCreatedNetworkPolicies, "NetworkPolicy updated for %s", ep.Service.Name)
		err = r.reconcileEndpointsForPolicy(ctx, newPolicy)
		if err != nil {
			return errors.Wrap(err)
		}
	}
	if len(newPolicy.Spec.Egress) > 0 {
		ep.ClientIntentsEventRecorder.RecordNormalEventf(consts.ReasonCreatedEgressNetworkPolicies, "Egress NetworkPolicy updated for %s", ep.Service.Name)
	}

	return nil

}

func (r *Reconciler) removeNetworkPoliciesThatShouldNotExist(ctx context.Context, netpolNamesThatShouldExist *goset.Set[types.NamespacedName]) error {
	logrus.Info("Searching for orphaned network policies")
	networkPolicyList := &v1.NetworkPolicyList{}
	selector, err := matchAccessNetworkPolicy()
	if err != nil {
		return errors.Wrap(err)
	}

	err = r.List(ctx, networkPolicyList, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return errors.Wrap(err)
	}

	logrus.Debugf("Selector: %s found %d network policies", selector.String(), len(networkPolicyList.Items))
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

	deletedCount := len(networkPolicyList.Items) - netpolNamesThatShouldExist.Len()
	if deletedCount > 0 {
		telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesDeleted, deletedCount)
		prometheus.IncrementNetpolDeleted(deletedCount)
	}
	return errors.Wrap(r.removeDeprecatedNetworkPolicies(ctx))
}

func (r *Reconciler) removeNetworkPolicy(ctx context.Context, networkPolicy v1.NetworkPolicy) error {
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

func (r *Reconciler) removeDeprecatedNetworkPolicies(ctx context.Context) error {
	logrus.Debug("Searching for network policies with deprecated labels")
	deprecatedLabels := []string{otterizev1alpha3.OtterizeEgressNetworkPolicy, otterizev1alpha3.OtterizeSvcEgressNetworkPolicy, otterizev1alpha3.OtterizeInternetNetworkPolicy, otterizev1alpha3.OtterizeSvcNetworkPolicy}
	deletedCount := 0
	for _, label := range deprecatedLabels {
		networkPolicyList := &v1.NetworkPolicyList{}
		selectorRequirement := metav1.LabelSelectorRequirement{
			Key:      label,
			Operator: metav1.LabelSelectorOpExists,
		}
		selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
			selectorRequirement,
		}})

		if err != nil {
			return errors.Wrap(err)
		}

		err = r.List(ctx, networkPolicyList, &client.ListOptions{LabelSelector: selector})
		if err != nil {
			return errors.Wrap(err)
		}

		logrus.Debugf("Selector: %s found %d network policies", selector.String(), len(networkPolicyList.Items))
		for _, networkPolicy := range networkPolicyList.Items {
			serverName := networkPolicy.Labels[label]
			logrus.Debugf("Removing deptecated network policy: %s server %s ns %s", networkPolicy.Name, serverName, networkPolicy.Namespace)
			err = r.removeNetworkPolicy(ctx, networkPolicy)
			if err != nil {
				return errors.Wrap(err)
			}
			deletedCount += 1
		}
	}
	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesDeleted, deletedCount)
	prometheus.IncrementNetpolDeleted(deletedCount)
	return nil
}

func (r *Reconciler) reconcileEndpointsForPolicy(ctx context.Context, newPolicy *v1.NetworkPolicy) error {
	selector, err := metav1.LabelSelectorAsSelector(&newPolicy.Spec.PodSelector)
	if err != nil {
		return errors.Wrap(err)
	}
	// Use the external netpolHandler to check if pods got affected and if so, if they need external allow policies
	return r.extNetpolHandler.HandlePodsByLabelSelector(ctx, newPolicy.Namespace, selector)
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
