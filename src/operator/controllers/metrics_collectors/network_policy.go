package metrics_collectors

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strconv"
	"strings"
)

const (
	ReasonEnforcementGloballyDisabled          = "EnforcementGloballyDisabled"
	ReasonCreatingMetricsCollectorPolicyFailed = "CreatingMetricsCollectorPolicyFailed"
	ReasonGettingMetricsCollectorPolicyFailed  = "GettingMetricsCollectorPolicyFailed"
	ReasonCreatedMetricsCollectorPolicy        = "CreatedMetricsCollectorPolicy"
	ReasonRemovingMetricsCollectorPolicy       = "RemovingMetricsCollectorPolicy"
	ReasonRemovingMetricsCollectorPolicyFailed = "RemovingMetricsCollectorFailed"
	ReasonRemovedMetricsCollectorPolicy        = "RemovedMetricsCollectorPolicy"
	OtterizeMetricsCollectorPolicyNameTemplate = "metrics-collector-access-to-%s"
	successMsgNetpolCreate                     = "created metrics collector network policy. service '%s' refers to pods protected by network policy '%s'"
)

type NetworkPolicyHandler struct {
	client client.Client
	scheme *runtime.Scheme
	injectablerecorder.InjectableRecorder
	allowMetricsCollector bool
}

func NewNetworkPolicyHandler(
	client client.Client,
	scheme *runtime.Scheme,
	allowMetricsCollector bool,
) *NetworkPolicyHandler {
	return &NetworkPolicyHandler{
		client:                client,
		scheme:                scheme,
		allowMetricsCollector: allowMetricsCollector,
	}
}

func (r *NetworkPolicyHandler) InjectRecorder(recorder record.EventRecorder) {
	r.Recorder = recorder
}

func (r *NetworkPolicyHandler) handleCreationErrors(ctx context.Context, err error, policy *v1.NetworkPolicy, owner *corev1.Service) error {
	if !k8serrors.IsAlreadyExists(err) {
		r.RecordWarningEventf(owner, ReasonCreatingMetricsCollectorPolicyFailed, "failed to create metrics collector network policy: %s", err.Error())
		return errors.Wrap(err)
	}
	existingPolicy := &v1.NetworkPolicy{}
	err = r.client.Get(ctx, types.NamespacedName{Name: policy.Name, Namespace: policy.Namespace}, existingPolicy)
	if err != nil {
		return errors.Wrap(err) // Don't retry anymore
	}
	return r.updatePolicy(ctx, existingPolicy, policy, owner)
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
	svc := corev1.Service{}
	err := r.client.Get(ctx, types.NamespacedName{Name: endpoints.GetName(), Namespace: endpoints.GetNamespace()}, &svc)
	if k8serrors.IsNotFound(err) {
		// delete is handled by garbage collection - the service owns the network policy
		return nil
	}

	if err != nil {
		return errors.Wrap(err)
	}

	//return nil
	ingressList, err := r.getIngressRefersToService(ctx, r.client, svc)
	if err != nil {
		return errors.Wrap(err)
	}

	return r.handleEndpointsWithIngressList(ctx, endpoints, ingressList)
}

func (r *NetworkPolicyHandler) handleEndpointsWithIngressList(ctx context.Context, endpoints *corev1.Endpoints, ingressList *v1.IngressList) error {
	addresses := r.getAddressesFromEndpoints(endpoints)
	foundOtterizeNetpolsAffectingPods := false
	for _, address := range addresses {
		pod, err := r.getAffectedPod(ctx, address)
		if k8sErr := &(k8serrors.StatusError{}); errors.As(err, &k8sErr) {
			if k8serrors.IsNotFound(k8sErr) {
				continue
			}
		}

		if err != nil {
			return errors.Wrap(err)
		}

		// only act on pods affected by Otterize
		serverLabel, ok := pod.Labels[v2alpha1.OtterizeServiceLabelKey]
		if !ok {
			continue
		}
		netpolSlice := make([]v1.NetworkPolicy, 0)

		serviceId, err := serviceidresolver.NewResolver(r.client).ResolvePodToServiceIdentity(ctx, pod)
		if err != nil {
			return errors.Wrap(err)
		}
		netpolList := &v1.NetworkPolicyList{}
		// Get netpolSlice which was created by intents targeting this pod by its owner with "kind"
		err = r.client.List(ctx, netpolList, client.MatchingLabels{v2alpha1.OtterizeNetworkPolicy: serviceId.GetFormattedOtterizeIdentityWithKind()})
		if err != nil {
			return errors.Wrap(err)
		}
		netpolSlice = append(netpolSlice, netpolList.Items...)
		// Get netpolSlice which was created by intents targeting this pod by its owner without "kind"
		err = r.client.List(ctx, netpolList, client.MatchingLabels{v2alpha1.OtterizeNetworkPolicy: serviceId.GetFormattedOtterizeIdentityWithoutKind()})
		if err != nil {
			return errors.Wrap(err)
		}
		netpolSlice = append(netpolSlice, netpolList.Items...)

		// Get netpolSlice which was created by intents targeting this pod by its service
		err = r.client.List(ctx, netpolList, client.MatchingLabels{v2alpha1.OtterizeNetworkPolicy: (&serviceidentity.ServiceIdentity{Name: endpoints.Name, Namespace: endpoints.Namespace, Kind: serviceidentity.KindService}).GetFormattedOtterizeIdentityWithKind()})
		if err != nil {
			return errors.Wrap(err)
		}

		netpolSlice = append(netpolSlice, netpolList.Items...)

		hasIngressRules := lo.SomeBy(netpolSlice, func(netpol v1.NetworkPolicy) bool {
			return lo.Contains(netpol.Spec.PolicyTypes, v1.PolicyTypeIngress)
		})

		if !hasIngressRules {
			// no ingress rules for the service, so we don't need to create a network policy

			// Do we need to handle something like this? I think not
			//if r.allowExternalTraffic == allowexternaltraffic.Always {
			//	err := r.handleNetpolsForOtterizeServiceWithoutIntents(ctx, endpoints, serverLabel, ingressList)
			//	if err != nil {
			//		return errors.Wrap(err)
			//	}
			//} else {
			policyName := r.formatPolicyName(endpoints.Name)
			err := r.handlePolicyDelete(ctx, policyName, endpoints.Namespace)
			if err != nil {
				return errors.Wrap(err)
			}
			//}
			continue
		}

		foundOtterizeNetpolsAffectingPods = true
		err = r.handleNetpolsForOtterizeService(ctx, endpoints, serverLabel, ingressList, netpolSlice)
		if err != nil {
			return errors.Wrap(err)
		}

	}

	if !foundOtterizeNetpolsAffectingPods {
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
			logrus.Debugf("can't get the owner of %s. So no events will be recorded for the deletion", policyName)
		}
		owner = ownerObj
	}

	r.RecordNormalEvent(owner, ReasonRemovingMetricsCollectorPolicy, "removing network policy, reconciler was disabled")

	err = r.client.Delete(ctx, policy)
	if err != nil && !k8serrors.IsNotFound(err) {
		r.RecordWarningEventf(owner, ReasonRemovingMetricsCollectorPolicyFailed, "failed removing network policy: %s", err.Error())
		return errors.Wrap(err)
	}
	r.RecordNormalEvent(owner, ReasonRemovedMetricsCollectorPolicy, "removed network policy, success")

	return nil
}

func (r *NetworkPolicyHandler) handleNetpolsForOtterizeService(ctx context.Context, endpoints *corev1.Endpoints, otterizeServiceName string, ingressList *v1.IngressList, netpolList []v1.NetworkPolicy) error {
	svc := &corev1.Service{}
	err := r.client.Get(ctx, types.NamespacedName{Name: endpoints.Name, Namespace: endpoints.Namespace}, svc)
	if err != nil {
		return errors.Wrap(err)
	}

	// delete policy if disabled
	if !r.allowMetricsCollector {
		r.RecordNormalEventf(svc, ReasonEnforcementGloballyDisabled, "Skipping created external traffic network policy for service '%s' because enforcement is globally disabled", endpoints.GetName())
		err = r.handlePolicyDelete(ctx, r.formatPolicyName(endpoints.Name), endpoints.Namespace)
		if err != nil {
			return errors.Wrap(err)
		}
		return nil
	}

	// ==========================================
	// Prometheus POC
	for _, netpol := range netpolList {
		successMsg := fmt.Sprintf(successMsgNetpolCreate, endpoints.GetName(), netpol.GetName())
		err = r.createOrUpdatePrometheusPolicy(ctx, endpoints, svc, otterizeServiceName, netpol.Spec.PodSelector, ingressList, successMsg)
		if err != nil {
			return errors.Wrap(err)
		}
	}
	// ==========================================

	return nil
}

func (r *NetworkPolicyHandler) createOrUpdatePrometheusPolicy(
	ctx context.Context, endpoints *corev1.Endpoints, owner *corev1.Service, otterizeServiceName string, selector metav1.LabelSelector, ingressList *v1.IngressList, successMsg string) error {
	policyName := r.formatPolicyName(endpoints.Name)
	newPolicy, err := r.buildPrometheusPolicyObjectForEndpoints(ctx, endpoints, owner, otterizeServiceName, selector, ingressList, policyName)
	err = controllerutil.SetOwnerReference(owner, newPolicy, r.scheme)
	if err != nil {
		return errors.Wrap(err)
	}
	existingPolicy := &v1.NetworkPolicy{}
	errGetExistingPolicy := r.client.Get(ctx, types.NamespacedName{Name: policyName, Namespace: endpoints.GetNamespace()}, existingPolicy)

	// No matching network policy found, create one
	if k8serrors.IsNotFound(errGetExistingPolicy) {
		logrus.Debugf("Creating network policy to allow external traffic to %s (ns %s)", endpoints.GetName(), endpoints.GetNamespace())
		err := r.client.Create(ctx, newPolicy)
		if err != nil {
			return r.handleCreationErrors(ctx, err, newPolicy, owner)
		}
		r.RecordNormalEvent(owner, ReasonCreatedMetricsCollectorPolicy, successMsg)
		return nil
	} else if errGetExistingPolicy != nil {
		r.RecordWarningEventf(owner, ReasonGettingMetricsCollectorPolicyFailed, "failed to get external traffic network policy: %s", err.Error())
		return errGetExistingPolicy
	}

	// Found matching policy, is an update needed?
	if r.arePoliciesEqual(existingPolicy, newPolicy) {
		return nil
	}

	return r.updatePolicy(ctx, existingPolicy, newPolicy, owner)
}

func (r *NetworkPolicyHandler) formatPolicyName(serviceName string) string {
	return fmt.Sprintf(OtterizeMetricsCollectorPolicyNameTemplate, serviceName)
}

func (r *NetworkPolicyHandler) buildPrometheusPolicyObjectForEndpoints(
	ctx context.Context, endpoints *corev1.Endpoints, svc *corev1.Service, otterizeServiceName string, selector metav1.LabelSelector, ingressList *v1.IngressList, policyName string) (*v1.NetworkPolicy, error) {

	shouldScrape := svc.Annotations["prometheus.io/scrape"]
	scrapePort := svc.Annotations["prometheus.io/port"]
	if shouldScrape != "true" {
		return nil, nil
	}

	//
	//podList := &corev1.PodList{}
	//err := r.client.List(ctx, podList,
	//	&client.ListOptions{Namespace: svc.Namespace},
	//	client.MatchingLabelsSelector{Selector: labels.SelectorFromSet(svc.Spec.Selector)})
	//
	//if err != nil {
	//	return nil, errors.Wrap(err)
	//}
	//
	//for _, pod := range podList.Items {
	//
	//	fmt.Print("shouldScrape: ", shouldScrape, " scrapePort: ", scrapePort)
	//}

	annotations := map[string]string{
		v2alpha1.OtterizeCreatedForServiceAnnotation: endpoints.GetName(),
	}

	if len(ingressList.Items) != 0 {
		annotations[v2alpha1.OtterizeCreatedForIngressAnnotation] = strings.Join(lo.Map(ingressList.Items, func(ingress v1.Ingress, _ int) string {
			return ingress.Name
		}), ",")
	}

	rule := v1.NetworkPolicyIngressRule{}

	netpol := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: endpoints.Namespace,
			Labels: map[string]string{
				v2alpha1.OtterizeNetworkPolicyMetricsCollector: otterizeServiceName,
			},
			Annotations: annotations,
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: selector,
			Ingress: []v1.NetworkPolicyIngressRule{
				rule,
			},
		},
	}

	portNumber, err := strconv.Atoi(scrapePort)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	netpol.Spec.Ingress[0].Ports = append(netpol.Spec.Ingress[0].Ports, v1.NetworkPolicyPort{
		Port: lo.ToPtr(intstr.IntOrString{IntVal: int32(portNumber), Type: intstr.Int}),
	})

	return netpol, nil
}

func (r *NetworkPolicyHandler) getIngressRefersToService(ctx context.Context, k8sClient client.Client, svc corev1.Service) (*v1.IngressList, error) {
	var ingressList v1.IngressList
	err := k8sClient.List(
		ctx,
		&ingressList,
		&client.MatchingFields{v2alpha1.IngressServiceNamesIndexField: svc.Name},
		&client.ListOptions{Namespace: svc.Namespace})
	if err != nil {
		return nil, errors.Wrap(err)
	}

	return &ingressList, nil
}
