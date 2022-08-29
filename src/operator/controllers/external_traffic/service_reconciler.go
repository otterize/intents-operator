package external_traffic

import (
	"context"
	"fmt"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const OtterizeNetworkPolicyNameTemplate = "external-access-to-%s-from-%s"
const OtterizeNetworkPolicy = "otterize/network-policy"

type ServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	svc := &corev1.Service{}
	err := r.Get(ctx, req.NamespacedName, svc)
	if k8serrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}

	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer && svc.Spec.Type != corev1.ServiceTypeNodePort {
		return ctrl.Result{}, nil
	}
	err = r.handleNetworkPolicyCreation(ctx, svc)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ServiceReconciler) handleNetworkPolicyCreation(
	ctx context.Context, service *corev1.Service) error {

	policyName := fmt.Sprintf(OtterizeNetworkPolicyNameTemplate, service.Name, service.Namespace)
	policy := &v1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: policyName, Namespace: service.Namespace}, policy)

	// No matching network policy found, create one
	if k8serrors.IsNotFound(err) {
		logrus.Infof(
			"Creating network policy to enable access from external traffic to load balancer service %s (ns %s)", service.Name, service.Namespace)
		policy := r.buildNetworkPolicyObjectForService(service, policyName)
		err := r.Create(ctx, policy)
		if err != nil {
			return err
		}
		return nil

	} else if err != nil {
		return err
	}

	return nil
}

// buildNetworkPolicyObjectForService builds the network policy that represents the intent from the parameter
func (r *ServiceReconciler) buildNetworkPolicyObjectForService(
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
		netpolPort := v1.NetworkPolicyPort{}
		if port.Port != 0 {
			netpolPort.Port = lo.ToPtr(intstr.FromInt(int(port.Port)))
		}
		if len(port.Protocol) != 0 {
			netpolPort.Protocol = lo.ToPtr(port.Protocol)
		}
		netpol.Spec.Ingress[0].Ports = append(netpol.Spec.Ingress[0].Ports, netpolPort)
	}

	return netpol
}
