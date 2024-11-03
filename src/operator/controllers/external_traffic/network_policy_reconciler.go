package external_traffic

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

//+kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;update;patch;list;watch;delete;create

type NetworkPolicyReconciler struct {
	client.Client
	extNetpolHandler *NetworkPolicyHandler
	injectablerecorder.InjectableRecorder
}

func NewNetworkPolicyReconciler(
	client client.Client,
	extNetpolHandler *NetworkPolicyHandler,
) *NetworkPolicyReconciler {
	return &NetworkPolicyReconciler{
		Client:           client,
		extNetpolHandler: extNetpolHandler,
	}
}

func (r *NetworkPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	recorder := mgr.GetEventRecorderFor("intents-operator")
	r.InjectRecorder(recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.NetworkPolicy{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}

// Reconcile handles network policy creation, update and delete. In all of these cases, it calls the NetworkPolicyHandler
// to handle the pods in the namespace of the network policy.
func (r *NetworkPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logrus.Debugf("Handling external for NetworkPolicy for namespace: %s", req.Namespace)
	err := r.extNetpolHandler.HandlePodsByNamespace(ctx, req.Namespace)
	if k8serrors.IsConflict(err) || k8serrors.IsNotFound(err) || k8serrors.IsForbidden(err) || k8serrors.IsAlreadyExists(err) {
		return ctrl.Result{Requeue: true}, nil
	}
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	return ctrl.Result{}, nil
}
