package networkpolicy

import (
	"context"
	goerrors "errors"
	"fmt"
	"github.com/amit7itz/goset"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"
	"time"
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
	HandleAllPods(ctx context.Context) error
}

type Reconciler struct {
	client.Client
	Scheme                              *runtime.Scheme
	RestrictToNamespaces                []string
	EnforcedNamespaces                  *goset.Set[string]
	EnableNetworkPolicyCreation         bool
	EnforcementDefaultState             bool
	CreateSeparateEgressIngressPolicies bool
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
	enforcedNamespaces *goset.Set[string],
	enableNetworkPolicyCreation bool,
	enforcementDefaultState bool,
	createSeparateEgressIngressPolicies bool,
	ingressBuilders []IngressRuleBuilder,
	egressBuilders []EgressRuleBuilder) *Reconciler {

	return &Reconciler{
		Client:                              c,
		Scheme:                              s,
		RestrictToNamespaces:                restrictToNamespaces,
		EnforcedNamespaces:                  enforcedNamespaces,
		EnableNetworkPolicyCreation:         enableNetworkPolicyCreation,
		EnforcementDefaultState:             enforcementDefaultState,
		CreateSeparateEgressIngressPolicies: createSeparateEgressIngressPolicies,
		egressRuleBuilders:                  egressBuilders,
		ingressRuleBuilders:                 ingressBuilders,
		extNetpolHandler:                    externalNetpolHandler,
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
	timeoutCtx, cancel := context.WithTimeoutCause(ctx, 30*time.Second, errors.Errorf("timeout while reconciling already-built list of service effective policies"))
	defer cancel()
	ctx = timeoutCtx

	currentPolicies := goset.NewSet[types.NamespacedName]()
	errorList := make([]error, 0)
	for _, ep := range eps {
		netpols, created, err := r.applyServiceEffectivePolicy(ctx, ep)
		if err != nil {
			errorList = append(errorList, errors.Wrap(err))
			continue
		}
		if created {
			currentPolicies.Add(netpols...)
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

func (r *Reconciler) applyServiceEffectivePolicy(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]types.NamespacedName, bool, error) {
	timeoutCtx, cancel := context.WithTimeoutCause(ctx, 5*time.Second, errors.Errorf("timeout while reconciling single service effective policy"))
	defer cancel()
	ctx = timeoutCtx
	if !r.EnableNetworkPolicyCreation {
		logrus.Debugf("Network policy creation is disabled, skipping network policy creation for service %s in namespace %s", ep.Service.Name, ep.Service.Namespace)
		if len(ep.Calls) > 0 && len(r.egressRuleBuilders) > 0 {
			ep.ClientIntentsEventRecorder.RecordNormalEventf(consts.ReasonEgressNetworkPolicyCreationDisabled, "Network policy creation is disabled, creation skipped")
		}
		if len(ep.CalledBy) > 0 && len(r.ingressRuleBuilders) > 0 {
			ep.RecordOnClientsNormalEventf(consts.ReasonNetworkPolicyCreationDisabled, "Network policy creation is disabled, creation skipped")
		}
		return nil, false, nil
	}
	netpols, shouldCreate, err := r.buildNetworkPolicies(ctx, ep)
	if err != nil {
		r.recordCreateFailedError(ep, err)
		return nil, false, errors.Wrap(err)
	}
	if !shouldCreate {
		return nil, shouldCreate, errors.Wrap(err)
	}

	netpolNames := make([]types.NamespacedName, 0)
	for _, netpol := range netpols {
		err := r.applyNetpol(ctx, ep, netpol)
		if err != nil {
			return nil, false, errors.Wrap(err)
		}
		netpolNames = append(netpolNames, types.NamespacedName{Name: netpol.Name, Namespace: netpol.Namespace})
	}

	return netpolNames, true, nil

}

func (r *Reconciler) applyNetpol(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy, netpol v1.NetworkPolicy) error {
	existingPolicy := &v1.NetworkPolicy{}

	err := r.Get(ctx, types.NamespacedName{
		Name:      netpol.Name,
		Namespace: ep.Service.Namespace},
		existingPolicy)

	if err != nil && !k8serrors.IsNotFound(err) {
		r.RecordWarningEventf(existingPolicy, consts.ReasonGettingNetworkPolicyFailed, "failed to get network policy: %s", err.Error())
		return errors.Wrap(err)
	}
	if k8serrors.IsNotFound(err) {
		err = r.createNetworkPolicy(ctx, ep, &netpol)
		if err != nil {
			r.recordCreateFailedError(ep, err)
			return errors.Wrap(err)
		}
		return nil
	}

	err = r.updateExistingPolicy(ctx, ep, existingPolicy, &netpol)
	if err != nil {
		r.recordCreateFailedError(ep, err)
		return errors.Wrap(err)
	}

	return nil
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

	if len(ep.Calls) == 0 {
		return rules, false, nil
	}

	if len(r.egressRuleBuilders) == 0 {
		hasInternetIntents := lo.SomeBy(ep.Calls, func(intent effectivepolicy.Call) bool { return intent.Internet != nil })
		if hasInternetIntents {
			logrus.Debugf("Client has interner intents but egress network policy is not enabled")
			ep.ClientIntentsEventRecorder.RecordWarningEvent(consts.ReasonInternetEgressNetworkPolicyWithEgressPolicyDisabled, "ClientIntents refer to the Internet but egress network policy is disabled")
		}

		return rules, false, nil
	}

	if !r.EnforcementDefaultState {
		logrus.Debugf("Enforcement is disabled globally skipping egress network policy creation for service %s in namespace %s", ep.Service.Name, ep.Service.Namespace)
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
	shouldCreatePolicy, err := protected_services.IsServerEnforcementEnabledDueToProtectionOrDefaultState(ctx, r.Client, ep.Service, r.EnforcementDefaultState, r.EnforcedNamespaces)
	if err != nil {
		return rules, false, errors.Wrap(err)
	}
	if !shouldCreatePolicy {
		logrus.Debugf("Enforcement is disabled globally and server is not explicitly protected, skipping network policy creation for server %s in namespace %s", ep.Service.Name, ep.Service.Namespace)
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
	labelsMap, ok, err := otterizev2alpha1.ServiceIdentityToLabelsForWorkloadSelection(ctx, r.Client, ep.Service)
	if err != nil {
		return metav1.LabelSelector{}, false, errors.Wrap(err)
	}
	if !ok {
		return metav1.LabelSelector{}, false, nil
	}

	return metav1.LabelSelector{MatchLabels: labelsMap}, true, nil
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

func (r *Reconciler) buildNetworkPolicies(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicy, bool, error) {
	networkPolicies := make([]v1.NetworkPolicy, 0)
	egressRules, shouldCreateEgress, err := r.buildEgressRules(ctx, ep)
	if err != nil {
		return nil, false, errors.Wrap(err)
	}

	ingressRules, shouldCreateIngress, err := r.buildIngressRules(ctx, ep)
	if err != nil {
		return nil, false, errors.Wrap(err)
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

	if r.CreateSeparateEgressIngressPolicies {
		networkPolicies = r.buildSeparatePolicies(ep, podSelector, shouldCreateIngress, ingressRules, shouldCreateEgress, egressRules)
	} else {
		networkPolicies = append(networkPolicies, r.buildSinglePolicy(ep, podSelector, shouldCreateIngress, ingressRules, shouldCreateEgress, egressRules))
	}

	for i := range networkPolicies {
		err = r.setNetworkPolicyOwnerReferenceIfNeeded(ctx, ep, &networkPolicies[i])
		if err != nil {
			return nil, false, errors.Wrap(err)
		}
	}

	return networkPolicies, true, nil
}

func (r *Reconciler) buildSinglePolicy(ep effectivepolicy.ServiceEffectivePolicy, podSelector metav1.LabelSelector, shouldCreateIngress bool, ingressRules []v1.NetworkPolicyIngressRule, shouldCreateEgress bool, egressRules []v1.NetworkPolicyEgressRule) v1.NetworkPolicy {
	policyTypes := make([]v1.PolicyType, 0)
	if shouldCreateIngress {
		policyTypes = append(policyTypes, v1.PolicyTypeIngress)
	}
	if shouldCreateEgress {
		policyTypes = append(policyTypes, v1.PolicyTypeEgress)
	}

	return v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(otterizev2alpha1.OtterizeSingleNetworkPolicyNameTemplate, ep.Service.GetRFC1123NameWithKind()),
			Namespace: ep.Service.Namespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeNetworkPolicy: ep.Service.GetFormattedOtterizeIdentityWithKind(),
			},
		},
		Spec: v1.NetworkPolicySpec{
			PodSelector: podSelector,
			PolicyTypes: policyTypes,
			Egress:      lo.Ternary(len(egressRules) > 0, egressRules, nil),
			Ingress:     lo.Ternary(len(ingressRules) > 0, ingressRules, nil),
		},
	}
}

func (r *Reconciler) buildSeparatePolicies(ep effectivepolicy.ServiceEffectivePolicy, podSelector metav1.LabelSelector, shouldCreateIngress bool, ingressRules []v1.NetworkPolicyIngressRule, shouldCreateEgress bool, egressRules []v1.NetworkPolicyEgressRule) []v1.NetworkPolicy {
	res := make([]v1.NetworkPolicy, 0)
	if shouldCreateIngress {
		ingressPolicy := v1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf(otterizev2alpha1.OtterizeNetworkPolicyIngressNameTemplate, ep.Service.GetRFC1123NameWithKind()),
				Namespace: ep.Service.Namespace,
				Labels: map[string]string{
					otterizev2alpha1.OtterizeNetworkPolicy: ep.Service.GetFormattedOtterizeIdentityWithKind(),
				},
			},
			Spec: v1.NetworkPolicySpec{
				PodSelector: podSelector,
				PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
				Ingress:     ingressRules,
			},
		}
		res = append(res, ingressPolicy)
	}
	if shouldCreateEgress {
		egressPolicy := v1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf(otterizev2alpha1.OtterizeNetworkPolicyEgressNameTemplate, ep.Service.GetRFC1123NameWithKind()),
				Namespace: ep.Service.Namespace,
				Labels: map[string]string{
					otterizev2alpha1.OtterizeNetworkPolicy: ep.Service.GetFormattedOtterizeIdentityWithKind(),
				},
			},
			Spec: v1.NetworkPolicySpec{
				PodSelector: podSelector,
				PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
				Egress:      egressRules,
			},
		}
		res = append(res, egressPolicy)
	}
	return res
}

func (r *Reconciler) createNetworkPolicy(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy, netpol *v1.NetworkPolicy) error {
	err := r.Create(ctx, netpol)
	if err != nil {
		return r.handleCreationErrors(ctx, ep, netpol, err)
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
	unmarshalledNewPolicy, err := marshalUnmarshalNetpol(newPolicy)
	if err != nil {
		return errors.Wrap(err)
	}

	if isNetworkPolicySpecEqual(existingPolicy.Spec, unmarshalledNewPolicy.Spec) {
		return nil
	}

	policyCopy := existingPolicy.DeepCopy()
	policyCopy.Labels = newPolicy.Labels
	policyCopy.Annotations = newPolicy.Annotations
	policyCopy.Spec = newPolicy.Spec

	err = r.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy))
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
	timeoutCtx, cancel := context.WithTimeoutCause(ctx, 5*time.Second, errors.Errorf("timeout while removing orphaned network policies"))
	defer cancel()
	ctx = timeoutCtx
	logrus.Debug("Searching for orphaned network policies")
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
			serverName := networkPolicy.Labels[otterizev2alpha1.OtterizeNetworkPolicy]
			logrus.Debugf("Removing orphaned network policy: %s server %s ns %s", networkPolicy.Name, serverName, networkPolicy.Namespace)
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
	if k8serrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Wrap(err)
	}
	return nil
}

func (r *Reconciler) removeDeprecatedNetworkPolicies(ctx context.Context) error {
	logrus.Debug("Searching for network policies with deprecated labels")
	deprecatedLabels := []string{otterizev2alpha1.OtterizeEgressNetworkPolicy, otterizev2alpha1.OtterizeSvcEgressNetworkPolicy, otterizev2alpha1.OtterizeInternetNetworkPolicy, otterizev2alpha1.OtterizeSvcNetworkPolicy}
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

func (r *Reconciler) handleExistingPolicyRetry(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy, netpol *v1.NetworkPolicy) error {
	existingPolicy := &v1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      netpol.Name,
		Namespace: ep.Service.Namespace},
		existingPolicy)
	if err != nil && !k8serrors.IsNotFound(err) {
		r.RecordWarningEventf(existingPolicy, consts.ReasonGettingNetworkPolicyFailed, "failed to get network policy: %s", err.Error())
		return errors.Wrap(err)
	}
	if k8serrors.IsNotFound(err) {
		return errors.Errorf("failed creating network policy with AlreadyExists err, but failed fetching network policy  from k8s api server")
	}

	return r.updateExistingPolicy(ctx, ep, existingPolicy, netpol)
}

func (r *Reconciler) handleCreationErrors(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy, netpol *v1.NetworkPolicy, err error) error {
	errStr := err.Error()
	if k8sErr := &(k8serrors.StatusError{}); errors.As(err, &k8sErr) {
		if k8serrors.IsAlreadyExists(k8sErr) {
			// Ideally we would just return {Requeue: true} but it is not possible without a mini-refactor
			return r.handleExistingPolicyRetry(ctx, ep, netpol)
		}

		if k8serrors.IsForbidden(k8sErr) && strings.Contains(errStr, "is being terminated") {
			// Namespace is being deleted, nothing to do further
			logrus.Debugf("Namespace %s is being terminated, ignoring api server error", netpol.Namespace)
			return nil
		}

		if k8serrors.IsNotFound(k8sErr) && strings.Contains(errStr, netpol.Namespace) {
			// Namespace was deleted since we started .Create() logic, nothing to do further
			logrus.Debugf("Namespace %s was deleted, ignoring api server error", netpol.Namespace)
			return nil
		}
	}

	return errors.Wrap(err)
}

func matchAccessNetworkPolicy() (labels.Selector, error) {
	isOtterizeNetworkPolicy := metav1.LabelSelectorRequirement{
		Key:      otterizev2alpha1.OtterizeNetworkPolicy,
		Operator: metav1.LabelSelectorOpExists,
	}
	isNotExternalTrafficPolicy := metav1.LabelSelectorRequirement{
		Key:      otterizev2alpha1.OtterizeNetworkPolicyExternalTraffic,
		Operator: metav1.LabelSelectorOpDoesNotExist,
	}
	isNotDefaultDenyPolicy := metav1.LabelSelectorRequirement{
		Key:      otterizev2alpha1.OtterizeNetworkPolicyServiceDefaultDeny,
		Operator: metav1.LabelSelectorOpDoesNotExist,
	}
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		isOtterizeNetworkPolicy,
		isNotExternalTrafficPolicy,
		isNotDefaultDenyPolicy,
	}})
}
