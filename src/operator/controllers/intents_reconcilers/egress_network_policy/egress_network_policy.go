package egress_network_policy

import (
	"context"
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The EgressNetworkPolicyReconciler creates network policies that allow egress traffic from pods.
type EgressNetworkPolicyReconciler struct {
	client.Client
	Scheme                      *runtime.Scheme
	RestrictToNamespaces        []string
	enableNetworkPolicyCreation bool
	enforcementDefaultState     bool
	injectablerecorder.InjectableRecorder
}

func NewEgressNetworkPolicyReconciler(
	c client.Client,
	s *runtime.Scheme,
	restrictToNamespaces []string,
	enableNetworkPolicyCreation bool,
	enforcementDefaultState bool) *EgressNetworkPolicyReconciler {
	return &EgressNetworkPolicyReconciler{
		Client:                      c,
		Scheme:                      s,
		RestrictToNamespaces:        restrictToNamespaces,
		enableNetworkPolicyCreation: enableNetworkPolicyCreation,
		enforcementDefaultState:     enforcementDefaultState,
	}
}

func (r *EgressNetworkPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	intents := &otterizev1alpha3.ClientIntents{}
	err := r.Get(ctx, req.NamespacedName, intents)
	if k8serrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
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
			r.RecordWarningEventf(intents, consts.ReasonRemovingEgressNetworkPolicyFailed, "could not remove network policies: %s", err.Error())
			return ctrl.Result{}, errors.Wrap(err)
		}
		return ctrl.Result{}, nil
	}

	createdNetpols := 0
	for _, intent := range intents.GetCallsList() {
		if intent.Type != "" && intent.Type != otterizev1alpha3.IntentTypeHTTP && intent.Type != otterizev1alpha3.IntentTypeKafka {
			continue
		}
		if intent.IsTargetServerKubernetesService() {
			continue
		}
		if len(r.RestrictToNamespaces) != 0 && !lo.Contains(r.RestrictToNamespaces, intents.Namespace) {
			// Namespace is not in list of namespaces we're allowed to act in, so drop it.
			r.RecordWarningEventf(intents, consts.ReasonNamespaceNotAllowed, "ClientIntents are in namespace %s but namespace is not allowed by configuration", intents.Namespace)
			continue
		}
		createdPolicies, err := r.handleNetworkPolicyCreation(ctx, intents, intent, req.Namespace)
		if err != nil {
			r.RecordWarningEventf(intents, consts.ReasonCreatingEgressNetworkPoliciesFailed, "could not create network policies: %s", err.Error())
			return ctrl.Result{}, errors.Wrap(err)
		}
		if createdPolicies {
			createdNetpols += 1
		}
	}

	err = r.removeOrphanNetworkPolicies(ctx)
	if err != nil {
		r.RecordWarningEventf(intents, consts.ReasonRemovingEgressNetworkPolicyFailed, "failed to remove network policies: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	if createdNetpols != 0 {
		callsCount := len(intents.GetCallsList())
		r.RecordNormalEventf(intents, consts.ReasonCreatedEgressNetworkPolicies, "NetworkPolicy reconcile complete, reconciled %d servers", callsCount)
		telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesCreated, createdNetpols)
		prometheus.IncrementNetpolCreated(createdNetpols)
	}
	return ctrl.Result{}, nil
}

func (r *EgressNetworkPolicyReconciler) handleNetworkPolicyCreation(
	ctx context.Context, intentsObj *otterizev1alpha3.ClientIntents, intent otterizev1alpha3.Intent, intentsObjNamespace string) (bool, error) {

	if !r.enforcementDefaultState {
		logrus.Infof("Enforcement is disabled globally and server is not explicitly protected, skipping network policy creation for server %s in namespace %s", intent.GetTargetServerName(), intent.GetTargetServerNamespace(intentsObjNamespace))
		r.RecordNormalEventf(intentsObj, consts.ReasonEnforcementDefaultOff, "Enforcement is disabled globally, network policy creation skipped", intent.Name)
		return false, nil
	}
	if !r.enableNetworkPolicyCreation {
		logrus.Infof("Network policy creation is disabled, skipping network policy creation for server %s in namespace %s", intent.GetTargetServerName(), intent.GetTargetServerNamespace(intentsObjNamespace))
		r.RecordNormalEvent(intentsObj, consts.ReasonEgressNetworkPolicyCreationDisabled, "Network policy creation is disabled, creation skipped")
		return false, nil
	}

	policyName := fmt.Sprintf(otterizev1alpha3.OtterizeEgressNetworkPolicyNameTemplate, intent.GetServerFullyQualifiedName(intentsObj.Namespace), intentsObj.GetServiceName())
	existingPolicy := &v1.NetworkPolicy{}
	newPolicy := r.buildNetworkPolicyObjectForIntents(intentsObj, intent, policyName)
	err := r.Get(ctx, types.NamespacedName{
		Name:      policyName,
		Namespace: intentsObjNamespace},
		existingPolicy)
	if err != nil && !k8serrors.IsNotFound(err) {
		r.RecordWarningEventf(existingPolicy, consts.ReasonGettingEgressNetworkPolicyFailed, "failed to get network policy: %s", err.Error())
		return false, errors.Wrap(err)
	}

	if k8serrors.IsNotFound(err) {
		return true, r.CreateNetworkPolicy(ctx, intentsObjNamespace, intent, newPolicy)
	}

	return true, r.UpdateExistingPolicy(ctx, existingPolicy, newPolicy, intent, intentsObjNamespace)
}

func (r *EgressNetworkPolicyReconciler) UpdateExistingPolicy(ctx context.Context, existingPolicy *v1.NetworkPolicy, newPolicy *v1.NetworkPolicy, intent otterizev1alpha3.Intent, intentsObjNamespace string) error {
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

func (r *EgressNetworkPolicyReconciler) CreateNetworkPolicy(ctx context.Context, intentsObjNamespace string, intent otterizev1alpha3.Intent, newPolicy *v1.NetworkPolicy) error {
	logrus.Infof(
		"Creating network policy to enable access from namespace %s to %s", intentsObjNamespace, intent.Name)
	return r.Create(ctx, newPolicy)
}

func (r *EgressNetworkPolicyReconciler) cleanPolicies(
	ctx context.Context, intents *otterizev1alpha3.ClientIntents) error {
	logrus.Infof("Removing network policies for deleted intents for service: %s", intents.Spec.Service.Name)
	for _, intent := range intents.GetCallsList() {
		err := r.handleIntentRemoval(ctx, intent, *intents)
		if err != nil {
			return errors.Wrap(err)
		}
	}

	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesDeleted, len(intents.GetCallsList()))
	prometheus.IncrementNetpolDeleted(len(intents.GetCallsList()))

	if err := r.Update(ctx, intents); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (r *EgressNetworkPolicyReconciler) handleIntentRemoval(
	ctx context.Context,
	intent otterizev1alpha3.Intent,
	intentsObj otterizev1alpha3.ClientIntents) error {

	logrus.Infof("No other intents in the namespace reference target server: %s", intent.Name)
	logrus.Infoln("Removing matching network policy for server")
	return r.deleteNetworkPolicy(ctx, intent, intentsObj)
}

func (r *EgressNetworkPolicyReconciler) removeOrphanNetworkPolicies(ctx context.Context) error {
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
		// Get all client intents that reference this network policy
		var intentsList otterizev1alpha3.ClientIntentsList
		formattedServerName := networkPolicy.Labels[otterizev1alpha3.OtterizeEgressNetworkPolicyTarget]
		clientNamespace := networkPolicy.Namespace
		err = r.List(
			ctx,
			&intentsList,
			&client.MatchingFields{otterizev1alpha3.OtterizeFormattedTargetServerIndexField: formattedServerName},
			&client.ListOptions{Namespace: clientNamespace},
		)
		if err != nil {
			return errors.Wrap(err)
		}

		if len(intentsList.Items) == 0 {
			logrus.Infof("Removing orphaned network policy: %s server %s ns %s", networkPolicy.Name, formattedServerName, networkPolicy.Namespace)
			err = r.removeNetworkPolicy(ctx, networkPolicy)
			if err != nil {
				return errors.Wrap(err)
			}
		}
	}

	return nil
}

func (r *EgressNetworkPolicyReconciler) removeNetworkPolicy(ctx context.Context, networkPolicy v1.NetworkPolicy) error {
	return r.Delete(ctx, &networkPolicy)
}

func matchAccessNetworkPolicy() (labels.Selector, error) {
	isOtterizeNetworkPolicy := metav1.LabelSelectorRequirement{
		Key:      otterizev1alpha3.OtterizeEgressNetworkPolicy,
		Operator: metav1.LabelSelectorOpExists,
	}
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		isOtterizeNetworkPolicy,
	}})
}

func (r *EgressNetworkPolicyReconciler) deleteNetworkPolicy(
	ctx context.Context,
	intent otterizev1alpha3.Intent,
	intentsObj otterizev1alpha3.ClientIntents) error {

	policyName := fmt.Sprintf(otterizev1alpha3.OtterizeEgressNetworkPolicyNameTemplate, intent.GetServerFullyQualifiedName(intentsObj.Namespace), intentsObj.GetServiceName())
	policy := &v1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: policyName, Namespace: intent.GetTargetServerNamespace(intentsObj.Namespace)}, policy)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrap(err)
	}

	return r.removeNetworkPolicy(ctx, *policy)
}

// buildNetworkPolicyObjectForIntents builds the network policy that represents the intent from the parameter
func (r *EgressNetworkPolicyReconciler) buildNetworkPolicyObjectForIntents(
	intentsObj *otterizev1alpha3.ClientIntents, intent otterizev1alpha3.Intent, policyName string) *v1.NetworkPolicy {
	// The intent's target server made of name + namespace + hash
	formattedClient := otterizev1alpha3.GetFormattedOtterizeIdentity(intentsObj.GetServiceName(), intentsObj.Namespace)
	formattedTargetServer := otterizev1alpha3.GetFormattedOtterizeIdentity(intent.GetTargetServerName(), intent.GetTargetServerNamespace(intentsObj.Namespace))
	podSelector := r.buildPodLabelSelectorFromIntents(intentsObj)
	return &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: intentsObj.Namespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeEgressNetworkPolicy:       formattedClient,
				otterizev1alpha3.OtterizeEgressNetworkPolicyTarget: formattedTargetServer,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			PodSelector: podSelector,
			Egress: []v1.NetworkPolicyEgressRule{
				{
					To: []v1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									otterizev1alpha3.OtterizeServerLabelKey: formattedTargetServer,
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									otterizev1alpha3.KubernetesStandardNamespaceNameLabelKey: intent.GetTargetServerNamespace(intentsObj.Namespace),
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *EgressNetworkPolicyReconciler) buildPodLabelSelectorFromIntents(intentsObj *otterizev1alpha3.ClientIntents) metav1.LabelSelector {
	// The intent's target server made of name + namespace + hash
	formattedClient := otterizev1alpha3.GetFormattedOtterizeIdentity(intentsObj.GetServiceName(), intentsObj.Namespace)

	return metav1.LabelSelector{
		MatchLabels: map[string]string{
			otterizev1alpha3.OtterizeClientLabelKey: formattedClient,
		},
	}
}
