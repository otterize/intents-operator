package external_traffic

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NetworkPolicyReconciler struct {
	client client.Client
	injectablerecorder.InjectableRecorder
	enabled bool
}

func NewNetworkPolicyReconciler(client client.Client, enabled bool) *NetworkPolicyReconciler {
	return &NetworkPolicyReconciler{client: client, enabled: enabled}
}

func (r *NetworkPolicyReconciler) handleNetworkPolicyCreationOrUpdate(
	ctx context.Context, service *corev1.Service, policyOwner client.Object, policyName string) error {

	existingPolicy := &v1.NetworkPolicy{}
	err := r.client.Get(ctx, types.NamespacedName{Name: policyName, Namespace: service.GetNamespace()}, existingPolicy)
	newPolicy := buildNetworkPolicyObjectForService(service, policyName)
	apiVersion, kind := policyOwner.GetObjectKind().GroupVersionKind().ToAPIVersionAndKind()
	newPolicy.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       policyOwner.GetName(),
		UID:        policyOwner.GetUID(),
	}})

	// No matching network policy found, create one
	if k8serrors.IsNotFound(err) {
		logrus.Infof(
			"Creating network policy to enable access from external traffic to load balancer service %s (ns %s)", service.GetName(), service.GetNamespace())
		err := r.client.Create(ctx, newPolicy)
		if err != nil {
			r.RecordWarningEvent(policyOwner, "failed to create external traffic network policy", err.Error())
			return err
		}
		return nil

	} else if err != nil {
		r.RecordWarningEvent(policyOwner, "failed to get external traffic network policy", err.Error())
		return err
	}

	if !r.enabled {
		r.RecordNormalEvent(policyOwner, "removing external traffic network policy", "reconciler was disabled")
		err := r.client.Delete(ctx, existingPolicy)
		if err != nil {
			r.RecordWarningEvent(policyOwner, "failed removing external traffic network policy", err.Error())
			return err
		}
		r.RecordNormalEvent(policyOwner, "removed external traffic network policy", "success")
		return nil
	}

	// Found matching policy, is an update needed?
	if reflect.DeepEqual(existingPolicy.Spec, newPolicy.Spec) {
		return nil
	}

	policyCopy := existingPolicy.DeepCopy()
	policyCopy.Spec = newPolicy.Spec
	policyCopy.SetOwnerReferences(newPolicy.GetOwnerReferences())

	err = r.client.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy))
	if err != nil {
		return err
	}

	return nil
}

func buildNetworkPolicyObjectForService(
	service *corev1.Service, policyName string) *v1.NetworkPolicy {
	serviceSpecCopy := service.Spec.DeepCopy()

	netpol := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: service.Namespace,
			Labels: map[string]string{
				OtterizeNetworkPolicy: "true",
			},
		},
		Spec: v1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: serviceSpecCopy.Selector,
			},
			Ingress: []v1.NetworkPolicyIngressRule{
				{},
			},
		},
	}

	for _, port := range serviceSpecCopy.Ports {
		netpolPort := v1.NetworkPolicyPort{
			Port: lo.ToPtr(port.TargetPort),
		}

		if port.TargetPort.IntVal == 0 && len(port.TargetPort.StrVal) == 0 {
			netpolPort.Port = lo.ToPtr(intstr.FromInt(int(port.Port)))
		}

		if len(port.Protocol) != 0 {
			netpolPort.Protocol = lo.ToPtr(port.Protocol)
		}
		netpol.Spec.Ingress[0].Ports = append(netpol.Spec.Ingress[0].Ports, netpolPort)
	}

	return netpol
}
