package external_traffic

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/reconcilergroup"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ExternalTrafficReconciler reconciles Services of type LoadBalancer, NodePort and Ingress resources and creates network
// policies allowing their traffic.
type ExternalTrafficReconciler struct {
	group *reconcilergroup.Group
}

func NewExternalTrafficReconciler(client client.Client, scheme *runtime.Scheme) *ExternalTrafficReconciler {
	return &ExternalTrafficReconciler{
		group: reconcilergroup.NewGroup("external-traffic-reconciler", client, scheme,
			&ServiceReconciler{Client: client, Scheme: scheme},
		)}
}

//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
//+kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;update;patch;list;watch;delete;create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ExternalTrafficReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.group.Reconcile(ctx, req)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ExternalTrafficReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Complete(r)
}
