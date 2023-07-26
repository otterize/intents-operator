package external_traffic

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"
)

const (
	ReasonEnforcementGloballyDisabled         = "EnforcementGloballyDisabled"
	ReasonCreatingExternalTrafficPolicyFailed = "CreatingExternalTrafficPolicyFailed"
	ReasonCreatedExternalTrafficPolicy        = "CreatedExternalTrafficPolicy"
	ReasonGettingExternalTrafficPolicyFailed  = "GettingExternalTrafficPolicyFailed"
	ReasonRemovingExternalTrafficPolicy       = "RemovingExternalTrafficPolicy"
	ReasonRemovingExternalTrafficPolicyFailed = "RemovingExternalTrafficPolicyFailed"
	ReasonRemovedExternalTrafficPolicy        = "RemovedExternalTrafficPolicy"
	OtterizeExternalNetworkPolicyNameTemplate = "external-access-to-%s"
	successMsgNetpolCreate                    = "created external traffic network policy. service '%s' refers to pods protected by network policy '%s'"
)

type NetworkPolicyHandler struct {
	client client.Client
	scheme *runtime.Scheme
	injectablerecorder.InjectableRecorder
	enabled                                bool
	createEvenIfNoPreexistingNetworkPolicy bool
	enforcementEnabledGlobally             bool
}

func NewNetworkPolicyHandler(client client.Client, scheme *runtime.Scheme, enabled bool, createEvenIfNoPreexistingNetworkPolicy bool, enforcementEnabledGlobally bool) *NetworkPolicyHandler {
	return &NetworkPolicyHandler{client: client, scheme: scheme, enabled: enabled, createEvenIfNoPreexistingNetworkPolicy: createEvenIfNoPreexistingNetworkPolicy, enforcementEnabledGlobally: enforcementEnabledGlobally}
}

func (r *NetworkPolicyHandler) createOrUpdateNetworkPolicy(
	ctx context.Context, endpoints *corev1.Endpoints, owner *corev1.Service, otterizeServiceName string, selector metav1.LabelSelector, ingressList *v1.IngressList, successMsg string) error {
	policyName := r.formatPolicyName(endpoints.Name)
	newPolicy := buildNetworkPolicyObjectForEndpoints(endpoints, otterizeServiceName, selector, ingressList, policyName)
	err := controllerutil.SetOwnerReference(owner, newPolicy, r.scheme)
	if err != nil {
		return err
	}
	existingPolicy := &v1.NetworkPolicy{}
	errGetExistingPolicy := r.client.Get(ctx, types.NamespacedName{Name: policyName, Namespace: endpoints.GetNamespace()}, existingPolicy)

	// No matching network policy found, create one
	if k8serrors.IsNotFound(errGetExistingPolicy) {
		logrus.Infof("Creating network policy to allow external traffic to %s (ns %s)", endpoints.GetName(), endpoints.GetNamespace())
		err := r.client.Create(ctx, newPolicy)
		if err != nil {
			r.RecordWarningEventf(owner, ReasonCreatingExternalTrafficPolicyFailed, "failed to create external traffic network policy: %s", err.Error())
			return err
		}
		r.RecordNormalEvent(owner, ReasonCreatedExternalTrafficPolicy, successMsg)
		return nil
	} else if errGetExistingPolicy != nil {
		r.RecordWarningEventf(owner, ReasonGettingExternalTrafficPolicyFailed, "failed to get external traffic network policy: %s", err.Error())
		return errGetExistingPolicy
	}

	// Found matching policy, is an update needed?
	if r.arePoliciesEqual(existingPolicy, newPolicy) {
		return nil
	}

	return r.updatePolicy(ctx, existingPolicy, newPolicy, owner)

}

func (r *NetworkPolicyHandler) updatePolicy(ctx context.Context, existingPolicy *v1.NetworkPolicy, newPolicy *v1.NetworkPolicy, owner *corev1.Service) error {
	policyCopy := existingPolicy.DeepCopy()
	policyCopy.Labels = newPolicy.Labels
	policyCopy.Annotations = newPolicy.Annotations
	policyCopy.Spec = newPolicy.Spec
	err := controllerutil.SetOwnerReference(owner, policyCopy, r.scheme)
	if err != nil {
		return err
	}

	err = r.client.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy))
	if err != nil {
		return err
	}
	return nil
}

func (r *NetworkPolicyHandler) arePoliciesEqual(existingPolicy *v1.NetworkPolicy, newPolicy *v1.NetworkPolicy) bool {
	return reflect.DeepEqual(existingPolicy.Spec, newPolicy.Spec) &&
		reflect.DeepEqual(existingPolicy.Labels, newPolicy.Labels) &&
		reflect.DeepEqual(existingPolicy.Annotations, newPolicy.Annotations) &&
		reflect.DeepEqual(existingPolicy.OwnerReferences, newPolicy.OwnerReferences)
}

func buildNetworkPolicyObjectForEndpoints(
	endpoints *corev1.Endpoints, otterizeServiceName string, selector metav1.LabelSelector, ingressList *v1.IngressList, policyName string) *v1.NetworkPolicy {
	serviceSpecCopy := endpoints.Subsets

	annotations := map[string]string{
		v1alpha2.OtterizeCreatedForServiceAnnotation: endpoints.GetName(),
	}

	if len(ingressList.Items) != 0 {
		annotations[v1alpha2.OtterizeCreatedForIngressAnnotation] = strings.Join(lo.Map(ingressList.Items, func(ingress v1.Ingress, _ int) string {
			return ingress.Name
		}), ",")
	}

	netpol := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: endpoints.Namespace,
			Labels: map[string]string{
				v1alpha2.OtterizeNetworkPolicyExternalTraffic: otterizeServiceName,
			},
			Annotations: annotations,
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: selector,
			Ingress: []v1.NetworkPolicyIngressRule{
				{},
			},
		},
	}

	for _, subsets := range serviceSpecCopy {
		for _, port := range subsets.Ports {
			netpolPort := v1.NetworkPolicyPort{
				Port: lo.ToPtr(intstr.FromInt(int(port.Port))),
			}

			if len(port.Protocol) != 0 {
				netpolPort.Protocol = lo.ToPtr(port.Protocol)
			}
			netpol.Spec.Ingress[0].Ports = append(netpol.Spec.Ingress[0].Ports, netpolPort)
		}
	}

	return netpol
}

func (r *NetworkPolicyHandler) HandleEndpointsByName(ctx context.Context, serviceName string, namespace string) error {
	endpoints := &corev1.Endpoints{}
	err := r.client.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: namespace}, endpoints)
	if k8serrors.IsNotFound(err) {
		// delete is handled by garbage collection - the service owns the network policy
		return nil
	}

	if err != nil {
		return err
	}

	return r.HandleEndpoints(ctx, endpoints)
}

func (r *NetworkPolicyHandler) HandlePodsByLabelSelector(ctx context.Context, namespace string, labelSelector labels.Selector) error {
	podList := &corev1.PodList{}
	err := r.client.List(ctx, podList,
		&client.ListOptions{Namespace: namespace},
		client.MatchingLabelsSelector{Selector: labelSelector})
	if err != nil {
		return err
	}
	return r.handlePodList(ctx, podList)
}

func (r *NetworkPolicyHandler) handlePodList(ctx context.Context, podList *corev1.PodList) error {
	for _, pod := range podList.Items {
		err := r.handlePod(ctx, &pod)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *NetworkPolicyHandler) handlePod(ctx context.Context, pod *corev1.Pod) error {
	var endpointsList corev1.EndpointsList

	err := r.client.List(
		ctx,
		&endpointsList,
		&client.MatchingFields{v1alpha2.EndpointsPodNamesIndexField: pod.Name},
		&client.ListOptions{Namespace: pod.Namespace},
	)

	if err != nil {
		return err
	}
	for _, endpoints := range endpointsList.Items {
		if err := r.HandleEndpoints(ctx, &endpoints); err != nil {
			return err
		}
	}

	return nil

}

func (r *NetworkPolicyHandler) HandleEndpoints(ctx context.Context, endpoints *corev1.Endpoints) error {
	svc := &corev1.Service{}
	err := r.client.Get(ctx, types.NamespacedName{Name: endpoints.GetName(), Namespace: endpoints.GetNamespace()}, svc)
	if k8serrors.IsNotFound(err) {
		// delete is handled by garbage collection - the service owns the network policy
		return nil
	}

	if err != nil {
		return err
	}

	ingressList, err := r.getIngressRefersToService(ctx, svc)
	if err != nil {
		return err
	}
	// If it's not a load balancer or a node port service, and the service is not referenced by any Ingress,
	// then there's nothing we need to do.
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer && svc.Spec.Type != corev1.ServiceTypeNodePort && len(ingressList.Items) == 0 {
		return r.handlePolicyDelete(ctx, r.formatPolicyName(svc.Name), svc.Namespace)
	}

	return r.handleEndpointsWithIngressList(ctx, endpoints, ingressList)
}

func (r *NetworkPolicyHandler) handleEndpointsWithIngressList(ctx context.Context, endpoints *corev1.Endpoints, ingressList *v1.IngressList) error {

	addresses := r.getAddressesFromEndpoints(endpoints)
	foundOtterizeNetpolsAffectingPods := false
	for _, address := range addresses {
		serverLabel, err := r.getOtterizeServerLabel(ctx, address)

		if err != nil {
			return err
		}
		// only act on pods affected by Otterize
		if len(serverLabel) == 0 {
			continue
		}

		netpolList := &v1.NetworkPolicyList{}
		// there's only ever one
		err = r.client.List(ctx, netpolList, client.MatchingLabels{v1alpha2.OtterizeNetworkPolicy: serverLabel}, client.Limit(1))
		if err != nil {
			if k8serrors.IsNotFound(err) {
				// only act on pods affected by Otterize policies - if they were not created yet,
				// the intents reconciler will call the endpoints reconciler once it does.
				continue
			}
			return err
		}

		if len(netpolList.Items) == 0 {
			if r.createEvenIfNoPreexistingNetworkPolicy {
				err := r.handleNetpolsForOtterizeServiceWithoutIntents(ctx, endpoints, serverLabel, ingressList)
				if err != nil {
					return err
				}
			}
			continue
		}

		foundOtterizeNetpolsAffectingPods = true
		err = r.handleNetpolsForOtterizeService(ctx, endpoints, serverLabel, ingressList, &netpolList.Items[0])
		if err != nil {
			return err
		}

	}

	if !foundOtterizeNetpolsAffectingPods && !r.createEvenIfNoPreexistingNetworkPolicy {
		policyName := r.formatPolicyName(endpoints.Name)
		err := r.handlePolicyDelete(ctx, policyName, endpoints.Namespace)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *NetworkPolicyHandler) getOtterizeServerLabel(ctx context.Context, address corev1.EndpointAddress) (string, error) {
	if address.TargetRef == nil || address.TargetRef.Kind != "Pod" {
		return "", nil
	}

	pod := &corev1.Pod{}
	err := r.client.Get(ctx, types.NamespacedName{Name: address.TargetRef.Name, Namespace: address.TargetRef.Namespace}, pod)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}

	serverLabel, ok := pod.Labels[v1alpha2.OtterizeServerLabelKey]
	if !ok {
		return "", nil
	}
	return serverLabel, nil
}

func (r *NetworkPolicyHandler) getAddressesFromEndpoints(endpoints *corev1.Endpoints) []corev1.EndpointAddress {
	addresses := make([]corev1.EndpointAddress, 0)
	for _, subset := range endpoints.Subsets {
		addresses = append(addresses, subset.Addresses...)
		addresses = append(addresses, subset.NotReadyAddresses...)

	}
	return addresses
}

func (r *NetworkPolicyHandler) getIngressRefersToService(ctx context.Context, svc *corev1.Service) (*v1.IngressList, error) {
	var ingressList v1.IngressList
	err := r.client.List(
		ctx, &ingressList,
		&client.MatchingFields{v1alpha2.IngressServiceNamesIndexField: svc.Name},
		&client.ListOptions{Namespace: svc.Namespace})

	if err != nil {
		return nil, err
	}

	return &ingressList, nil

}

func (r *NetworkPolicyHandler) handlePolicyDelete(ctx context.Context, policyName string, policyNamespace string) error {
	policy := &v1.NetworkPolicy{}
	err := r.client.Get(ctx, types.NamespacedName{Name: policyName, Namespace: policyNamespace}, policy)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// nothing to do
			return nil
		}

		return err
	}

	var owner *unstructured.Unstructured

	if len(policy.GetOwnerReferences()) > 0 {
		ownerRef := policy.GetOwnerReferences()[0]
		ownerObj := &unstructured.Unstructured{}
		ownerObj.SetAPIVersion(ownerRef.APIVersion)
		ownerObj.SetKind(ownerRef.Kind)
		err := r.client.Get(ctx, types.NamespacedName{Name: ownerRef.Name, Namespace: policyNamespace}, ownerObj)
		if err != nil {
			logrus.Infof("can't get the owner of %s. So no events will be recorded for the deletion", policyName)
		}
		owner = ownerObj
	}

	r.RecordNormalEvent(owner, ReasonRemovingExternalTrafficPolicy, "removing external traffic network policy, reconciler was disabled")

	err = r.client.Delete(ctx, policy)
	if err != nil {
		r.RecordWarningEventf(owner, ReasonRemovingExternalTrafficPolicyFailed, "failed removing external traffic network policy: %s", err.Error())
		return err
	}
	r.RecordNormalEvent(owner, ReasonRemovedExternalTrafficPolicy, "removed external traffic network policy, success")

	return nil
}

func (r *NetworkPolicyHandler) handleNetpolsForOtterizeService(ctx context.Context, endpoints *corev1.Endpoints, otterizeServiceName string, ingressList *v1.IngressList, netpol *v1.NetworkPolicy) error {
	svc := &corev1.Service{}
	err := r.client.Get(ctx, types.NamespacedName{Name: endpoints.Name, Namespace: endpoints.Namespace}, svc)
	if err != nil {
		return err
	}

	// delete policy if disabled
	if !r.enforcementEnabledGlobally || !r.enabled {
		r.RecordNormalEventf(svc, ReasonEnforcementGloballyDisabled, "Skipping created external traffic network policy for service '%s' because enforcement is globally disabled", endpoints.GetName())
		err = r.handlePolicyDelete(ctx, r.formatPolicyName(endpoints.Name), endpoints.Namespace)
		if err != nil {
			return err
		}
		return nil
	}

	successMsg := fmt.Sprintf(successMsgNetpolCreate, endpoints.GetName(), netpol.GetName())
	err = r.createOrUpdateNetworkPolicy(ctx, endpoints, svc, otterizeServiceName, netpol.Spec.PodSelector, ingressList, successMsg)

	if err != nil {
		return err
	}
	return nil
}

func (r *NetworkPolicyHandler) handleNetpolsForOtterizeServiceWithoutIntents(ctx context.Context, endpoints *corev1.Endpoints, otterizeServiceName string, ingressList *v1.IngressList) error {
	svc := &corev1.Service{}
	err := r.client.Get(ctx, types.NamespacedName{Name: endpoints.Name, Namespace: endpoints.Namespace}, svc)
	if err != nil {
		return err
	}

	// delete policy if disabled
	if !r.enforcementEnabledGlobally || !r.enabled {
		r.RecordNormalEventf(svc, ReasonEnforcementGloballyDisabled, "Skipping created external traffic network policy for service '%s' because enforcement is globally disabled", endpoints.GetName())
		err = r.handlePolicyDelete(ctx, r.formatPolicyName(endpoints.Name), endpoints.Namespace)
		if err != nil {
			return err
		}
		return nil
	}

	err = r.createOrUpdateNetworkPolicy(ctx, endpoints, svc, otterizeServiceName, metav1.LabelSelector{MatchLabels: svc.Spec.Selector}, ingressList, fmt.Sprintf("created external traffic network policy for service '%s'", endpoints.GetName()))
	if err != nil {
		return err
	}
	return nil
}

func (r *NetworkPolicyHandler) formatPolicyName(serviceName string) string {
	return fmt.Sprintf(OtterizeExternalNetworkPolicyNameTemplate, serviceName)
}
