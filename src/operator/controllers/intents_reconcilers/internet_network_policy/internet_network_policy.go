package internet_network_policy

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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"net"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
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

func (r *InternetNetworkPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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

	err = r.removeOrphanNetworkPolicies(ctx)
	if err != nil {
		r.RecordWarningEventf(intents, consts.ReasonRemovingEgressNetworkPolicyFailed, "failed to remove network policies: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	hasAnyInternetIntents := slices.ContainsFunc(intents.GetCallsList(), func(intent otterizev1alpha3.Intent) bool {
		return intent.Type == otterizev1alpha3.IntentTypeInternet
	})
	if !hasAnyInternetIntents {
		return ctrl.Result{}, nil
	}

	logrus.Infof("Reconciling internet network policies for service %s in namespace %s",
		intents.Spec.Service.Name, req.Namespace)

	if !intents.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, intents)
	}

	if len(r.RestrictToNamespaces) != 0 && !lo.Contains(r.RestrictToNamespaces, intents.Namespace) {
		r.RecordWarningEventf(intents, consts.ReasonNamespaceNotAllowed, "ClientIntents are in namespace %s but namespace is not allowed by configuration", intents.Namespace)
		return ctrl.Result{}, nil
	}

	if !r.enforcementDefaultState {
		logrus.Infof("Enforcement is disabled globally skipping internet network policy creation for service %s in namespace %s", intents.Spec.Service.Name, req.Namespace)
		r.RecordNormalEventf(intents, consts.ReasonEnforcementDefaultOff, "Enforcement is disabled globally, internet network policy creation skipped")
		return ctrl.Result{}, nil
	}
	if !r.enableNetworkPolicyCreation {
		logrus.Infof("Network policy creation is disabled, skipping internet network policy creation for service %s in namespace %s", intents.Spec.Service.Name, req.Namespace)
		r.RecordNormalEvent(intents, consts.ReasonEgressNetworkPolicyCreationDisabled, "Network policy creation is disabled, internet network policy creation skipped")
		return ctrl.Result{}, nil
	}

	err = r.handleNetworkPolicyCreation(ctx, intents, req.Namespace)
	if err != nil {
		r.RecordWarningEventf(intents, consts.ReasonCreatingEgressNetworkPoliciesFailed, "could not create network policies: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	r.RecordNormalEvent(intents, consts.ReasonCreatedInternetEgressNetworkPolicies, "InternetNetworkPolicy reconcile complete")
	prometheus.IncrementNetpolCreated(1)

	return ctrl.Result{}, nil
}

func (r *InternetNetworkPolicyReconciler) handleDeletion(ctx context.Context, intents *otterizev1alpha3.ClientIntents) (ctrl.Result, error) {
	err := r.cleanPolicies(ctx, intents)
	if err != nil && !k8serrors.IsConflict(err) {
		r.RecordWarningEventf(intents, consts.ReasonRemovingEgressNetworkPolicyFailed, "could not remove network policies: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}
	if k8serrors.IsConflict(err) {
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, nil
}

func (r *InternetNetworkPolicyReconciler) handleNetworkPolicyCreation(
	ctx context.Context,
	intentsObj *otterizev1alpha3.ClientIntents,
	intentsObjNamespace string,
) error {
	policyName := policyNameFor(intentsObj.GetServiceName())
	existingPolicy := &v1.NetworkPolicy{}
	newPolicy, err := r.buildNetworkPolicy(intentsObj, policyName)
	if err != nil {
		return errors.Wrap(err)
	}

	err = r.Get(ctx, types.NamespacedName{
		Name:      policyName,
		Namespace: intentsObjNamespace},
		existingPolicy)
	if err != nil && !k8serrors.IsNotFound(err) {
		r.RecordWarningEventf(existingPolicy, consts.ReasonGettingEgressNetworkPolicyFailed, "failed to get network policy: %s", err.Error())
		return errors.Wrap(err)
	}

	if k8serrors.IsNotFound(err) {
		return r.CreateNetworkPolicy(ctx, newPolicy)
	}

	return r.UpdateExistingPolicy(ctx, existingPolicy, newPolicy)
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

func (r *InternetNetworkPolicyReconciler) cleanPolicies(
	ctx context.Context, intents *otterizev1alpha3.ClientIntents) error {
	logrus.Infof("Removing internet network policy for deleted intents for service: %s", intents.Spec.Service.Name)
	err := r.deleteInternetNetworkPolicy(ctx, *intents)
	if err != nil {
		return errors.Wrap(err)
	}

	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesDeleted, 1)
	prometheus.IncrementNetpolDeleted(1)

	return nil
}

func (r *InternetNetworkPolicyReconciler) removeOrphanNetworkPolicies(ctx context.Context) error {
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
		// Get all client intents that reference this network policy
		var intentsList otterizev1alpha3.ClientIntentsList
		formattedServerName := networkPolicy.Labels[otterizev1alpha3.OtterizeInternetNetworkPolicy]
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

func (r *InternetNetworkPolicyReconciler) removeNetworkPolicy(ctx context.Context, networkPolicy v1.NetworkPolicy) error {
	err := r.Delete(ctx, &networkPolicy)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (r *InternetNetworkPolicyReconciler) deleteInternetNetworkPolicy(
	ctx context.Context,
	intentsObj otterizev1alpha3.ClientIntents) error {
	policyName := policyNameFor(intentsObj.GetServiceName())
	policy := &v1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: policyName, Namespace: intentsObj.Namespace}, policy)
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Wrap(err)
	}
	if k8serrors.IsNotFound(err) {
		return nil
	}

	return r.removeNetworkPolicy(ctx, *policy)
}

func (r *InternetNetworkPolicyReconciler) buildNetworkPolicy(
	intentsObj *otterizev1alpha3.ClientIntents,
	policyName string,
) (*v1.NetworkPolicy, error) {
	// The intent's target server made of name + namespace + hash
	formattedClient := otterizev1alpha3.GetFormattedOtterizeIdentity(intentsObj.GetServiceName(), intentsObj.Namespace)
	podSelector := r.buildPodLabelSelectorFromIntents(intentsObj)

	rules := make([]v1.NetworkPolicyEgressRule, 0)

	// Get all intents to the internet
	intents := lo.Filter(intentsObj.GetCallsList(), func(intent otterizev1alpha3.Intent, _ int) bool {
		return intent.Type == otterizev1alpha3.IntentTypeInternet
	})

	for _, intent := range intents {
		peers, err := r.parseIps(intent)
		if err != nil {
			return nil, errors.Wrap(err)
		}
		ports := r.parsePorts(intent)
		rules = append(rules, v1.NetworkPolicyEgressRule{
			To:    peers,
			Ports: ports,
		})
	}

	return &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: intentsObj.Namespace,
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

func (r *InternetNetworkPolicyReconciler) parseIps(intent otterizev1alpha3.Intent) ([]v1.NetworkPolicyPeer, error) {
	var cidrs []string
	for _, ip := range intent.Internet.Ips {
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

func (r *InternetNetworkPolicyReconciler) buildPodLabelSelectorFromIntents(intentsObj *otterizev1alpha3.ClientIntents) metav1.LabelSelector {
	// The intent's target server made of name + namespace + hash
	formattedClient := otterizev1alpha3.GetFormattedOtterizeIdentity(intentsObj.GetServiceName(), intentsObj.Namespace)

	return metav1.LabelSelector{
		MatchLabels: map[string]string{
			otterizev1alpha3.OtterizeClientLabelKey: formattedClient,
		},
	}
}
