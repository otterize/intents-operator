package intents_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/protected_services"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type externalNetpolHandler interface {
	HandlePodsByLabelSelector(ctx context.Context, namespace string, labelSelector labels.Selector) error
	HandleBeforeAccessPolicyRemoval(ctx context.Context, accessPolicy *v1.NetworkPolicy) error
}

type NetworkPolicyReconciler struct {
	client.Client
	Scheme                                        *runtime.Scheme
	extNetpolHandler                              externalNetpolHandler
	RestrictToNamespaces                          []string
	enableNetworkPolicyCreation                   bool
	enforcementDefaultState                       bool
	externalNetworkPoliciesCreatedEvenIfNoIntents bool
	injectablerecorder.InjectableRecorder
}

func NewNetworkPolicyReconciler(
	c client.Client,
	s *runtime.Scheme,
	extNetpolHandler externalNetpolHandler,
	restrictToNamespaces []string,
	enableNetworkPolicyCreation bool,
	enforcementDefaultState bool,
	externalNetworkPoliciesCreatedEvenIfNoIntents bool) *NetworkPolicyReconciler {
	return &NetworkPolicyReconciler{
		Client:                      c,
		Scheme:                      s,
		extNetpolHandler:            extNetpolHandler,
		RestrictToNamespaces:        restrictToNamespaces,
		enableNetworkPolicyCreation: enableNetworkPolicyCreation,
		enforcementDefaultState:     enforcementDefaultState,
		externalNetworkPoliciesCreatedEvenIfNoIntents: externalNetworkPoliciesCreatedEvenIfNoIntents,
	}
}

func (r *NetworkPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	intents := &otterizev1alpha2.ClientIntents{}
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
		err := r.cleanFinalizerAndPolicies(ctx, intents)
		if err != nil {
			if k8serrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			r.RecordWarningEventf(intents, consts.ReasonRemovingNetworkPolicyFailed, "could not remove network policies: %s", err.Error())
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(intents, otterizev1alpha2.NetworkPolicyFinalizerName) {
		logrus.WithField("namespacedName", req.String()).Infof("Adding finalizer %s", otterizev1alpha2.NetworkPolicyFinalizerName)
		controllerutil.AddFinalizer(intents, otterizev1alpha2.NetworkPolicyFinalizerName)
		if err := r.Update(ctx, intents); err != nil {
			return ctrl.Result{}, err
		}
	}
	createdNetpols := 0
	for _, intent := range intents.GetCallsList() {
		targetNamespace := intent.GetServerNamespace(req.Namespace)
		if len(r.RestrictToNamespaces) != 0 && !lo.Contains(r.RestrictToNamespaces, targetNamespace) {
			// Namespace is not in list of namespaces we're allowed to act in, so drop it.
			r.RecordWarningEventf(intents, consts.ReasonNamespaceNotAllowed, "namespace %s was specified in intent, but is not allowed by configuration", targetNamespace)
			continue
		}
		createdPolicies, err := r.handleNetworkPolicyCreation(ctx, intents, intent, req.Namespace)
		if err != nil {
			r.RecordWarningEventf(intents, consts.ReasonCreatingNetworkPoliciesFailed, "could not create network policies: %s", err.Error())
			return ctrl.Result{}, err
		}
		if createdPolicies {
			createdNetpols += 1
		}
	}

	err = r.removeOrphanNetworkPolicies(ctx)
	if err != nil {
		r.RecordWarningEventf(intents, consts.ReasonRemovingNetworkPolicyFailed, "failed to remove network policies: %s", err.Error())
		return ctrl.Result{}, err
	}

	if createdNetpols != 0 {
		callsCount := len(intents.GetCallsList())
		r.RecordNormalEventf(intents, consts.ReasonCreatedNetworkPolicies, "NetworkPolicy reconcile complete, reconciled %d servers", callsCount)
		telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesCreated, createdNetpols)
	}
	return ctrl.Result{}, nil
}

func (r *NetworkPolicyReconciler) handleNetworkPolicyCreation(
	ctx context.Context, intentsObj *otterizev1alpha2.ClientIntents, intent otterizev1alpha2.Intent, intentsObjNamespace string) (bool, error) {

	shouldCreatePolicy, err := protected_services.IsServerEnforcementEnabledDueToProtectionOrDefaultState(ctx, r.Client, intent.GetServerName(), intent.GetServerNamespace(intentsObjNamespace), r.enforcementDefaultState)
	if err != nil {
		return false, err
	}

	if !shouldCreatePolicy {
		logrus.Infof("Enforcement is disabled globally and server is not explicitly protected, skipping network policy creation for server %s in namespace %s", intent.GetServerName(), intent.GetServerNamespace(intentsObjNamespace))
		r.RecordNormalEventf(intentsObj, consts.ReasonEnforcementDefaultOff, "Enforcement is disabled globally and called service '%s' is not explicitly protected using a ProtectedService resource, network policy creation skipped", intent.Name)
		return false, nil
	}
	if !r.enableNetworkPolicyCreation {
		logrus.Infof("Network policy creation is disabled, skipping network policy creation for server %s in namespace %s", intent.GetServerName(), intent.GetServerNamespace(intentsObjNamespace))
		r.RecordNormalEvent(intentsObj, consts.ReasonNetworkPolicyCreationDisabled, "Network policy creation is disabled, creation skipped")
		return false, nil
	}

	logrus.Debugf("Server %s in namespace %s is in protected list: %t", intent.GetServerName(), intent.GetServerNamespace(intentsObjNamespace), shouldCreatePolicy)

	policyName := fmt.Sprintf(otterizev1alpha2.OtterizeNetworkPolicyNameTemplate, intent.GetServerName(), intentsObjNamespace)
	existingPolicy := &v1.NetworkPolicy{}
	newPolicy := r.buildNetworkPolicyObjectForIntent(intent, policyName, intentsObjNamespace)
	err = r.Get(ctx, types.NamespacedName{
		Name:      policyName,
		Namespace: intent.GetServerNamespace(intentsObjNamespace)},
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

func (r *NetworkPolicyReconciler) UpdateExistingPolicy(ctx context.Context, existingPolicy *v1.NetworkPolicy, newPolicy *v1.NetworkPolicy, intent otterizev1alpha2.Intent, intentsObjNamespace string) error {
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

func (r *NetworkPolicyReconciler) CreateNetworkPolicy(ctx context.Context, intentsObjNamespace string, intent otterizev1alpha2.Intent, newPolicy *v1.NetworkPolicy) error {
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

func (r *NetworkPolicyReconciler) cleanFinalizerAndPolicies(
	ctx context.Context, intents *otterizev1alpha2.ClientIntents) error {
	if !controllerutil.ContainsFinalizer(intents, otterizev1alpha2.NetworkPolicyFinalizerName) {
		return nil
	}
	logrus.Infof("Removing network policies for deleted intents for service: %s", intents.Spec.Service.Name)
	for _, intent := range intents.GetCallsList() {
		err := r.handleIntentRemoval(ctx, intent, intents.Namespace)
		if err != nil {
			return err
		}
	}

	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesDeleted, len(intents.GetCallsList()))

	RemoveIntentFinalizers(intents, otterizev1alpha2.NetworkPolicyFinalizerName)
	if err := r.Update(ctx, intents); err != nil {
		return err
	}

	return nil
}

func (r *NetworkPolicyReconciler) handleIntentRemoval(
	ctx context.Context,
	intent otterizev1alpha2.Intent,
	intentsObjNamespace string) error {

	var intentsList otterizev1alpha2.ClientIntentsList
	err := r.List(
		ctx, &intentsList,
		&client.MatchingFields{otterizev1alpha2.OtterizeTargetServerIndexField: intent.GetServerFullyQualifiedName(intentsObjNamespace)},
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
		if err = r.extNetpolHandler.HandlePodsByLabelSelector(ctx, intent.GetServerNamespace(intentsObjNamespace), selector); err != nil {
			return err
		}

	}
	return nil
}

func (r *NetworkPolicyReconciler) removeOrphanNetworkPolicies(ctx context.Context) error {
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
		// Get all client intents that reference this network policy
		var intentsList otterizev1alpha2.ClientIntentsList
		serverName := networkPolicy.Labels[otterizev1alpha2.OtterizeNetworkPolicy]
		clientNamespace := networkPolicy.Spec.Ingress[0].From[0].NamespaceSelector.MatchLabels[otterizev1alpha2.OtterizeNamespaceLabelKey]
		err = r.List(
			ctx,
			&intentsList,
			&client.MatchingFields{otterizev1alpha2.OtterizeFormattedTargetServerIndexField: serverName},
			&client.ListOptions{Namespace: clientNamespace},
		)
		if err != nil {
			return err
		}

		if len(intentsList.Items) == 0 {
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
		Key:      otterizev1alpha2.OtterizeNetworkPolicy,
		Operator: metav1.LabelSelectorOpExists,
	}
	isNotExternalTrafficPolicy := metav1.LabelSelectorRequirement{
		Key:      otterizev1alpha2.OtterizeNetworkPolicyExternalTraffic,
		Operator: metav1.LabelSelectorOpDoesNotExist,
	}
	isNotDefaultDenyPolicy := metav1.LabelSelectorRequirement{
		Key:      otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny,
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
	intent otterizev1alpha2.Intent,
	intentsObjNamespace string) error {

	policyName := fmt.Sprintf(otterizev1alpha2.OtterizeNetworkPolicyNameTemplate, intent.GetServerName(), intentsObjNamespace)
	policy := &v1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: policyName, Namespace: intent.GetServerNamespace(intentsObjNamespace)}, policy)
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

	var protectedServicesResources otterizev1alpha2.ProtectedServiceList
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
		serverName := otterizev1alpha2.GetFormattedOtterizeIdentity(protectedService.Spec.Name, namespace)
		protectedServersByNamespace.Insert(serverName)
	}

	for _, networkPolicy := range policies.Items {
		serverName := networkPolicy.Labels[otterizev1alpha2.OtterizeNetworkPolicy]
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
	intent otterizev1alpha2.Intent, policyName, intentsObjNamespace string) *v1.NetworkPolicy {
	targetNamespace := intent.GetServerNamespace(intentsObjNamespace)
	// The intent's target server made of name + namespace + hash
	formattedTargetServer := otterizev1alpha2.GetFormattedOtterizeIdentity(intent.GetServerName(), targetNamespace)
	podSelector := r.buildPodLabelSelectorFromIntent(intent, intentsObjNamespace)
	return &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: targetNamespace,
			Labels: map[string]string{
				otterizev1alpha2.OtterizeNetworkPolicy: formattedTargetServer,
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
										otterizev1alpha2.OtterizeAccessLabelKey, formattedTargetServer): "true",
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									otterizev1alpha2.OtterizeNamespaceLabelKey: intentsObjNamespace,
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *NetworkPolicyReconciler) buildPodLabelSelectorFromIntent(intent otterizev1alpha2.Intent, intentsObjNamespace string) metav1.LabelSelector {
	targetNamespace := intent.GetServerNamespace(intentsObjNamespace)
	// The intent's target server made of name + namespace + hash
	formattedTargetServer := otterizev1alpha2.GetFormattedOtterizeIdentity(intent.GetServerName(), targetNamespace)

	return metav1.LabelSelector{
		MatchLabels: map[string]string{
			otterizev1alpha2.OtterizeServerLabelKey: formattedTargetServer,
		},
	}
}
