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

const OtterizeIngressNetworkPolicyNameTemplate = "ingress-external-access-to-svc-%s-from-ingress"

type IngressReconciler struct {
	client client.Client
	Scheme *runtime.Scheme
}

func (r *IngressReconciler) serviceFromIngressBackendService(ctx context.Context, namespace string, backend *v1.IngressServiceBackend) (*corev1.Service, error) {
	svc := &corev1.Service{}
	err := r.client.Get(ctx, types.NamespacedName{Name: backend.Name, Namespace: namespace}, svc)
	if err != nil {
		return nil, err
	}

	return svc, nil
}

func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ingress := &v1.Ingress{}
	err := r.client.Get(ctx, req.NamespacedName, ingress)
	if k8serrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	services := make(map[string]*corev1.Service)
	svc, err := r.serviceFromIngressBackendService(ctx, req.Namespace, ingress.Spec.DefaultBackend.Service)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// The service might not exist yet, if the ingress was created first. Retry later.
			return ctrl.Result{Requeue: true}, nil
		}
	}
	services[ingress.Spec.DefaultBackend.Service.Name] = svc

	for _, rule := range ingress.Spec.Rules {
		for _, path := range rule.HTTP.Paths {
			svc, err := r.serviceFromIngressBackendService(ctx, req.Namespace, path.Backend.Service)
			if err != nil {
				if k8serrors.IsNotFound(err) {
					// The service might not exist yet, if the ingress was created first. Retry later.
					return ctrl.Result{Requeue: true}, nil
				}
			}
			services[path.Backend.Service.Name] = svc
		}
	}

	err = r.handleNetworkPolicyCreation(ctx, ingress, services)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *IngressReconciler) handleNetworkPolicyCreation(
	ctx context.Context, ingress *v1.Ingress, services map[string]*corev1.Service) error {

	for _, service := range services {
		policyName := fmt.Sprintf(OtterizeIngressNetworkPolicyNameTemplate, service.Name)
		err := r.client.Get(ctx, types.NamespacedName{Name: policyName, Namespace: ingress.Namespace}, &v1.NetworkPolicy{})

		// No matching network policy found, create one
		if k8serrors.IsNotFound(err) {
			logrus.Infof(
				"Creating network policy to enable access from external traffic to load balancer service %s (ns %s)", service.Name, service.Namespace)

			policy := r.buildNetworkPolicyObjectForService(ingress, service, policyName)
			err := r.client.Create(ctx, policy)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}

	return nil
}

// buildNetworkPolicyObjectForService builds the network policy that represents the intent from the parameter
func (r *IngressReconciler) buildNetworkPolicyObjectForService(
	ingress *v1.Ingress, service *corev1.Service, policyName string) *v1.NetworkPolicy {
	serviceSpecCopy := service.Spec.DeepCopy()
	netpol := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: ingress.Namespace,
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
