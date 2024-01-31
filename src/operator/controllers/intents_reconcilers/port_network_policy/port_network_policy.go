package port_network_policy

import (
	"context"
	goerrors "errors"
	"fmt"
	"github.com/amit7itz/goset"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var ErrorTypeStringPortNotSupported = errors.New("port of type string is not supported")

type externalNetpolHandler interface {
	HandlePodsByLabelSelector(ctx context.Context, namespace string, labelSelector labels.Selector) error
	HandleBeforeAccessPolicyRemoval(ctx context.Context, accessPolicy *v1.NetworkPolicy) error
}

type PortNetworkPolicyReconciler struct {
	client.Client
	Scheme                      *runtime.Scheme
	extNetpolHandler            externalNetpolHandler
	RestrictToNamespaces        []string
	enableNetworkPolicyCreation bool
	enforcementDefaultState     bool
	injectablerecorder.InjectableRecorder
}

func NewPortNetworkPolicyReconciler(
	c client.Client,
	s *runtime.Scheme,
	extNetpolHandler externalNetpolHandler,
	restrictToNamespaces []string,
	enableNetworkPolicyCreation bool,
	enforcementDefaultState bool,
) *PortNetworkPolicyReconciler {
	return &PortNetworkPolicyReconciler{
		Client:                      c,
		Scheme:                      s,
		extNetpolHandler:            extNetpolHandler,
		RestrictToNamespaces:        restrictToNamespaces,
		enableNetworkPolicyCreation: enableNetworkPolicyCreation,
		enforcementDefaultState:     enforcementDefaultState,
	}
}

func (r *PortNetworkPolicyReconciler) ReconcileEffectivePolicies(ctx context.Context, eps []effectivepolicy.ServiceEffectivePolicy) (int, error) {
	currentPolicies := goset.NewSet[types.NamespacedName]()
	errorList := make([]error, 0)
	for _, ep := range eps {
		netpols, err := r.applyServiceEffectivePolicy(ctx, ep)
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

	return currentPolicies.Len(), errors.Wrap(err)

}

func (r *PortNetworkPolicyReconciler) applyServiceEffectivePolicy(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]types.NamespacedName, error) {
	logrus.Infof("Reconciling network policies for service %s in namespace %s",
		ep.Service.Name, ep.Service.Namespace)

	networkPolicies := make([]types.NamespacedName, 0)
	for _, intent := range ep.Calls {
		if intent.Type != "" && intent.Type != otterizev1alpha3.IntentTypeHTTP && intent.Type != otterizev1alpha3.IntentTypeKafka {
			continue
		}
		if !intent.IsTargetServerKubernetesService() {
			continue
		}
		if intent.IsTargetTheKubernetesAPIServer(ep.Service.Namespace) {
			// Currently only egress is supported for the kubernetes API server
			continue
		}
		if !r.enableNetworkPolicyCreation {
			logrus.Infof("Network policy creation is disabled, skipping network policy creation for server %s in namespace %s", intent.GetTargetServerName(), intent.GetTargetServerNamespace(ep.Service.Namespace))
			ep.ClientIntentsEventRecorder.RecordNormalEvent(consts.ReasonNetworkPolicyCreationDisabled, "Network policy creation is disabled, creation skipped")
			continue
		}

		targetNamespace := intent.GetTargetServerNamespace(ep.Service.Namespace)
		if len(r.RestrictToNamespaces) != 0 && !lo.Contains(r.RestrictToNamespaces, targetNamespace) {
			// Namespace is not in list of namespaces we're allowed to act in, so drop it.
			ep.ClientIntentsEventRecorder.RecordWarningEventf(consts.ReasonNamespaceNotAllowed, "namespace %s was specified in intent, but is not allowed by configuration", targetNamespace)
			continue
		}
		netpol, created, err := r.handleNetworkPolicyCreation(ctx, ep, intent)
		if err != nil {
			ep.ClientIntentsEventRecorder.RecordWarningEventf(consts.ReasonCreatingNetworkPoliciesFailed, "could not create network policies: %s", err.Error())
			return nil, errors.Wrap(err)
		}
		if created {
			networkPolicies = append(networkPolicies, types.NamespacedName{Name: netpol.Name, Namespace: netpol.Namespace})
		}
	}

	if len(networkPolicies) != 0 {
		callsCount := len(ep.Calls)
		ep.ClientIntentsEventRecorder.RecordNormalEventf(consts.ReasonCreatedNetworkPolicies, "reconciled %d servers, created %d policies", callsCount, len(networkPolicies))
		telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesCreated, len(networkPolicies))
		prometheus.IncrementNetpolCreated(len(networkPolicies))

	}

	return networkPolicies, nil

}

func (r *PortNetworkPolicyReconciler) handleNetworkPolicyCreation(
	ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy, intent otterizev1alpha3.Intent) (*v1.NetworkPolicy, bool, error) {

	// TODO: Add protected service support
	//shouldCreatePolicy, err := protected_services.IsServerEnforcementEnabledDueToProtectionOrDefaultState(ctx, r.Client, intent.GetTargetServerName(), intent.GetTargetServerNamespace(intentsObjNamespace), r.enforcementDefaultState)
	//if err != nil {
	//	return nil, errors.Wrap(err)
	//}
	//
	//if !shouldCreatePolicy {
	//	logrus.Infof("Enforcement is disabled globally and server is not explicitly protected, skipping network policy creation for server %s in namespace %s", intent.GetTargetServerName(), intent.GetTargetServerNamespace(intentsObjNamespace))
	//	r.RecordNormalEventf(intentsObj, consts.ReasonEnforcementDefaultOff, "Enforcement is disabled globally and called service '%s' is not explicitly protected using a ProtectedService resource, network policy creation skipped", intent.Name)
	//	return nil, nil
	//}

	policyName := fmt.Sprintf(otterizev1alpha3.OtterizeServiceNetworkPolicyNameTemplate, intent.GetTargetServerName(), ep.Service.Namespace)
	existingPolicy := &v1.NetworkPolicy{}
	svc := corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: intent.GetTargetServerName(), Namespace: intent.GetTargetServerNamespace(ep.Service.Namespace)}, &svc)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return existingPolicy, false, nil
		}
		return nil, false, errors.Wrap(err)
	}
	newPolicy, err := r.buildNetworkPolicyObjectForIntent(&svc, intent, policyName, ep.Service.Namespace)
	if err != nil {
		return nil, false, errors.Wrap(err)
	}

	err = r.Get(ctx, types.NamespacedName{
		Name:      policyName,
		Namespace: intent.GetTargetServerNamespace(ep.Service.Namespace)},
		existingPolicy)
	if err != nil && !k8serrors.IsNotFound(err) {
		r.RecordWarningEventf(existingPolicy, consts.ReasonGettingNetworkPolicyFailed, "failed to get network policy: %s", err.Error())
		return nil, false, errors.Wrap(err)
	}

	if k8serrors.IsNotFound(err) {
		err = r.CreateNetworkPolicy(ctx, ep.Service.Namespace, intent, newPolicy)
		if err != nil {
			return nil, false, errors.Wrap(err)
		}
		return newPolicy, true, nil
	}

	err = r.UpdateExistingPolicy(ctx, existingPolicy, newPolicy, intent, ep.Service.Namespace)
	if err != nil {
		return nil, false, errors.Wrap(err)
	}
	return newPolicy, true, nil
}

func (r *PortNetworkPolicyReconciler) UpdateExistingPolicy(ctx context.Context, existingPolicy *v1.NetworkPolicy, newPolicy *v1.NetworkPolicy, intent otterizev1alpha3.Intent, intentsObjNamespace string) error {
	if reflect.DeepEqual(existingPolicy.Spec, newPolicy.Spec) {
		return nil
	}
	policyCopy := existingPolicy.DeepCopy()
	policyCopy.Labels = newPolicy.Labels
	policyCopy.Annotations = newPolicy.Annotations
	policyCopy.Spec = newPolicy.Spec
	policyCopy.SetOwnerReferences(newPolicy.GetOwnerReferences())

	return errors.Wrap(r.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy)))
}

func (r *PortNetworkPolicyReconciler) CreateNetworkPolicy(ctx context.Context, intentsObjNamespace string, intent otterizev1alpha3.Intent, newPolicy *v1.NetworkPolicy) error {
	logrus.Infof(
		"Creating network policy to enable access from namespace %s to %s", intentsObjNamespace, intent.Name)
	err := r.Create(ctx, newPolicy)
	if err != nil {
		return errors.Wrap(err)
	}

	return r.reconcileEndpointsForPolicy(ctx, newPolicy)
}

func (r *PortNetworkPolicyReconciler) reconcileEndpointsForPolicy(ctx context.Context, newPolicy *v1.NetworkPolicy) error {
	selector, err := metav1.LabelSelectorAsSelector(&newPolicy.Spec.PodSelector)
	if err != nil {
		return errors.Wrap(err)
	}
	// Use the external netpolHandler to check if pods got affected and if so, if they need external allow policies
	return r.extNetpolHandler.HandlePodsByLabelSelector(ctx, newPolicy.Namespace, selector)
}

func (r *PortNetworkPolicyReconciler) removeNetworkPoliciesThatShouldNotExist(ctx context.Context, netpolNamesThatShouldExist *goset.Set[types.NamespacedName]) error {
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
	deleted := 0
	for _, networkPolicy := range networkPolicyList.Items {
		namespacedName := types.NamespacedName{Namespace: networkPolicy.Namespace, Name: networkPolicy.Name}
		if !netpolNamesThatShouldExist.Contains(namespacedName) {
			serverName := networkPolicy.Labels[otterizev1alpha3.OtterizeSvcEgressNetworkPolicy]
			logrus.Infof("Removing orphaned network policy: %s server %s ns %s", networkPolicy.Name, serverName, networkPolicy.Namespace)
			err = r.removeNetworkPolicy(ctx, networkPolicy)
			if err != nil {
				return errors.Wrap(err)
			}
			deleted += 1
		}
	}

	if deleted > 0 {
		telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesDeleted, deleted)
		prometheus.IncrementNetpolCreated(deleted)
	}

	return nil
}

func (r *PortNetworkPolicyReconciler) removeNetworkPolicy(ctx context.Context, networkPolicy v1.NetworkPolicy) error {
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
		Key:      otterizev1alpha3.OtterizeSvcNetworkPolicy,
		Operator: metav1.LabelSelectorOpExists,
	}
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		isOtterizeNetworkPolicy,
	}})
}

// buildNetworkPolicyObjectForIntent builds the network policy that represents the intent from the parameter
func (r *PortNetworkPolicyReconciler) buildNetworkPolicyObjectForIntent(
	svc *corev1.Service, intent otterizev1alpha3.Intent, policyName, intentsObjNamespace string) (*v1.NetworkPolicy, error) {
	targetNamespace := intent.GetTargetServerNamespace(intentsObjNamespace)
	// The intent's target server made of name + namespace + hash
	formattedTargetServer := otterizev1alpha3.GetFormattedOtterizeIdentity(intent.GetTargetServerName(), targetNamespace)
	if svc.Spec.Selector == nil {
		return nil, fmt.Errorf("service %s/%s has no selector", svc.Namespace, svc.Name)
	}
	podSelector := metav1.LabelSelector{MatchLabels: svc.Spec.Selector}

	netpol := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: targetNamespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeSvcNetworkPolicy: formattedTargetServer,
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
										otterizev1alpha3.OtterizeSvcAccessLabelKey, formattedTargetServer): "true",
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

	// Add ports to network policy spec
	netpol.Spec.Ingress[0].Ports = networkPolicyPorts

	err := controllerutil.SetOwnerReference(svc, netpol, r.Scheme)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	return netpol, nil
}
