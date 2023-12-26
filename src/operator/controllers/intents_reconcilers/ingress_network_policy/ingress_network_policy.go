package ingress_network_policy

import (
	"context"
	"fmt"
	"github.com/amit7itz/goset"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/protected_services"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/prometheus"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/allowexternaltraffic"
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
	"k8s.io/apimachinery/pkg/util/sets"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
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

type NetworkPolicyReconciler struct {
	client.Client
	Scheme                      *runtime.Scheme
	extNetpolHandler            externalNetpolHandler
	RestrictToNamespaces        []string
	enableNetworkPolicyCreation bool
	enforcementDefaultState     bool
	allowExternalTraffic        allowexternaltraffic.Enum
	injectablerecorder.InjectableRecorder
}

func NewNetworkPolicyReconciler(
	c client.Client,
	s *runtime.Scheme,
	extNetpolHandler externalNetpolHandler,
	restrictToNamespaces []string,
	enableNetworkPolicyCreation bool,
	enforcementDefaultState bool,
	allowExternalTraffic allowexternaltraffic.Enum,
) *NetworkPolicyReconciler {
	return &NetworkPolicyReconciler{
		Client:                      c,
		Scheme:                      s,
		extNetpolHandler:            extNetpolHandler,
		RestrictToNamespaces:        restrictToNamespaces,
		enableNetworkPolicyCreation: enableNetworkPolicyCreation,
		enforcementDefaultState:     enforcementDefaultState,
		allowExternalTraffic:        allowExternalTraffic,
	}
}

// TODO: Should get all the effective policies

func (r *NetworkPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	intents := &otterizev1alpha3.ClientIntents{}
	err := r.Get(ctx, req.NamespacedName, intents)
	if k8serrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	if intents.Spec == nil {
		return ctrl.Result{}, nil
	}

	logrus.Infof("Reconciling network policies for service %s in namespace %s",
		intents.Spec.Service.Name, req.Namespace)

	// Object is deleted, handle finalizer and network policy clean up
	if !intents.DeletionTimestamp.IsZero() {
		err := r.cleanPolicies(ctx, intents)
		if err != nil {
			if k8serrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			r.RecordWarningEventf(intents, consts.ReasonRemovingNetworkPolicyFailed, "could not remove network policies: %s", err.Error())
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	currentPolicies := goset.NewSet[types.NamespacedName]()

	eps, err := effectivepolicy.GetAllServiceEffectivePolicies(ctx, r.Client, &r.InjectableRecorder)

	for _, ep := range eps {
		// Return policies
		netpols, err := r.ReconcileServiceEffectivePolicy(ctx, ep)
		currentPolicies.Add(netpols...)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// remove policies that doesn't exist in the policies list
	err = r.removeOrphanNetworkPolicies(ctx, currentPolicies)
	if err != nil {
		r.RecordWarningEventf(intents, consts.ReasonRemovingNetworkPolicyFailed, "failed to remove network policies: %s", err.Error())
		return ctrl.Result{}, err
	}

	if currentPolicies.Len() != 0 {
		callsCount := len(intents.GetCallsList())
		r.RecordNormalEventf(intents, consts.ReasonCreatedNetworkPolicies, "NetworkPolicy reconcile complete, reconciled %d servers", callsCount)
		telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesCreated, currentPolicies.Len())
		prometheus.IncrementNetpolCreated(currentPolicies.Len())
	}
	return ctrl.Result{}, nil
}

// ReconcileServiceEffectivePolicy - reconcile ingress netpols for a service. returns the list of policies' namespaced names
func (r *NetworkPolicyReconciler) ReconcileServiceEffectivePolicy(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]types.NamespacedName, error) {
	shouldCreatePolicy, err := protected_services.IsServerEnforcementEnabledDueToProtectionOrDefaultState(ctx, r.Client, ep.Service.Name, ep.Service.Namespace, r.enforcementDefaultState)
	if err != nil {
		return nil, err
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
			return networkPolicies, err
		}

		if k8serrors.IsNotFound(err) {
			err = r.CreateNetworkPolicy(ctx, intentCall.Service.Namespace, intentCall.IntendedCall, newPolicy)
			if err != nil {
				return networkPolicies, err
			}
			networkPolicies = append(networkPolicies, types.NamespacedName{Name: newPolicy.Name, Namespace: newPolicy.Namespace})
			continue
		}
		err = r.UpdateExistingPolicy(ctx, existingPolicy, newPolicy, intentCall.IntendedCall, intentCall.Service.Namespace)
		if err != nil {
			return networkPolicies, err
		}
		networkPolicies = append(networkPolicies, types.NamespacedName{Name: newPolicy.Name, Namespace: newPolicy.Namespace})
	}
	return networkPolicies, nil
}

func (r *NetworkPolicyReconciler) handleNetworkPolicyCreation(
	ctx context.Context, intentsObj *otterizev1alpha3.ClientIntents, intent otterizev1alpha3.Intent, intentsObjNamespace string) (bool, error) {

	shouldCreatePolicy, err := protected_services.IsServerEnforcementEnabledDueToProtectionOrDefaultState(ctx, r.Client, intent.GetTargetServerName(), intent.GetTargetServerNamespace(intentsObjNamespace), r.enforcementDefaultState)
	if err != nil {
		return false, err
	}

	if !shouldCreatePolicy {
		logrus.Infof("Enforcement is disabled globally and server is not explicitly protected, skipping network policy creation for server %s in namespace %s", intent.GetTargetServerName(), intent.GetTargetServerNamespace(intentsObjNamespace))
		r.RecordNormalEventf(intentsObj, consts.ReasonEnforcementDefaultOff, "Enforcement is disabled globally and called service '%s' is not explicitly protected using a ProtectedService resource, network policy creation skipped", intent.Name)
		return false, nil
	}
	if !r.enableNetworkPolicyCreation {
		logrus.Infof("Network policy creation is disabled, skipping network policy creation for server %s in namespace %s", intent.GetTargetServerName(), intent.GetTargetServerNamespace(intentsObjNamespace))
		r.RecordNormalEvent(intentsObj, consts.ReasonNetworkPolicyCreationDisabled, "Network policy creation is disabled, creation skipped")
		return false, nil
	}

	logrus.Debugf("Server %s in namespace %s is in protected list: %t", intent.GetTargetServerName(), intent.GetTargetServerNamespace(intentsObjNamespace), shouldCreatePolicy)

	policyName := fmt.Sprintf(otterizev1alpha3.OtterizeNetworkPolicyNameTemplate, intent.GetTargetServerName(), intentsObjNamespace)
	existingPolicy := &v1.NetworkPolicy{}
	newPolicy := r.buildNetworkPolicyObjectForIntent(intent, policyName, intentsObjNamespace)
	err = r.Get(ctx, types.NamespacedName{
		Name:      policyName,
		Namespace: intent.GetTargetServerNamespace(intentsObjNamespace)},
		existingPolicy)
	if err != nil && !k8serrors.IsNotFound(err) {
		r.RecordWarningEventf(existingPolicy, consts.ReasonGettingNetworkPolicyFailed, "failed to get network policy: %s", err.Error())
		return false, err
	}

	if k8serrors.IsNotFound(err) {
		return true, r.CreateNetworkPolicy(ctx, intentsObjNamespace, intent, newPolicy)
	}

	return true, r.UpdateExistingPolicy(ctx, existingPolicy, newPolicy, intent, intentsObjNamespace)
}

func (r *NetworkPolicyReconciler) UpdateExistingPolicy(ctx context.Context, existingPolicy *v1.NetworkPolicy, newPolicy *v1.NetworkPolicy, intent otterizev1alpha3.Intent, intentsObjNamespace string) error {
	if !reflect.DeepEqual(existingPolicy.Spec, newPolicy.Spec) {
		policyCopy := existingPolicy.DeepCopy()
		policyCopy.Labels = newPolicy.Labels
		policyCopy.Annotations = newPolicy.Annotations
		policyCopy.Spec = newPolicy.Spec

		err := r.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy))
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *NetworkPolicyReconciler) CreateNetworkPolicy(ctx context.Context, intentsObjNamespace string, intent otterizev1alpha3.Intent, newPolicy *v1.NetworkPolicy) error {
	logrus.Infof(
		"Creating network policy to enable access from namespace %s to %s", intentsObjNamespace, intent.Name)
	err := r.Create(ctx, newPolicy)
	if err != nil {
		return err
	}

	return r.reconcileEndpointsForPolicy(ctx, newPolicy)
}

func (r *NetworkPolicyReconciler) reconcileEndpointsForPolicy(ctx context.Context, newPolicy *v1.NetworkPolicy) error {
	selector, err := metav1.LabelSelectorAsSelector(&newPolicy.Spec.PodSelector)
	if err != nil {
		return err
	}
	// Use the external netpolHandler to check if pods got affected and if so, if they need external allow policies
	return r.extNetpolHandler.HandlePodsByLabelSelector(ctx, newPolicy.Namespace, selector)
}

func (r *NetworkPolicyReconciler) cleanPolicies(
	ctx context.Context, intents *otterizev1alpha3.ClientIntents) error {
	logrus.Infof("Removing network policies for deleted intents for service: %s", intents.Spec.Service.Name)
	for _, intent := range intents.GetCallsList() {
		if intent.Type != "" && intent.Type != otterizev1alpha3.IntentTypeHTTP && intent.Type != otterizev1alpha3.IntentTypeKafka {
			continue
		}
		err := r.handleIntentRemoval(ctx, intent, intents.Namespace)
		if err != nil {
			return err
		}
	}

	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesDeleted, len(intents.GetCallsList()))
	prometheus.IncrementNetpolCreated(len(intents.GetCallsList()))

	return nil
}

func (r *NetworkPolicyReconciler) handleIntentRemoval(
	ctx context.Context,
	intent otterizev1alpha3.Intent,
	intentsObjNamespace string) error {

	var intentsList otterizev1alpha3.ClientIntentsList
	err := r.List(
		ctx, &intentsList,
		&client.MatchingFields{otterizev1alpha3.OtterizeTargetServerIndexField: intent.GetServerFullyQualifiedName(intentsObjNamespace)},
		&client.ListOptions{Namespace: intentsObjNamespace})

	if err != nil {
		return err
	}

	if len(intentsList.Items) == 1 {
		// We have only 1 intents resource that has this server as its target - and it's the current one
		// We need to delete the network policy that allows access from this namespace, as there are no other
		// clients in that namespace that need to access the target server
		logrus.Infof("No other intents in the namespace reference target server: %s", intent.Name)
		logrus.Infoln("Removing matching network policy for server")
		if err = r.deleteNetworkPolicy(ctx, intent, intentsObjNamespace); err != nil {
			return err
		}

		labelSelector := r.buildPodLabelSelectorFromIntent(intent, intentsObjNamespace)
		selector, err := metav1.LabelSelectorAsSelector(&labelSelector)
		if err != nil {
			return err
		}
		if err = r.extNetpolHandler.HandlePodsByLabelSelector(ctx, intent.GetTargetServerNamespace(intentsObjNamespace), selector); err != nil {
			return err
		}

	}
	return nil
}

func (r *NetworkPolicyReconciler) removeOrphanNetworkPolicies(ctx context.Context, netpolNamesThatShouldExist *goset.Set[types.NamespacedName]) error {
	logrus.Info("Searching for orphaned network policies")
	networkPolicyList := &v1.NetworkPolicyList{}
	selector, err := matchAccessNetworkPolicy()
	if err != nil {
		return err
	}

	err = r.List(ctx, networkPolicyList, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		logrus.Infof("Error listing network policies: %s", err.Error())
		return err
	}

	logrus.Infof("Selector: %s found %d network policies", selector.String(), len(networkPolicyList.Items))
	for _, networkPolicy := range networkPolicyList.Items {
		namespacedName := types.NamespacedName{Namespace: networkPolicy.Namespace, Name: networkPolicy.Name}
		if !netpolNamesThatShouldExist.Contains(namespacedName) {
			serverName := networkPolicy.Labels[otterizev1alpha3.OtterizeNetworkPolicy]
			logrus.Infof("Removing orphaned network policy: %s server %s ns %s", networkPolicy.Name, serverName, networkPolicy.Namespace)
			err = r.removeNetworkPolicy(ctx, networkPolicy)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *NetworkPolicyReconciler) removeNetworkPolicy(ctx context.Context, networkPolicy v1.NetworkPolicy) error {
	err := r.extNetpolHandler.HandleBeforeAccessPolicyRemoval(ctx, &networkPolicy)
	if err != nil {
		return err
	}
	err = r.Delete(ctx, &networkPolicy)
	if err != nil {
		return err
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

func (r *NetworkPolicyReconciler) deleteNetworkPolicy(
	ctx context.Context,
	intent otterizev1alpha3.Intent,
	intentsObjNamespace string) error {

	policyName := fmt.Sprintf(otterizev1alpha3.OtterizeNetworkPolicyNameTemplate, intent.GetTargetServerName(), intentsObjNamespace)
	policy := &v1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: policyName, Namespace: intent.GetTargetServerNamespace(intentsObjNamespace)}, policy)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return r.removeNetworkPolicy(ctx, *policy)
}

func (r *NetworkPolicyReconciler) CleanPoliciesFromUnprotectedServices(ctx context.Context, namespace string) error {
	selector, err := matchAccessNetworkPolicy()
	if err != nil {
		return err
	}

	policies := &v1.NetworkPolicyList{}
	err = r.List(ctx, policies, &client.ListOptions{Namespace: namespace, LabelSelector: selector})
	if err != nil {
		return err
	}

	if len(policies.Items) == 0 {
		return nil
	}

	var protectedServicesResources otterizev1alpha3.ProtectedServiceList
	err = r.List(ctx, &protectedServicesResources, &client.ListOptions{Namespace: namespace})
	if err != nil {
		return err
	}

	protectedServersByNamespace := sets.Set[string]{}
	for _, protectedService := range protectedServicesResources.Items {
		// skip protected services that are in deletion process
		if !protectedService.DeletionTimestamp.IsZero() {
			continue
		}
		serverName := otterizev1alpha3.GetFormattedOtterizeIdentity(protectedService.Spec.Name, namespace)
		protectedServersByNamespace.Insert(serverName)
	}

	for _, networkPolicy := range policies.Items {
		serverName := networkPolicy.Labels[otterizev1alpha3.OtterizeNetworkPolicy]
		if !protectedServersByNamespace.Has(serverName) {
			err = r.removeNetworkPolicy(ctx, networkPolicy)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *NetworkPolicyReconciler) CleanAllNamespaces(ctx context.Context) error {
	namespaces := corev1.NamespaceList{}
	err := r.List(ctx, &namespaces)
	if err != nil {
		return err
	}

	for _, namespace := range namespaces.Items {
		err = r.CleanPoliciesFromUnprotectedServices(ctx, namespace.Name)
		if err != nil {
			return err
		}
	}

	return nil
}

// buildNetworkPolicyObjectForIntent builds the network policy that represents the intent from the parameter
func (r *NetworkPolicyReconciler) buildNetworkPolicyObjectForIntent(
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

func (r *NetworkPolicyReconciler) buildPodLabelSelectorFromIntent(intent otterizev1alpha3.Intent, intentsObjNamespace string) metav1.LabelSelector {
	targetNamespace := intent.GetTargetServerNamespace(intentsObjNamespace)
	// The intent's target server made of name + namespace + hash
	formattedTargetServer := otterizev1alpha3.GetFormattedOtterizeIdentity(intent.GetTargetServerName(), targetNamespace)

	return metav1.LabelSelector{
		MatchLabels: map[string]string{
			otterizev1alpha3.OtterizeServerLabelKey: formattedTargetServer,
		},
	}
}
