package external_traffic

import (
	"context"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"
)

const (
	ReasonCreatingExternalTrafficPolicyFailed = "CreatingExternalTrafficPolicyFailed"
	ReasonCreatedExternalTrafficPolicy        = "CreatedExternalTrafficPolicy"
	ReasonGettingExternalTrafficPolicyFailed  = "GettingExternalTrafficPolicyFailed"
	ReasonRemovingExternalTrafficPolicy       = "RemovingExternalTrafficPolicy"
	ReasonRemovingExternalTrafficPolicyFailed = "RemovingExternalTrafficPolicyFailed"
	ReasonRemovedExternalTrafficPolicy        = "RemovedExternalTrafficPolicy"
)

type NetworkPolicyCreator struct {
	client client.Client
	scheme *runtime.Scheme
	injectablerecorder.InjectableRecorder
	enabled bool
}

func NewNetworkPolicyCreator(client client.Client, scheme *runtime.Scheme, enabled bool) *NetworkPolicyCreator {
	return &NetworkPolicyCreator{client: client, scheme: scheme, enabled: enabled}
}

func (r *NetworkPolicyCreator) handleNetworkPolicyCreationOrUpdate(
	ctx context.Context, endpoints *corev1.Endpoints, owner client.Object, otterizeServiceName string, eventsObject client.Object, netpol *v1.NetworkPolicy, ingressList *v1.IngressList, policyName string) error {

	existingPolicy := &v1.NetworkPolicy{}
	errGetExistingPolicy := r.client.Get(ctx, types.NamespacedName{Name: policyName, Namespace: endpoints.GetNamespace()}, existingPolicy)
	newPolicy := buildNetworkPolicyObjectForService(endpoints, otterizeServiceName, netpol.Spec.PodSelector, ingressList, policyName)
	err := controllerutil.SetOwnerReference(owner, newPolicy, r.scheme)
	if err != nil {
		return err
	}

	// No matching network policy found, create one
	if k8serrors.IsNotFound(errGetExistingPolicy) {
		if r.enabled {
			logrus.Infof(
				"Creating network policy to enable access from external traffic to external-facing service %s (ns %s)", endpoints.GetName(), endpoints.GetNamespace())
			err := r.client.Create(ctx, newPolicy)
			if err != nil {
				r.RecordWarningEventf(eventsObject, ReasonCreatingExternalTrafficPolicyFailed, "failed to create external traffic network policy: %s", err.Error())
				return err
			}
			r.RecordNormalEventf(eventsObject, ReasonCreatedExternalTrafficPolicy, "created external traffic network policy. service '%s' refers to pods protected by network policy '%s'", endpoints.GetName(), netpol.GetName())
		}
		return nil
	} else if errGetExistingPolicy != nil {
		r.RecordWarningEventf(eventsObject, ReasonGettingExternalTrafficPolicyFailed, "failed to get external traffic network policy: %s", err.Error())
		return errGetExistingPolicy
	}

	if !r.enabled {
		r.RecordNormalEvent(eventsObject, ReasonRemovingExternalTrafficPolicy, "removing external traffic network policy, reconciler was disabled")
		err := r.client.Delete(ctx, existingPolicy)
		if err != nil {
			r.RecordWarningEventf(eventsObject, ReasonRemovingExternalTrafficPolicyFailed, "failed removing external traffic network policy: %s", err.Error())
			return err
		}
		r.RecordNormalEvent(eventsObject, ReasonRemovedExternalTrafficPolicy, "removed external traffic network policy, success")
		return nil
	}

	// Found matching policy, is an update needed?
	if reflect.DeepEqual(existingPolicy.Spec, newPolicy.Spec) &&
		reflect.DeepEqual(existingPolicy.Labels, newPolicy.Labels) &&
		reflect.DeepEqual(existingPolicy.Annotations, newPolicy.Annotations) &&
		reflect.DeepEqual(existingPolicy.OwnerReferences, newPolicy.OwnerReferences) {
		return nil
	}

	policyCopy := existingPolicy.DeepCopy()
	policyCopy.Labels = newPolicy.Labels
	policyCopy.Annotations = newPolicy.Annotations
	policyCopy.Spec = newPolicy.Spec
	err = controllerutil.SetOwnerReference(owner, policyCopy, r.scheme)
	if err != nil {
		return err
	}

	err = r.client.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy))
	if err != nil {
		return err
	}

	return nil
}

func buildNetworkPolicyObjectForService(
	service *corev1.Endpoints, otterizeServiceName string, selector metav1.LabelSelector, ingressList *v1.IngressList, policyName string) *v1.NetworkPolicy {
	serviceSpecCopy := service.Subsets

	annotations := map[string]string{
		v1alpha1.OtterizeCreatedForServiceAnnotation: service.GetName(),
	}

	if len(ingressList.Items) != 0 {
		annotations[v1alpha1.OtterizeCreatedForIngressAnnotation] = strings.Join(lo.Map(ingressList.Items, func(ingress v1.Ingress, _ int) string {
			return ingress.Name
		}), ",")
	}

	netpol := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: service.Namespace,
			Labels: map[string]string{
				v1alpha1.OtterizeNetworkPolicyExternalTraffic: otterizeServiceName,
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
