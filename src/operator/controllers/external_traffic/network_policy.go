package external_traffic

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/allowexternaltraffic"
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
	allowExternalTraffic allowexternaltraffic.Enum
}

func NewNetworkPolicyHandler(
	client client.Client,
	scheme *runtime.Scheme,
	allowExternalTraffic allowexternaltraffic.Enum,
) *NetworkPolicyHandler {
	return &NetworkPolicyHandler{client: client, scheme: scheme, allowExternalTraffic: allowExternalTraffic}
}

func (r *NetworkPolicyHandler) createOrUpdateNetworkPolicy(
	ctx context.Context, endpoints *corev1.Endpoints, owner *corev1.Service, otterizeServiceName string, selector metav1.LabelSelector, ingressList *v1.IngressList, successMsg string) error {
	policyName := r.formatPolicyName(endpoints.Name)
	newPolicy := buildNetworkPolicyObjectForEndpoints(endpoints, otterizeServiceName, selector, ingressList, policyName)
	err := controllerutil.SetOwnerReference(owner, newPolicy, r.scheme)
	if err != nil {
		return errors.Wrap(err)
	}
	existingPolicy := &v1.NetworkPolicy{}
	errGetExistingPolicy := r.client.Get(ctx, types.NamespacedName{Name: policyName, Namespace: endpoints.GetNamespace()}, existingPolicy)

	// No matching network policy found, create one
	if k8serrors.IsNotFound(errGetExistingPolicy) {
		logrus.Infof("Creating network policy to allow external traffic to %s (ns %s)", endpoints.GetName(), endpoints.GetNamespace())
		err := r.client.Create(ctx, newPolicy)
		if err != nil {
			r.RecordWarningEventf(owner, ReasonCreatingExternalTrafficPolicyFailed, "failed to create external traffic network policy: %s", err.Error())
			return errors.Wrap(err)
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
		return errors.Wrap(err)
	}

	err = r.client.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy))
	if err != nil {
		return errors.Wrap(err)
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
		v1alpha3.OtterizeCreatedForServiceAnnotation: endpoints.GetName(),
	}

	if len(ingressList.Items) != 0 {
		annotations[v1alpha3.OtterizeCreatedForIngressAnnotation] = strings.Join(lo.Map(ingressList.Items, func(ingress v1.Ingress, _ int) string {
			return ingress.Name
		}), ",")
	}

	netpol := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: endpoints.Namespace,
			Labels: map[string]string{
				v1alpha3.OtterizeNetworkPolicyExternalTraffic: otterizeServiceName,
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

// HandleBeforeAccessPolicyRemoval - call this function when an access policy is being deleted, and you want to make sure
//
//	that related external policies will be removed as well (if needed)
func (r *NetworkPolicyHandler) HandleBeforeAccessPolicyRemoval(ctx context.Context, accessPolicy *v1.NetworkPolicy) error {
	// if allowExternalTraffic is Always - external policies are not dependent on access policies
	if r.allowExternalTraffic == allowexternaltraffic.Always {
		return nil
	}

	nonExternalPolicyList := &v1.NetworkPolicyList{}
	serviceNameLabel := accessPolicy.Labels[v1alpha3.OtterizeNetworkPolicy]

	// list policies the are not external policies (access + default deny)
	err := r.client.List(ctx, nonExternalPolicyList, client.MatchingLabels{v1alpha3.OtterizeNetworkPolicy: serviceNameLabel},
		&client.ListOptions{Namespace: accessPolicy.Namespace})
	if err != nil {
		return errors.Wrap(err)
	}
	// If more than one related policies still exist don't remove the external policy
	if len(nonExternalPolicyList.Items) > 1 {
		return nil
	}

	externalPolicyList := &v1.NetworkPolicyList{}
	err = r.client.List(ctx, externalPolicyList, client.MatchingLabels{v1alpha3.OtterizeNetworkPolicyExternalTraffic: serviceNameLabel},
		&client.ListOptions{Namespace: accessPolicy.Namespace})
	if err != nil {
		return errors.Wrap(err)
	}

	for _, externalPolicy := range externalPolicyList.Items {
		err := r.client.Delete(ctx, externalPolicy.DeepCopy())
		if err != nil {
			return errors.Wrap(err)
		}
	}
	return nil
}

func (r *NetworkPolicyHandler) HandleEndpointsByName(ctx context.Context, serviceName string, namespace string) error {
	endpoints := &corev1.Endpoints{}
	err := r.client.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: namespace}, endpoints)
	if k8serrors.IsNotFound(err) {
		// delete is handled by garbage collection - the service owns the network policy
		return nil
	}

	if err != nil {
		return errors.Wrap(err)
	}

	return r.HandleEndpoints(ctx, endpoints)
}

func (r *NetworkPolicyHandler) HandlePodsByLabelSelector(ctx context.Context, namespace string, labelSelector labels.Selector) error {
	podList := &corev1.PodList{}
	err := r.client.List(ctx, podList,
		&client.ListOptions{Namespace: namespace},
		client.MatchingLabelsSelector{Selector: labelSelector})
	if err != nil {
		return errors.Wrap(err)
	}
	return r.handlePodList(ctx, podList)
}

func (r *NetworkPolicyHandler) HandlePodsByNamespace(ctx context.Context, namespace string) error {
	selector, err := r.otterizeServerLabeledPodsSelector()
	if err != nil {
		return errors.Wrap(err)
	}
	podList := &corev1.PodList{}
	err = r.client.List(ctx, podList,
		&client.ListOptions{Namespace: namespace},
		client.MatchingLabelsSelector{Selector: selector})
	if err != nil {
		return errors.Wrap(err)
	}
	return r.handlePodList(ctx, podList)
}

func (r *NetworkPolicyHandler) HandleAllPods(ctx context.Context) error {
	selector, err := r.otterizeServerLabeledPodsSelector()
	if err != nil {
		return errors.Wrap(err)
	}
	podList := &corev1.PodList{}
	err = r.client.List(ctx, podList,
		client.MatchingLabelsSelector{Selector: selector})
	if err != nil {
		return errors.Wrap(err)
	}
	return r.handlePodList(ctx, podList)
}

func (r *NetworkPolicyHandler) handlePodList(ctx context.Context, podList *corev1.PodList) error {
	for _, pod := range podList.Items {
		err := r.handlePod(ctx, &pod)
		if err != nil {
			return errors.Wrap(err)
		}
	}
	return nil
}

func (r *NetworkPolicyHandler) handlePod(ctx context.Context, pod *corev1.Pod) error {
	var endpointsList corev1.EndpointsList

	err := r.client.List(
		ctx,
		&endpointsList,
		&client.MatchingFields{v1alpha3.EndpointsPodNamesIndexField: pod.Name},
		&client.ListOptions{Namespace: pod.Namespace},
	)

	if err != nil {
		return errors.Wrap(err)
	}
	for _, endpoints := range endpointsList.Items {
		if err := r.HandleEndpoints(ctx, &endpoints); err != nil {
			return errors.Wrap(err)
		}
	}

	return nil

}

// HandleEndpoints
// Every HandleX function goes through this function, and it handles this cases:
// (1) Endpoints reconciler watch endpoints and call HandleEndpoints, which means it gets updates when Services are updated, or the pods backing them are updated.
// (2) It receives handle requests from the IngressReconciler, when Ingresses are created, updated or deleted.
// (3) It receives handle requests from the Intents NetworkPolicyReconciler, when Network Policies that apply intents
//
//	are created, updated or deleted. This means that if you create, update or delete intents, the corresponding
//	external traffic policy will be created (if there were no other intents affecting the service before then) or
//	deleted (if no intents network policies refer to the pods backing the service any longer).
//
//	 When HandleEndpoints is called, and the Service is of type LoadBalancer, NodePort, or is referenced by an Ingress,
//		   it checks if the backing pods are affected by Otterize Intents Network Policies.
//		   If so, and the reconciler is enabled, it will create network policies to allow external traffic to those pods.
//		   If the Endpoints (= Services) update port, it will update the port specified in the corresponding network policy.
//		   If the Endpoints no longer refer to pods affected by Intents, then the network policy will be deleted.
//		   If the Service is deleted completely, then the corresponding network policy will be deleted, since it is owned
//		   by the service.
func (r *NetworkPolicyHandler) HandleEndpoints(ctx context.Context, endpoints *corev1.Endpoints) error {
	svc := &corev1.Service{}
	err := r.client.Get(ctx, types.NamespacedName{Name: endpoints.GetName(), Namespace: endpoints.GetNamespace()}, svc)
	if k8serrors.IsNotFound(err) {
		// delete is handled by garbage collection - the service owns the network policy
		return nil
	}

	if err != nil {
		return errors.Wrap(err)
	}

	ingressList, err := getIngressRefersToService(ctx, r.client, svc)
	if err != nil {
		return errors.Wrap(err)
	}

	if !isServiceExternallyAccessible(svc, ingressList) {
		return r.handlePolicyDelete(ctx, r.formatPolicyName(svc.Name), svc.Namespace)
	}

	return r.handleEndpointsWithIngressList(ctx, endpoints, ingressList)
}

func (r *NetworkPolicyHandler) handleEndpointsWithIngressList(ctx context.Context, endpoints *corev1.Endpoints, ingressList *v1.IngressList) error {
	addresses := r.getAddressesFromEndpoints(endpoints)
	foundOtterizeNetpolsAffectingPods := false
	for _, address := range addresses {
		pod, err := r.getAffectedPod(ctx, address)
		if k8serrors.IsNotFound(err) {
			continue
		}

		if err != nil {
			return errors.Wrap(err)
		}

		// only act on pods affected by Otterize
		serverLabel, ok := pod.Labels[v1alpha3.OtterizeServiceLabelKey]
		if !ok {
			continue
		}

		netpolList := &v1.NetworkPolicyList{}

		err = r.client.List(ctx, netpolList, client.MatchingLabels{v1alpha3.OtterizeNetworkPolicy: serverLabel})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				// only act on pods affected by Otterize policies - if they were not created yet,
				// the intents reconciler will call the endpoints reconciler once it does.
				continue
			}
			return errors.Wrap(err)
		}

		hasIngressRules := lo.SomeBy(netpolList.Items, func(netpol v1.NetworkPolicy) bool {
			return lo.Contains(netpol.Spec.PolicyTypes, v1.PolicyTypeIngress)
		})

		if !hasIngressRules {
			if r.allowExternalTraffic == allowexternaltraffic.Always {
				err := r.handleNetpolsForOtterizeServiceWithoutIntents(ctx, endpoints, serverLabel, ingressList)
				if err != nil {
					return errors.Wrap(err)
				}
			}
			continue
		}

		netpolSlice := make([]v1.NetworkPolicy, 0)
		netpolSlice = append(netpolSlice, netpolList.Items...)

		foundOtterizeNetpolsAffectingPods = true
		err = r.handleNetpolsForOtterizeService(ctx, endpoints, serverLabel, ingressList, netpolSlice)
		if err != nil {
			return errors.Wrap(err)
		}

	}

	if !foundOtterizeNetpolsAffectingPods && r.allowExternalTraffic != allowexternaltraffic.Always {
		policyName := r.formatPolicyName(endpoints.Name)
		err := r.handlePolicyDelete(ctx, policyName, endpoints.Namespace)
		if err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (r *NetworkPolicyHandler) getAffectedPod(ctx context.Context, address corev1.EndpointAddress) (*corev1.Pod, error) {
	if address.TargetRef == nil || address.TargetRef.Kind != "Pod" {
		return nil, k8serrors.NewNotFound(corev1.Resource("Pod"), "not-a-pod")
	}

	pod := &corev1.Pod{}
	err := r.client.Get(ctx, types.NamespacedName{Name: address.TargetRef.Name, Namespace: address.TargetRef.Namespace}, pod)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	return pod, nil
}

func (r *NetworkPolicyHandler) getAddressesFromEndpoints(endpoints *corev1.Endpoints) []corev1.EndpointAddress {
	addresses := make([]corev1.EndpointAddress, 0)
	for _, subset := range endpoints.Subsets {
		addresses = append(addresses, subset.Addresses...)
		addresses = append(addresses, subset.NotReadyAddresses...)

	}
	return addresses
}

func (r *NetworkPolicyHandler) handlePolicyDelete(ctx context.Context, policyName string, policyNamespace string) error {
	policy := &v1.NetworkPolicy{}
	err := r.client.Get(ctx, types.NamespacedName{Name: policyName, Namespace: policyNamespace}, policy)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// nothing to do
			return nil
		}

		return errors.Wrap(err)
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
		return errors.Wrap(err)
	}
	r.RecordNormalEvent(owner, ReasonRemovedExternalTrafficPolicy, "removed external traffic network policy, success")

	return nil
}

func (r *NetworkPolicyHandler) handleNetpolsForOtterizeService(ctx context.Context, endpoints *corev1.Endpoints, otterizeServiceName string, ingressList *v1.IngressList, netpolList []v1.NetworkPolicy) error {
	svc := &corev1.Service{}
	err := r.client.Get(ctx, types.NamespacedName{Name: endpoints.Name, Namespace: endpoints.Namespace}, svc)
	if err != nil {
		return errors.Wrap(err)
	}

	// delete policy if disabled
	if r.allowExternalTraffic == allowexternaltraffic.Off {
		r.RecordNormalEventf(svc, ReasonEnforcementGloballyDisabled, "Skipping created external traffic network policy for service '%s' because enforcement is globally disabled", endpoints.GetName())
		err = r.handlePolicyDelete(ctx, r.formatPolicyName(endpoints.Name), endpoints.Namespace)
		if err != nil {
			return errors.Wrap(err)
		}
		return nil
	}

	for _, netpol := range netpolList {
		successMsg := fmt.Sprintf(successMsgNetpolCreate, endpoints.GetName(), netpol.GetName())
		err = r.createOrUpdateNetworkPolicy(ctx, endpoints, svc, otterizeServiceName, netpol.Spec.PodSelector, ingressList, successMsg)

		if err != nil {
			return errors.Wrap(err)
		}
	}
	return nil
}

func (r *NetworkPolicyHandler) handleNetpolsForOtterizeServiceWithoutIntents(ctx context.Context, endpoints *corev1.Endpoints, otterizeServiceName string, ingressList *v1.IngressList) error {
	svc := &corev1.Service{}
	err := r.client.Get(ctx, types.NamespacedName{Name: endpoints.Name, Namespace: endpoints.Namespace}, svc)
	if err != nil {
		return errors.Wrap(err)
	}

	// delete policy if disabled
	if r.allowExternalTraffic == allowexternaltraffic.Off {
		r.RecordNormalEventf(svc, ReasonEnforcementGloballyDisabled, "Skipping created external traffic network policy for service '%s' because enforcement is globally disabled", endpoints.GetName())
		err = r.handlePolicyDelete(ctx, r.formatPolicyName(endpoints.Name), endpoints.Namespace)
		if err != nil {
			return errors.Wrap(err)
		}
		return nil
	}

	err = r.createOrUpdateNetworkPolicy(ctx, endpoints, svc, otterizeServiceName, metav1.LabelSelector{MatchLabels: svc.Spec.Selector}, ingressList, fmt.Sprintf("created external traffic network policy for service '%s'", endpoints.GetName()))
	if err != nil {
		return errors.Wrap(err)
	}
	return nil
}

func (r *NetworkPolicyHandler) formatPolicyName(serviceName string) string {
	return fmt.Sprintf(OtterizeExternalNetworkPolicyNameTemplate, serviceName)
}

func (r *NetworkPolicyHandler) otterizeServerLabeledPodsSelector() (labels.Selector, error) {
	return metav1.LabelSelectorAsSelector(
		&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      v1alpha3.OtterizeServiceLabelKey,
				Operator: metav1.LabelSelectorOpExists,
			},
		},
		})
}
