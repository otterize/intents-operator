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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"strings"
)

type K8sResourceEnum string

const (
	K8sResourceInvalid K8sResourceEnum = ""
	K8sResourcePod     K8sResourceEnum = "pod"
	K8sResourceService K8sResourceEnum = "service"
)

const (
	OtterizeMetricsCollectorPolicyNameTemplate = "metrics-collector-access-to-%s-%s"
)

type NetworkPolicyByName map[string]*v1.NetworkPolicy

type PotentiallyScrapeMetricPod struct {
	pod                *corev1.Pod
	scrapeResource     K8sResourceEnum
	scrapeResourceMeta *metav1.ObjectMeta
	scrapeResourceType *metav1.TypeMeta
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

func (r *NetworkPolicyHandler) HandleAllPodsInNamespace(ctx context.Context, namespace string) error {
	// Fetch all the pods we handle in the given namespace
	podsList, err := r.getAllOtterizeHandledPodsInNamespace(ctx, namespace)
	if err != nil {
		return errors.Wrap(err)
	}

	podsWithScrapeResource := lo.Map(podsList.Items, func(item corev1.Pod, _ int) PotentiallyScrapeMetricPod {
		return PotentiallyScrapeMetricPod{pod: &item,
			scrapeResourceMeta: &item.ObjectMeta,
			scrapeResourceType: &item.TypeMeta,
			scrapeResource:     K8sResourcePod}
	})

	reducedPolicies, err := r.reducedNetworkPoliciesInNamespace(ctx, podsWithScrapeResource)
	if err != nil {
		return errors.Wrap(err)
	}

	currentNetworkPolicies, err := r.getCurrentNetworkPoliciesInNamespace(ctx, namespace, K8sResourcePod)
	if err != nil {
		return errors.Wrap(err)
	}

	reducedPoliciesNames := lo.Keys(reducedPolicies)
	currentNetworkPoliciesName := lo.Keys(currentNetworkPolicies)
	policiesToAdd, policiesToDelete := lo.Difference(reducedPoliciesNames, currentNetworkPoliciesName)

	err = r.handlePoliciesToAdd(ctx, policiesToAdd, reducedPolicies)
	if err != nil {
		return errors.Wrap(err)
	}

	err = r.handlePoliciesToDelete(ctx, policiesToDelete, currentNetworkPolicies)
	if err != nil {
		return errors.Wrap(err)
	}

	commonPoliciesNames := lo.Intersect(reducedPoliciesNames, currentNetworkPoliciesName)
	err = r.handlePoliciesToUpdate(ctx, commonPoliciesNames, reducedPolicies)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil

}

func (r *NetworkPolicyHandler) handlePoliciesToDelete(ctx context.Context, policiesNamesToDelete []string, policiesByName NetworkPolicyByName) error {
	for _, policyName := range policiesNamesToDelete {
		err := r.handlePolicyDelete(ctx, policiesByName[policyName])
		if err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (r *NetworkPolicyHandler) handlePoliciesToAdd(ctx context.Context, policiesNamesToAdd []string, policiesByName NetworkPolicyByName) error {
	for _, policyName := range policiesNamesToAdd {
		err := r.client.Create(ctx, policiesByName[policyName])
		if err != nil {
			return r.handleCreationErrors(ctx, err, policiesByName[policyName])
		}
	}

	return nil
}

func (r *NetworkPolicyHandler) handlePoliciesToUpdate(ctx context.Context, policiesNames []string, policiesByName NetworkPolicyByName) error {
	for _, policyName := range policiesNames {

		existingPolicy := &v1.NetworkPolicy{}
		err := r.client.Get(ctx, types.NamespacedName{Name: policyName, Namespace: policiesByName[policyName].Namespace}, existingPolicy)

		// No matching network policy found to update, create one
		if k8serrors.IsNotFound(err) {
			err = r.client.Create(ctx, policiesByName[policyName])
			if err != nil {
				return r.handleCreationErrors(ctx, err, policiesByName[policyName])
			}
			continue
		}

		if err != nil {
			return errors.Wrap(err)
		}

		// Found existing matching policy, if it is identical to this one - do nothing
		if r.arePoliciesEqual(existingPolicy, policiesByName[policyName]) {
			continue
		}

		err = r.updatePolicy(ctx, existingPolicy, policiesByName[policyName])
		if err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (r *NetworkPolicyHandler) getAllOtterizeHandledPodsInNamespace(ctx context.Context, namespace string) (*corev1.PodList, error) {
	otterizeSelector, err := metav1.LabelSelectorAsSelector(
		&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      v2alpha1.OtterizeServiceLabelKey,
				Operator: metav1.LabelSelectorOpExists,
			},
		},
		})

	if err != nil {
		return &corev1.PodList{}, errors.Wrap(err)
	}

	podList := &corev1.PodList{}
	err = r.client.List(ctx, podList, client.InNamespace(namespace), client.MatchingLabelsSelector{Selector: otterizeSelector})
	if err != nil {
		return &corev1.PodList{}, errors.Wrap(err)
	}

	return podList, nil
}

func (r *NetworkPolicyHandler) reducedNetworkPoliciesInNamespace(ctx context.Context, pods []PotentiallyScrapeMetricPod) (NetworkPolicyByName, error) {
	reducedPolicies := make(NetworkPolicyByName)
	for _, pod := range pods {
		if !r.resourceMarkedForMetricsScraping(pod.scrapeResourceMeta) {
			continue
		}

		netpol, netpolCreated, err := r.buildNetworkPolicyIfNeeded(ctx, &pod)
		if err != nil {
			return make(NetworkPolicyByName), errors.Wrap(err)
		}

		if netpolCreated {
			reducedPolicies[netpol.Name] = &netpol
		}
	}

	return reducedPolicies, nil
}

func (r *NetworkPolicyHandler) getCurrentNetworkPoliciesInNamespace(ctx context.Context, namespace string, annotationFrom K8sResourceEnum) (NetworkPolicyByName, error) {
	metricsCollectorNetpolSelector, err := metav1.LabelSelectorAsSelector(
		&metav1.LabelSelector{MatchLabels: map[string]string{
			v2alpha1.OtterizeNetPolMetricsCollectorsLevel: string(annotationFrom)},
		})

	if err != nil {
		return make(NetworkPolicyByName), errors.Wrap(err)
	}

	networkPoliciesList := &v1.NetworkPolicyList{}
	err = r.client.List(ctx, networkPoliciesList, client.InNamespace(namespace), client.MatchingLabelsSelector{Selector: metricsCollectorNetpolSelector})
	if err != nil {
		return make(NetworkPolicyByName), errors.Wrap(err)
	}

	networkPolicies := make(NetworkPolicyByName)
	for _, netpol := range networkPoliciesList.Items {
		networkPolicies[netpol.Name] = &netpol
	}

	return networkPolicies, nil
}

func (r *NetworkPolicyHandler) buildNetworkPolicyIfNeeded(ctx context.Context, pod *PotentiallyScrapeMetricPod) (v1.NetworkPolicy, bool, error) {
	serverLabel, ok := pod.pod.Labels[v2alpha1.OtterizeServiceLabelKey]
	if !ok {
		// This should not really happen since we filtered only Otterize-affected pods before
		return v1.NetworkPolicy{}, false, nil
	}

	serviceId, err := serviceidresolver.NewResolver(r.client).ResolvePodToServiceIdentity(ctx, pod.pod)
	if err != nil {
		return v1.NetworkPolicy{}, false, errors.Wrap(err)
	}

	policyName := r.formatPolicyName(pod.scrapeResourceType, serviceId)

	// If configuration is set to "Always", we want to create the network policy regardless of other network policies
	if r.allowMetricsCollector == allowexternaltraffic.Always {
		netpol, errBuild := r.buildNetpolForPod(ctx, pod, policyName, serviceId, serverLabel)
		if errBuild != nil {
			return v1.NetworkPolicy{}, false, errors.Wrap(errBuild)
		}
		return netpol, true, nil

	}

	// If configuration is set to "Off", we want to delete the network policy regardless of other network policies
	if r.allowMetricsCollector == allowexternaltraffic.Off {
		return v1.NetworkPolicy{}, false, nil
	}

	// From this point we only handle the case where the configuration is set to "IfBlockedByOtterize", which means that
	// we want to create the network policy only if there are other network policies that block the traffic.

	netpolSlice, err := r.getAllApplicableNetworkPolicies(ctx, pod.pod, serviceId)
	if err != nil {
		return v1.NetworkPolicy{}, false, errors.Wrap(err)
	}

	foundNetpolWithIngressRule := lo.SomeBy(netpolSlice, func(netpol v1.NetworkPolicy) bool {
		return lo.Contains(netpol.Spec.PolicyTypes, v1.PolicyTypeIngress)
	})

	if !foundNetpolWithIngressRule {
		return v1.NetworkPolicy{}, false, nil
	}

	netpol, errBuild := r.buildNetpolForPod(ctx, pod, policyName, serviceId, serverLabel)
	if errBuild != nil {
		return v1.NetworkPolicy{}, false, errors.Wrap(errBuild)
	}
	return netpol, true, nil
}

func (r *NetworkPolicyHandler) getAllApplicableNetworkPolicies(ctx context.Context, pod *corev1.Pod, serviceId serviceidentity.ServiceIdentity) ([]v1.NetworkPolicy, error) {
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

func (r *NetworkPolicyHandler) buildNetpolForPod(ctx context.Context, pod *PotentiallyScrapeMetricPod, policyName string, identity serviceidentity.ServiceIdentity, serverLabel string) (v1.NetworkPolicy, error) {
	serviceIdentityLabels, ok, err := v2alpha1.ServiceIdentityToLabelsForWorkloadSelection(ctx, r.client, identity)
	if err != nil {
		return v1.NetworkPolicy{}, errors.Wrap(err)
	}
	if !ok {
		return v1.NetworkPolicy{}, errors.Wrap(err)
	}

	annotations := map[string]string{
		v2alpha1.OtterizeCreatedForServiceAnnotation: serviceIdentityLabels[v2alpha1.OtterizeServiceLabelKey],
	}

	rule := v1.NetworkPolicyIngressRule{}

	newPolicy := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: pod.pod.Namespace,
			Labels: map[string]string{
				v2alpha1.OtterizeNetPolMetricsCollectors:      serverLabel,
				v2alpha1.OtterizeNetPolMetricsCollectorsLevel: string(pod.scrapeResource),
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

	scrapePort, err := r.getMetricsPort(pod.scrapeResourceMeta)
	if err != nil {
		return v1.NetworkPolicy{}, errors.Wrap(err)
	}

	newPolicy.Spec.Ingress[0].Ports = append(newPolicy.Spec.Ingress[0].Ports, v1.NetworkPolicyPort{
		Port:     lo.ToPtr(intstr.IntOrString{IntVal: scrapePort, Type: intstr.Int}),
		Protocol: lo.ToPtr(corev1.ProtocolTCP),
	})

	return newPolicy, nil
}

func (r *NetworkPolicyHandler) handleCreationErrors(ctx context.Context, err error, policy *v1.NetworkPolicy) error {
	if !k8serrors.IsAlreadyExists(err) {
		return errors.Wrap(err)
	}

	// We tried to create a policy that already exists, let's try to update it
	existingPolicy := &v1.NetworkPolicy{}
	err = r.client.Get(ctx, types.NamespacedName{Name: policy.Name, Namespace: policy.Namespace}, existingPolicy)
	if err != nil {
		return errors.Wrap(err) // Don't retry anymore
	}
	return r.updatePolicy(ctx, existingPolicy, policy)
}

func (r *NetworkPolicyHandler) updatePolicy(ctx context.Context, existingPolicy *v1.NetworkPolicy, newPolicy *v1.NetworkPolicy) error {
	policyCopy := existingPolicy.DeepCopy()
	policyCopy.Labels = newPolicy.Labels
	policyCopy.Annotations = newPolicy.Annotations
	policyCopy.Spec = newPolicy.Spec

	err := r.client.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy))
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (r *NetworkPolicyHandler) arePoliciesEqual(existingPolicy *v1.NetworkPolicy, newPolicy *v1.NetworkPolicy) bool {
	return reflect.DeepEqual(existingPolicy.Spec, newPolicy.Spec) &&
		reflect.DeepEqual(existingPolicy.Labels, newPolicy.Labels) &&
		reflect.DeepEqual(existingPolicy.Annotations, newPolicy.Annotations)
}

func (r *NetworkPolicyHandler) handlePolicyDelete(ctx context.Context, networkPolicy *v1.NetworkPolicy) error {
	err := r.client.Delete(ctx, networkPolicy)
	if k8serrors.IsNotFound(err) {
		return nil
	}

	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (r *NetworkPolicyHandler) formatPolicyName(ownerResourceKind *metav1.TypeMeta, serviceId serviceidentity.ServiceIdentity) string {
	return fmt.Sprintf(OtterizeMetricsCollectorPolicyNameTemplate, strings.ToLower(ownerResourceKind.Kind), strings.ToLower(serviceId.Name))
}

func (r *NetworkPolicyHandler) resourceMarkedForMetricsScraping(resource *metav1.ObjectMeta) bool {
	shouldScrape := resource.Annotations["prometheus.io/scrape"]
	scrapePort := resource.Annotations["prometheus.io/port"]
	return shouldScrape == "true" && scrapePort != ""
}

func (r *NetworkPolicyHandler) getMetricsPort(resource *metav1.ObjectMeta) (int32, error) {
	scrapePort := resource.Annotations["prometheus.io/port"]
	if scrapePort == "" {
		// Port is not defined for this resource, although it ts marked for scraping - but we don't want to create a network policy.
		// Prometheus default behavior would be to try and scrape all ports defined, but we don't want to allow that.
		return 0, errors.Wrap(fmt.Errorf("resource does not have a prometheus.io/port annotation"))
	}

	port, err := strconv.Atoi(scrapePort)
	if err != nil {
		logrus.Errorf("failed to convert port to int: %s", err)
		return 0, errors.Wrap(err)
	}

	return int32(port), nil
}
