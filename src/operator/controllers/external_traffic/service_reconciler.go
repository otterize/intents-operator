package external_traffic

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const OtterizeNetworkPolicyNameTemplate = "external-access-to-%s"
const OtterizeNetworkPolicy = "otterize/network-policy"

//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
//+kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;update;patch;list;watch;delete;create

type ServiceReconciler struct {
	client            client.Client
	Scheme            *runtime.Scheme
	ingressReconciler *IngressReconciler
	netpolReconciler  *NetworkPolicyReconciler
	injectablerecorder.InjectableRecorder
}

func (r *ServiceReconciler) formatPolicyName(serviceName string) string {
	return fmt.Sprintf(OtterizeNetworkPolicyNameTemplate, serviceName)
}

func NewServiceReconciler(client client.Client, scheme *runtime.Scheme, ingressReconciler *IngressReconciler) *ServiceReconciler {
	return &ServiceReconciler{
		client:            client,
		Scheme:            scheme,
		ingressReconciler: ingressReconciler,
		netpolReconciler:  NewNetworkPolicyReconciler(client),
	}
}

func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	recorder := mgr.GetEventRecorderFor("intents-operator")
	r.InjectRecorder(recorder)
	r.netpolReconciler.InjectRecorder(recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Complete(r)
}

func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	res, err := r.ingressReconciler.ReconcileService(ctx, req)
	if err != nil || !res.IsZero() {
		return res, err
	}

	svc := &corev1.Service{}
	err = r.client.Get(ctx, req.NamespacedName, svc)
	if k8serrors.IsNotFound(err) {
		// delete is handled by garbage collection - the service owns the network policy
		return ctrl.Result{}, nil
	}

	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer && svc.Spec.Type != corev1.ServiceTypeNodePort {
		return ctrl.Result{}, nil
	}
	err = r.netpolReconciler.handleNetworkPolicyCreationOrUpdate(ctx, svc, svc, r.formatPolicyName(svc.Name))
	if err != nil {
		if k8serrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}
