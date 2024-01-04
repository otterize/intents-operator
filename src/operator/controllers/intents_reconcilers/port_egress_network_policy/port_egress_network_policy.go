package port_egress_network_policy

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
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The PortEgressNetworkPolicyReconciler creates network policies that allow egress traffic from pods to specific ports,
// based on which Kubernetes service is specified in the intents.
type PortEgressNetworkPolicyReconciler struct {
	client.Client
	Scheme                      *runtime.Scheme
	RestrictToNamespaces        []string
	enableNetworkPolicyCreation bool
	enforcementDefaultState     bool
	injectablerecorder.InjectableRecorder
}

func NewPortEgressNetworkPolicyReconciler(
	c client.Client,
	s *runtime.Scheme,
	restrictToNamespaces []string,
	enableNetworkPolicyCreation bool,
	enforcementDefaultState bool) *PortEgressNetworkPolicyReconciler {
	return &PortEgressNetworkPolicyReconciler{
		Client:                      c,
		Scheme:                      s,
		RestrictToNamespaces:        restrictToNamespaces,
		enableNetworkPolicyCreation: enableNetworkPolicyCreation,
		enforcementDefaultState:     enforcementDefaultState,
	}
}

func (r *PortEgressNetworkPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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
			r.RecordWarningEventf(intents, consts.ReasonRemovingEgressNetworkPolicyFailed, "could not remove network policies: %s", err.Error())
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	createdNetpols := 0
	for _, intent := range intents.GetCallsList() {
		if intent.Type != "" && intent.Type != otterizev1alpha3.IntentTypeHTTP && intent.Type != otterizev1alpha3.IntentTypeKafka {
			continue
		}
		if !intent.IsTargetServerKubernetesService() {
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
			return ctrl.Result{}, err
		}
		if createdPolicies {
			createdNetpols += 1
		}
	}

	err = r.removeOrphanNetworkPolicies(ctx)
	if err != nil {
		r.RecordWarningEventf(intents, consts.ReasonRemovingEgressNetworkPolicyFailed, "failed to remove network policies: %s", err.Error())
		return ctrl.Result{}, err
	}

	if createdNetpols != 0 {
		callsCount := len(intents.GetCallsList())
		r.RecordNormalEventf(intents, consts.ReasonCreatedEgressNetworkPolicies, "NetworkPolicy reconcile complete, reconciled %d servers", callsCount)
		telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesCreated, createdNetpols)
		prometheus.IncrementNetpolCreated(createdNetpols)

	}
	return ctrl.Result{}, nil
}

func (r *PortEgressNetworkPolicyReconciler) handleNetworkPolicyCreation(
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

	svc := corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: intent.GetTargetServerName(), Namespace: intent.GetTargetServerNamespace(intentsObjNamespace)}, &svc)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	existingPolicy := &v1.NetworkPolicy{}
	policyName := fmt.Sprintf(otterizev1alpha3.OtterizeSvcEgressNetworkPolicyNameTemplate, intent.GetServerFullyQualifiedName(intentsObj.Namespace), intentsObj.GetServiceName())
	newPolicy, err := r.buildNetworkPolicyObjectForIntents(ctx, &svc, intentsObj, intent, policyName)
	if err != nil {
		return false, err
	}
	err = r.Get(ctx, types.NamespacedName{
		Name:      policyName,
		Namespace: intentsObjNamespace},
		existingPolicy)
	if err != nil && !k8serrors.IsNotFound(err) {
		r.RecordWarningEventf(existingPolicy, consts.ReasonGettingEgressNetworkPolicyFailed, "failed to get network policy: %s", err.Error())
		return false, err
	}

	if k8serrors.IsNotFound(err) {
		return true, r.CreateNetworkPolicy(ctx, intentsObjNamespace, intent, newPolicy)
	}

	return true, r.UpdateExistingPolicy(ctx, existingPolicy, newPolicy, intent, intentsObjNamespace)
}

func (r *PortEgressNetworkPolicyReconciler) UpdateExistingPolicy(ctx context.Context, existingPolicy *v1.NetworkPolicy, newPolicy *v1.NetworkPolicy, intent otterizev1alpha3.Intent, intentsObjNamespace string) error {
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

func (r *PortEgressNetworkPolicyReconciler) CreateNetworkPolicy(ctx context.Context, intentsObjNamespace string, intent otterizev1alpha3.Intent, newPolicy *v1.NetworkPolicy) error {
	logrus.Infof(
		"Creating network policy to enable access from namespace %s to %s", intentsObjNamespace, intent.Name)
	return r.Create(ctx, newPolicy)
}

func (r *PortEgressNetworkPolicyReconciler) cleanPolicies(
	ctx context.Context, intents *otterizev1alpha3.ClientIntents) error {
	logrus.Infof("Removing network policies for deleted intents for service: %s", intents.Spec.Service.Name)
	for _, intent := range intents.GetCallsList() {
		err := r.handleIntentRemoval(ctx, intent, *intents)
		if err != nil {
			return err
		}
	}

	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesDeleted, len(intents.GetCallsList()))
	prometheus.IncrementNetpolCreated(len(intents.GetCallsList()))

	if err := r.Update(ctx, intents); err != nil {
		return err
	}

	return nil
}

func (r *PortEgressNetworkPolicyReconciler) handleIntentRemoval(
	ctx context.Context,
	intent otterizev1alpha3.Intent,
	intentsObj otterizev1alpha3.ClientIntents) error {

	logrus.Infof("No other intents in the namespace reference target server: %s", intent.Name)
	logrus.Infoln("Removing matching network policy for server")
	return r.deleteNetworkPolicy(ctx, intent, intentsObj)
}

func (r *PortEgressNetworkPolicyReconciler) removeOrphanNetworkPolicies(ctx context.Context) error {
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
		var intentsList otterizev1alpha3.ClientIntentsList
		serverName := fmt.Sprintf("svc:%s", networkPolicy.Labels[otterizev1alpha3.OtterizeSvcEgressNetworkPolicyTarget])
		clientNamespace := networkPolicy.Namespace
		err = r.List(
			ctx,
			&intentsList,
			&client.MatchingFields{otterizev1alpha3.OtterizeFormattedTargetServerIndexField: serverName},
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

func (r *PortEgressNetworkPolicyReconciler) removeNetworkPolicy(ctx context.Context, networkPolicy v1.NetworkPolicy) error {
	return r.Delete(ctx, &networkPolicy)
}

func matchAccessNetworkPolicy() (labels.Selector, error) {
	isOtterizeNetworkPolicy := metav1.LabelSelectorRequirement{
		Key:      otterizev1alpha3.OtterizeSvcEgressNetworkPolicy,
		Operator: metav1.LabelSelectorOpExists,
	}
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		isOtterizeNetworkPolicy,
	}})
}

func (r *PortEgressNetworkPolicyReconciler) deleteNetworkPolicy(
	ctx context.Context,
	intent otterizev1alpha3.Intent,
	intentsObj otterizev1alpha3.ClientIntents) error {

	policyName := fmt.Sprintf(otterizev1alpha3.OtterizeSvcEgressNetworkPolicyNameTemplate, intent.GetServerFullyQualifiedName(intentsObj.Namespace), intentsObj.GetServiceName())
	policy := &v1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: policyName, Namespace: intent.GetTargetServerNamespace(intentsObj.Namespace)}, policy)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return r.removeNetworkPolicy(ctx, *policy)
}

// buildNetworkPolicyObjectForIntents builds the network policy that represents the intent from the parameter
func (r *PortEgressNetworkPolicyReconciler) buildNetworkPolicyObjectForIntents(ctx context.Context, svc *corev1.Service, intentsObj *otterizev1alpha3.ClientIntents, intent otterizev1alpha3.Intent, policyName string) (*v1.NetworkPolicy, error) {
	// The intent's target server made of name + namespace + hash
	formattedClient := otterizev1alpha3.GetFormattedOtterizeIdentity(intentsObj.GetServiceName(), intentsObj.Namespace)
	formattedTargetServer := otterizev1alpha3.GetFormattedOtterizeIdentity(intent.GetTargetServerName(), intent.GetTargetServerNamespace(intentsObj.Namespace))
	clientPodSelector := r.buildPodLabelSelectorFromIntents(intentsObj)
	var egressRule v1.NetworkPolicyEgressRule
	var err error
	if svc.Spec.Selector != nil {
		egressRule = getPodSelectorRule(svc, intentsObj, intent)
	} else if intent.IsTargetTheKubernetesAPIServer(intentsObj.Namespace) {
		egressRule, err = r.getIPRuleFromEndpoint(ctx, svc)
		if err != nil {
			return nil, errors.Wrap(err)
		}
	} else {
		return nil, fmt.Errorf("service %s/%s has no selector", svc.Namespace, svc.Name)
	}

	netpol := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: intentsObj.Namespace,
			Annotations: map[string]string{
				otterizev1alpha3.OtterizeSvcEgressNetworkPolicyTargetService:          svc.Name,
				otterizev1alpha3.OtterizeSvcEgressNetworkPolicyTargetServiceNamespace: svc.Namespace,
			},
			Labels: map[string]string{
				otterizev1alpha3.OtterizeSvcEgressNetworkPolicy:       formattedClient,
				otterizev1alpha3.OtterizeSvcEgressNetworkPolicyTarget: formattedTargetServer,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			PodSelector: clientPodSelector,
			Egress:      []v1.NetworkPolicyEgressRule{egressRule},
		},
	}

	return netpol, nil
}

func getPodSelectorRule(svc *corev1.Service, intentsObj *otterizev1alpha3.ClientIntents, intent otterizev1alpha3.Intent) v1.NetworkPolicyEgressRule {
	svcPodSelector := metav1.LabelSelector{MatchLabels: svc.Spec.Selector}
	podSelectorEgressRule := v1.NetworkPolicyEgressRule{
		To: []v1.NetworkPolicyPeer{
			{
				PodSelector: &svcPodSelector,
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						otterizev1alpha3.KubernetesStandardNamespaceNameLabelKey: intent.GetTargetServerNamespace(intentsObj.Namespace),
					},
				},
			},
		},
	}

	portToProtocol := make(map[int]corev1.Protocol)
	// Gather all target ports (target ports in the pod the service proxies to)
	for _, port := range svc.Spec.Ports {
		if port.TargetPort.StrVal != "" {
			continue
		}
		portToProtocol[port.TargetPort.IntValue()] = port.Protocol
	}

	networkPolicyPorts := make([]v1.NetworkPolicyPort, 0)
	// Create a list of network policy ports
	for port, protocol := range portToProtocol {
		netpolPort := v1.NetworkPolicyPort{
			Port: &intstr.IntOrString{IntVal: int32(port)},
		}
		if len(protocol) != 0 {
			netpolPort.Protocol = lo.ToPtr(protocol)
		}
		networkPolicyPorts = append(networkPolicyPorts, netpolPort)
	}

	podSelectorEgressRule.Ports = networkPolicyPorts

	return podSelectorEgressRule
}

func (r *PortEgressNetworkPolicyReconciler) getIPRuleFromEndpoint(ctx context.Context, svc *corev1.Service) (v1.NetworkPolicyEgressRule, error) {
	ipAddresses := make([]string, 0)
	ports := make([]v1.NetworkPolicyPort, 0)

	var endpoint corev1.Endpoints
	err := r.Client.Get(ctx, types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, &endpoint)
	if err != nil {
		return v1.NetworkPolicyEgressRule{}, errors.Wrap(err)
	}

	if len(endpoint.Subsets) == 0 {
		return v1.NetworkPolicyEgressRule{}, fmt.Errorf("no endpoints found for service %s/%s", svc.Namespace, svc.Name)
	}

	for _, subset := range endpoint.Subsets {
		for _, address := range subset.Addresses {
			ipAddresses = append(ipAddresses, address.IP)
		}
		for _, port := range subset.Ports {
			ports = append(ports, v1.NetworkPolicyPort{
				Port:     &intstr.IntOrString{IntVal: port.Port},
				Protocol: lo.ToPtr(port.Protocol),
			})
		}
	}

	if len(ipAddresses) == 0 {
		return v1.NetworkPolicyEgressRule{}, fmt.Errorf("no endpoints found for service %s/%s", svc.Namespace, svc.Name)
	}

	podSelectorEgressRule := v1.NetworkPolicyEgressRule{}
	for _, ip := range ipAddresses {
		podSelectorEgressRule.To = append(podSelectorEgressRule.To, v1.NetworkPolicyPeer{
			IPBlock: &v1.IPBlock{
				CIDR:   fmt.Sprintf("%s/32", ip),
				Except: nil,
			},
		})
	}

	if len(ports) > 0 {
		podSelectorEgressRule.Ports = ports
	}

	return podSelectorEgressRule, nil
}

func (r *PortEgressNetworkPolicyReconciler) buildPodLabelSelectorFromIntents(intentsObj *otterizev1alpha3.ClientIntents) metav1.LabelSelector {
	// The intent's target server made of name + namespace + hash
	formattedClient := otterizev1alpha3.GetFormattedOtterizeIdentity(intentsObj.GetServiceName(), intentsObj.Namespace)

	return metav1.LabelSelector{
		MatchLabels: map[string]string{
			otterizev1alpha3.OtterizeClientLabelKey: formattedClient,
		},
	}
}
