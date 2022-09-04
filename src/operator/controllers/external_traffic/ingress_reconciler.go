package external_traffic

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const OtterizeIngressNetworkPolicyNameTemplate = "ingress-external-access-to-%s"

type IngressReconciler struct {
	client           client.Client
	Scheme           *runtime.Scheme
	netpolReconciler *NetworkPolicyReconciler
	injectablerecorder.InjectableRecorder
}

func NewIngressReconciler(client client.Client, scheme *runtime.Scheme) *IngressReconciler {
	return &IngressReconciler{client: client, Scheme: scheme, netpolReconciler: NewNetworkPolicyReconciler(client)}
}

func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	recorder := mgr.GetEventRecorderFor("intents-operator")
	r.InjectRecorder(recorder)
	r.netpolReconciler.InjectRecorder(recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Ingress{}).
		Complete(r)
}

func (r *IngressReconciler) formatPolicyName(serviceName string) string {
	return fmt.Sprintf(OtterizeIngressNetworkPolicyNameTemplate, serviceName)
}

func (r *IngressReconciler) serviceFromIngressBackendService(ctx context.Context, namespace string, backend *v1.IngressServiceBackend) (*corev1.Service, error) {
	svc := &corev1.Service{}
	err := r.client.Get(ctx, types.NamespacedName{Name: backend.Name, Namespace: namespace}, svc)
	if err != nil {
		return nil, err
	}

	return svc, nil
}

func (r *IngressReconciler) ingressFromNetworkPolicy(ctx context.Context, networkPolicy *v1.NetworkPolicy) (*v1.Ingress, error) {
	for _, ownerRef := range networkPolicy.OwnerReferences {
		if ownerRef.Kind == "Ingress" {
			ingress := &v1.Ingress{}
			err := r.client.Get(ctx, types.NamespacedName{Name: ownerRef.Name, Namespace: networkPolicy.Namespace}, ingress)
			if err != nil {
				return nil, err
			}
			return ingress, nil
		}
	}

	return nil, k8serrors.NewNotFound(v1.Resource("ingress"), networkPolicy.Name)
}

// ReconcileService updates the network policy for an ingress and all its services when one of the related services is
// updated.
// When a service is updated, its labels may have changed, and the network policy needs to be refreshed.
// When a service's labels change, the ingress does not change at all, and yet we need to update
// the policy.
// This only handles service updates - if a service was missing at the time the ingress was created,
// reconciliation for the ingress itself will fail and will be retried.
func (r *IngressReconciler) ReconcileService(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	existingPolicy, existingPolicyErr := r.getExistingPolicyForService(ctx, req.Name, req.Namespace)

	if existingPolicyErr != nil {
		if k8serrors.IsNotFound(existingPolicyErr) {
			// Policy does not exist, so service is unrelated to an ingress, nothing to do
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, existingPolicyErr
	}

	ingress, err := r.ingressFromNetworkPolicy(ctx, existingPolicy)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// no ingress for this network policy, nothing to do here
			return ctrl.Result{}, nil
		}
	}

	return r.reconcileIngress(ctx, ingress)
}

func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ingress := &v1.Ingress{}
	err := r.client.Get(ctx, req.NamespacedName, ingress)
	if k8serrors.IsNotFound(err) {
		// delete is handled by garbage collection - the ingress owns the network policy - nothing to do here
		return ctrl.Result{}, nil
	}

	return r.reconcileIngress(ctx, ingress)
}

func (r *IngressReconciler) reconcileIngress(ctx context.Context, ingress *v1.Ingress) (ctrl.Result, error) {
	services := make(map[string]*corev1.Service)
	if ingress.Spec.DefaultBackend != nil {
		svc, err := r.serviceFromIngressBackendService(ctx, ingress.Namespace, ingress.Spec.DefaultBackend.Service)
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
			svc, err := r.serviceFromIngressBackendService(ctx, ingress.Namespace, path.Backend.Service)
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
		err := r.netpolReconciler.handleNetworkPolicyCreationOrUpdate(ctx, service, ingress, r.formatPolicyName(service.Name))
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
	policyName := r.formatPolicyName(name)
	existingPolicy := &v1.NetworkPolicy{}
	err := r.client.Get(ctx, types.NamespacedName{Name: policyName, Namespace: namespace}, existingPolicy)
	if err != nil {
		return nil, err
	}

	return existingPolicy, nil
}
