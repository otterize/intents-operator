package external_traffic

import (
	"context"
	"fmt"
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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const OtterizeIngressNetworkPolicyNameTemplate = "ingress-external-access-to-%s-from-%s"

type IngressReconciler struct {
	client client.Client
	Scheme *runtime.Scheme
	injectablerecorder.InjectableRecorder
}

func NewIngressReconciler(client client.Client, scheme *runtime.Scheme) *IngressReconciler {
	return &IngressReconciler{client: client, Scheme: scheme}
}

func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.InjectRecorder(mgr.GetEventRecorderFor("intents-operator"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Ingress{}).
		Complete(r)
}

func formatIngressPolicyName(name string, namespace string) string {
	return fmt.Sprintf(OtterizeIngressNetworkPolicyNameTemplate, name, namespace)
}

func (r *IngressReconciler) serviceFromIngressBackendService(ctx context.Context, namespace string, backend *v1.IngressServiceBackend) (*corev1.Service, error) {
	svc := &corev1.Service{}
	err := r.client.Get(ctx, types.NamespacedName{Name: backend.Name, Namespace: namespace}, svc)
	if err != nil {
		return nil, err
	}

	return svc, nil
}

func (r *IngressReconciler) ReconcileService(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	existingPolicy, existingPolicyErr := r.getExistingPolicyForService(ctx, req.Name, req.Namespace)
	service := &corev1.Service{}
	getServiceErr := r.client.Get(ctx, req.NamespacedName, service)
	if k8serrors.IsNotFound(getServiceErr) {
		if existingPolicyErr != nil {
			if k8serrors.IsNotFound(existingPolicyErr) {
				// no policy, nothing to do
				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, getServiceErr
		}
		getServiceErr = r.client.Delete(ctx, existingPolicy)
		if getServiceErr != nil {
			return ctrl.Result{}, getServiceErr
		}
		return ctrl.Result{}, nil
	}

	// Service exists
	if existingPolicyErr != nil {
		if k8serrors.IsNotFound(existingPolicyErr) {
			// Policy does not exist, so service is unrelated to an ingress
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, existingPolicyErr
	}

	if len(existingPolicy.OwnerReferences) != 0 && existingPolicy.OwnerReferences[0].Kind == "Ingress" {
		err := r.handleNetworkPolicyCreationOrUpdate(ctx, service, nil)
		if err != nil {
			if k8serrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ingress := &v1.Ingress{}
	err := r.client.Get(ctx, req.NamespacedName, ingress)
	if k8serrors.IsNotFound(err) {
		// delete is handled by garbage collection - the ingress owns the network policy
		return ctrl.Result{}, nil
	}

	services := make(map[string]*corev1.Service)
	if ingress.Spec.DefaultBackend != nil {
		svc, err := r.serviceFromIngressBackendService(ctx, req.Namespace, ingress.Spec.DefaultBackend.Service)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				// The service might not exist yet, if the ingress was created first. Retry later.
				return ctrl.Result{Requeue: true}, nil
			}
		}
		services[ingress.Spec.DefaultBackend.Service.Name] = svc
	}

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

	for _, service := range services {
		ownerRefs := []metav1.OwnerReference{{
			APIVersion: ingress.APIVersion,
			Kind:       ingress.Kind,
			Name:       ingress.Name,
			UID:        ingress.UID,
		}}
		err := r.handleNetworkPolicyCreationOrUpdate(ctx, service, ownerRefs)
		if err != nil {
			if k8serrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *IngressReconciler) getExistingPolicyForService(ctx context.Context, name string, namespace string) (*v1.NetworkPolicy, error) {
	policyName := formatIngressPolicyName(name, namespace)
	existingPolicy := &v1.NetworkPolicy{}
	err := r.client.Get(ctx, types.NamespacedName{Name: policyName, Namespace: namespace}, existingPolicy)
	if err != nil {
		return nil, err
	}

	return existingPolicy, nil
}

func (r *IngressReconciler) handleNetworkPolicyCreationOrUpdate(
	ctx context.Context, service *corev1.Service, ownerRefsOverride []metav1.OwnerReference) error {

	existingPolicy, err := r.getExistingPolicyForService(ctx, service.Name, service.Namespace)

	newPolicy := r.buildNetworkPolicyObjectForService(service)
	if ownerRefsOverride != nil {
		newPolicy.SetOwnerReferences(ownerRefsOverride)
	}

	// No matching network policy found, create one
	if k8serrors.IsNotFound(err) {
		logrus.Infof(
			"Creating network policy to enable access from external traffic to ingress service %s (ns %s)", service.Name, service.Namespace)
		err := r.client.Create(ctx, newPolicy)
		if err != nil {
			r.RecordWarningEvent(service, "failed to create external traffic network policy", err.Error())
			return err
		}
		return nil

	} else if err != nil {
		r.RecordWarningEvent(service, "failed to get external traffic network policy", err.Error())
		return err
	}

	// Found matching policy, is an update needed?
	if reflect.DeepEqual(existingPolicy.Spec, newPolicy.Spec) {
		return nil
	}

	policyCopy := existingPolicy.DeepCopy()
	policyCopy.Spec = newPolicy.Spec
	if newPolicy.OwnerReferences != nil {
		policyCopy.SetOwnerReferences(newPolicy.GetOwnerReferences())
	}

	err = r.client.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy))
	if err != nil {
		return err
	}

	return nil
}

func (r *IngressReconciler) buildNetworkPolicyObjectForService(
	service *corev1.Service) *v1.NetworkPolicy {
	serviceSpecCopy := service.Spec.DeepCopy()

	netpol := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      formatIngressPolicyName(service.Name, service.Namespace),
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
