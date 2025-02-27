package metrics_collectors

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/allowexternaltraffic"
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
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strconv"
)

const (
	ReasonEnforcementGloballyDisabled          = "MetricsCollectorEnforcementGloballyDisabled"
	ReasonEnforcementGloballyEnabled           = "MetricsCollectorEnforcementGloballyEnabled"
	ReasonCreatingMetricsCollectorPolicyFailed = "CreatingMetricsCollectorPolicyFailed"
	ReasonGettingMetricsCollectorPolicyFailed  = "GettingMetricsCollectorPolicyFailed"
	ReasonCreatedMetricsCollectorPolicy        = "CreatedMetricsCollectorPolicy"
	ReasonRemovingMetricsCollectorPolicy       = "RemovingMetricsCollectorPolicy"
	ReasonRemovingMetricsCollectorPolicyFailed = "RemovingMetricsCollectorFailed"
	ReasonRemovedMetricsCollectorPolicy        = "RemovedMetricsCollectorPolicy"
	OtterizeMetricsCollectorPolicyNameTemplate = "metrics-collector-access-to-%s-%s"
)

type K8sResourceEnum int

const (
	K8sResourceInvalid K8sResourceEnum = 0
	K8sResourcePod                     = 1
	K8sResourceService                 = 2
)

func (r *K8sResource) GetRuntimeObject() metav1.Object {
	if r.resourceType == K8sResourcePod {
		return r.pod
	}

	return r.service
}

func (r *K8sResource) DeepCopyObject() runtime.Object {
	if r.resourceType == K8sResourcePod {
		return r.pod.DeepCopyObject()
	}

	return r.service.DeepCopyObject()
}

type K8sResource struct {
	resourceType K8sResourceEnum
	pod          *corev1.Pod
	service      *corev1.Service
	metav1.TypeMeta
	metav1.ObjectMeta
}

type NetworkPolicyHandler struct {
	client client.Client
	scheme *runtime.Scheme
	injectablerecorder.InjectableRecorder
	allowMetricsCollector allowexternaltraffic.Enum
}

func NewNetworkPolicyHandler(
	client client.Client,
	scheme *runtime.Scheme,
	allowMetricsCollector allowexternaltraffic.Enum,
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

func (r *NetworkPolicyHandler) HandleEndpoints(ctx context.Context, endpoints *corev1.Endpoints) error {
	// When we handle endpoints - we really wants the handle the service that is behind the endpoints
	svc, err := r.getEndpointsService(ctx, endpoints)
	if k8serrors.IsNotFound(err) {
		return nil
	}

	if err != nil {
		return errors.Wrap(err)
	}

	err = r.HandleService(ctx, svc)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (r *NetworkPolicyHandler) HandleService(ctx context.Context, service *corev1.Service) error {
	// when we handle a service - we want to handle all the related pods. We fetch them using the endpoints
	endpoints := &corev1.Endpoints{}
	err := r.client.Get(ctx, client.ObjectKey{Namespace: service.Namespace, Name: service.Name}, endpoints)
	if err != nil {
		return errors.Wrap(err)
	}

	endpointsPods, err := r.getEndpointsPods(ctx, endpoints)
	if err != nil {
		return errors.Wrap(err)
	}

	ownerResource := K8sResource{K8sResourceService, nil, service, service.TypeMeta, service.ObjectMeta}
	for _, pod := range endpointsPods {
		// We want to handle the pod once from the service perspective (that will determine the Prometheus scrape properties)
		err = r.handlePod(ctx, &pod, &ownerResource)
		// And another time from the pod perspective. It seems like the PodReconciler would be good enough, but since we don't know who
		// will go up first - the pod or its service endpoint - we might not find the pods' network policy that was created based on the
		// service representing the pod
		podOwnerResource := K8sResource{K8sResourcePod, &pod, nil, pod.TypeMeta, pod.ObjectMeta}
		err = r.handlePod(ctx, &pod, &podOwnerResource)
		if err != nil {
			return errors.Wrap(err)
		}
	}

	return nil

}

func (r *NetworkPolicyHandler) HandlePod(ctx context.Context, pod *corev1.Pod) error {
	podOwnerResource := K8sResource{K8sResourcePod, pod, nil, pod.TypeMeta, pod.ObjectMeta}
	err := r.handlePod(ctx, pod, &podOwnerResource)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (r *NetworkPolicyHandler) handlePod(ctx context.Context, pod *corev1.Pod, ownerResource *K8sResource) error {
	// only act on pods affected by Otterize
	serverLabel, ok := pod.Labels[v2alpha1.OtterizeServiceLabelKey]
	if !ok {
		return nil
	}

	serviceId, err := serviceidresolver.NewResolver(r.client).ResolvePodToServiceIdentity(ctx, pod)
	if err != nil {
		return errors.Wrap(err)
	}

	policyName := r.formatPolicyName(serviceId, ownerResource)

	// If configuration is set to "Always", we want to create the network policy regardless of other network policies
	if r.allowMetricsCollector == allowexternaltraffic.Always {
		r.RecordNormalEventf(pod, ReasonEnforcementGloballyEnabled, "Creating network policy for resource '%s' because enforcement is globally enabled", pod.GetName())
		err = r.createNetpolForPod(ctx, pod, policyName, serviceId, serverLabel, ownerResource)
		if err != nil {
			return errors.Wrap(err)
		}
		return nil
	}

	// If configuration is set to "Off", we want to delete the network policy regardless of other network policies
	if r.allowMetricsCollector == allowexternaltraffic.Off {
		r.RecordNormalEventf(pod, ReasonEnforcementGloballyDisabled, "Skipping created network policy for resource '%s' because enforcement is globally disabled", pod.GetName())
		err = r.handlePolicyDelete(ctx, policyName, pod.Namespace)
		if err != nil {
			return errors.Wrap(err)
		}
		return nil
	}

	// From this point we only handle the case where the configuration is set to "IfBlockedByOtterize", which means that
	// we want to create the network policy only if there are other network policies that block the traffic.

	netpolSlice, err := r.getAllPossibleNetworkPolicies(ctx, pod, serviceId)
	if err != nil {
		return errors.Wrap(err)
	}

	_, foundNetpolWithIngressRule := lo.Find(netpolSlice, func(netpol v1.NetworkPolicy) bool {
		return lo.Contains(netpol.Spec.PolicyTypes, v1.PolicyTypeIngress)
	})

	if !foundNetpolWithIngressRule {
		// We didn't find any ingress rules for the service, so there isn't any network policy that currently blocks
		// traffic to this pod
		err = r.handlePolicyDelete(ctx, policyName, pod.Namespace)
		if err != nil {
			return errors.Wrap(err)
		}
		return nil
	}

	err = r.createNetpolForPod(ctx, pod, policyName, serviceId, serverLabel, ownerResource)
	if err != nil {
		return errors.Wrap(err)
	}
	return nil
}

func (r *NetworkPolicyHandler) getEndpointsService(ctx context.Context, endpoints *corev1.Endpoints) (*corev1.Service, error) {
	svc := &corev1.Service{}
	err := r.client.Get(ctx, types.NamespacedName{Name: endpoints.Name, Namespace: endpoints.Namespace}, svc)
	if k8serrors.IsNotFound(err) {
		// we do not wrap the error here on purpose, this should not fail the flow - only stop it
		return &corev1.Service{}, err
	}

	if err != nil {
		return &corev1.Service{}, errors.Wrap(err)
	}

	return svc, nil
}

func (r *NetworkPolicyHandler) getEndpointsPods(ctx context.Context, endpoints *corev1.Endpoints) ([]corev1.Pod, error) {
	addresses := make([]corev1.EndpointAddress, 0)
	for _, subset := range endpoints.Subsets {
		addresses = append(addresses, subset.Addresses...)
		addresses = append(addresses, subset.NotReadyAddresses...)
	}

	pods := make([]corev1.Pod, 0)
	for _, address := range addresses {
		if address.TargetRef == nil || address.TargetRef.Kind != "Pod" {
			// If we could not find the relevant pod, we just skip to the next one
			continue
		}

		pod := &corev1.Pod{}
		err := r.client.Get(ctx, types.NamespacedName{Name: address.TargetRef.Name, Namespace: address.TargetRef.Namespace}, pod)
		if err != nil {
			return make([]corev1.Pod, 0), errors.Wrap(err)
		}

		pods = append(pods, *pod)
	}
	return pods, nil
}

func (r *NetworkPolicyHandler) getAllPossibleNetworkPolicies(ctx context.Context, pod *corev1.Pod, serviceId serviceidentity.ServiceIdentity) ([]v1.NetworkPolicy, error) {
	netpolSlice := make([]v1.NetworkPolicy, 0)
	netpolList := &v1.NetworkPolicyList{}

	// Get policies which were created by intents targeting this pod by its owner with "kind"
	err := r.client.List(ctx, netpolList, client.MatchingLabels{v2alpha1.OtterizeNetworkPolicy: serviceId.GetFormattedOtterizeIdentityWithKind()})
	if err != nil {
		return make([]v1.NetworkPolicy, 0), errors.Wrap(err)
	}
	netpolSlice = append(netpolSlice, netpolList.Items...)

	// Get policies which were created by intents targeting this pod by its owner without "kind"
	err = r.client.List(ctx, netpolList, client.MatchingLabels{v2alpha1.OtterizeNetworkPolicy: serviceId.GetFormattedOtterizeIdentityWithoutKind()})
	if err != nil {
		return make([]v1.NetworkPolicy, 0), errors.Wrap(err)
	}
	netpolSlice = append(netpolSlice, netpolList.Items...)

	// Get policies which were created by intents targeting this pod by its service
	endpointsList := &corev1.EndpointsList{}
	err = r.client.List(
		ctx,
		endpointsList,
		&client.MatchingFields{v2alpha1.EndpointsPodNamesIndexField: pod.Name},
		&client.ListOptions{Namespace: pod.Namespace},
	)
	if err != nil {
		return make([]v1.NetworkPolicy, 0), errors.Wrap(err)
	}

	for _, endpoint := range endpointsList.Items {
		err = r.client.List(ctx, netpolList, client.MatchingLabels{v2alpha1.OtterizeNetworkPolicy: (&serviceidentity.ServiceIdentity{Name: endpoint.Name, Namespace: pod.Namespace, Kind: serviceidentity.KindService}).GetFormattedOtterizeIdentityWithKind()})
		if err != nil {
			return make([]v1.NetworkPolicy, 0), errors.Wrap(err)
		}
		netpolSlice = append(netpolSlice, netpolList.Items...)
	}

	return netpolSlice, nil
}

func (r *NetworkPolicyHandler) createNetpolForPod(ctx context.Context, pod *corev1.Pod, policyName string, identity serviceidentity.ServiceIdentity, serverLabel string, ownerResource *K8sResource) error {

	if !r.shouldScrape(ownerResource) {
		r.RecordNormalEventf(pod, ReasonRemovingMetricsCollectorPolicy, "Removing policy for resource '%s' - it is not marked for metrics collection", pod.GetName())
		err := r.handlePolicyDelete(ctx, policyName, pod.Namespace)
		if err != nil {
			return errors.Wrap(err)
		}
		return nil
	}

	annotations := map[string]string{
		v2alpha1.OtterizeCreatedForServiceAnnotation: pod.GetName(),
	}

	serviceIdentityLabels, ok, err := v2alpha1.ServiceIdentityToLabelsForWorkloadSelection(ctx, r.client, identity)
	if err != nil {
		return errors.Wrap(err)
	}
	if !ok {
		return errors.Wrap(err)
	}

	rule := v1.NetworkPolicyIngressRule{}

	newPolicy := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: pod.Namespace,
			Labels: map[string]string{
				v2alpha1.OtterizeNetworkPolicyMetricsCollectors: serverLabel,
			},
			Annotations: annotations,
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: serviceIdentityLabels,
			},
			Ingress: []v1.NetworkPolicyIngressRule{
				rule,
			},
		},
	}

	scrapePorts, err := r.getMetricsPorts(ownerResource)
	if err != nil {
		return errors.Wrap(err)
	}

	if len(scrapePorts) == 0 {
		// In case there aren't any ports defined - we don't want to create a network policy.
		// Prometheus default behavior would be to try and scrape all ports defined, but we don't want to allow that.
		return nil
	}

	for _, portNumber := range scrapePorts {
		newPolicy.Spec.Ingress[0].Ports = append(newPolicy.Spec.Ingress[0].Ports, v1.NetworkPolicyPort{
			Port: lo.ToPtr(intstr.IntOrString{IntVal: portNumber, Type: intstr.Int}),
		})
	}

	err = controllerutil.SetOwnerReference(ownerResource.GetRuntimeObject(), newPolicy, r.scheme)
	if err != nil {
		return errors.Wrap(err)
	}

	existingPolicy := &v1.NetworkPolicy{}
	errGetExistingPolicy := r.client.Get(ctx, types.NamespacedName{Name: policyName, Namespace: pod.GetNamespace()}, existingPolicy)

	// No matching network policy found, create one
	if k8serrors.IsNotFound(errGetExistingPolicy) {
		logrus.Debugf("Creating network policy to allow external traffic to %s (ns %s)", pod.GetName(), pod.GetNamespace())
		err = r.client.Create(ctx, newPolicy)
		if err != nil {
			return r.handleCreationErrors(ctx, err, newPolicy, pod)
		}
		r.RecordNormalEventf(pod, ReasonCreatedMetricsCollectorPolicy, "Created metrics collector network policy for resource '%s'", pod.GetName())
		return nil
	}

	if errGetExistingPolicy != nil {
		r.RecordWarningEventf(pod, ReasonGettingMetricsCollectorPolicyFailed, "failed to get exiting metrics collector network policy: %s", errGetExistingPolicy.Error())
		return errGetExistingPolicy
	}

	// Found existing matching policy, if it is identical to this one - do nothing
	if r.arePoliciesEqual(existingPolicy, newPolicy) {
		return nil
	}

	return r.updatePolicy(ctx, existingPolicy, newPolicy, pod)
}

func (r *NetworkPolicyHandler) handleCreationErrors(ctx context.Context, err error, policy *v1.NetworkPolicy, ownerPod *corev1.Pod) error {
	if !k8serrors.IsAlreadyExists(err) {
		r.RecordWarningEventf(ownerPod, ReasonCreatingMetricsCollectorPolicyFailed, "failed to create metrics collector network policy: %s", err.Error())
		return errors.Wrap(err)
	}

	existingPolicy := &v1.NetworkPolicy{}
	err = r.client.Get(ctx, types.NamespacedName{Name: policy.Name, Namespace: policy.Namespace}, existingPolicy)
	if err != nil {
		return errors.Wrap(err) // Don't retry anymore
	}
	return r.updatePolicy(ctx, existingPolicy, policy, ownerPod)
}

func (r *NetworkPolicyHandler) updatePolicy(ctx context.Context, existingPolicy *v1.NetworkPolicy, newPolicy *v1.NetworkPolicy, ownerPod *corev1.Pod) error {
	policyCopy := existingPolicy.DeepCopy()
	policyCopy.Labels = newPolicy.Labels
	policyCopy.Annotations = newPolicy.Annotations
	policyCopy.Spec = newPolicy.Spec

	err := controllerutil.SetOwnerReference(ownerPod, policyCopy, r.scheme)
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

func (r *NetworkPolicyHandler) formatPolicyName(serviceId serviceidentity.ServiceIdentity, ownerResource *K8sResource) string {
	return fmt.Sprintf(OtterizeMetricsCollectorPolicyNameTemplate, strings.ToLower(ownerResource.Kind), strings.ToLower(serviceId.Name))
}

func (r *NetworkPolicyHandler) shouldScrape(resource *K8sResource) bool {
	shouldScrape := resource.Annotations["prometheus.io/scrape"]
	return shouldScrape == "true"
}

func (r *NetworkPolicyHandler) getMetricsPorts(resource *K8sResource) ([]int32, error) {
	ports := make([]int32, 0)

	if !r.shouldScrape(resource) {
		return ports, nil
	}

	port, err := strconv.Atoi(resource.Annotations["prometheus.io/port"])
	if err != nil {
		logrus.Errorf("failed to convert port to int: %s", err)
		return nil, errors.Wrap(err)
	}
	ports = append(ports, int32(port))

	return ports, nil
}
