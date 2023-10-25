package port_network_policy

import (
	"context"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ServiceWatcher struct {
	client.Client
	reconcilers []reconcile.Reconciler
	injectablerecorder.InjectableRecorder
}

func NewServiceWatcher(c client.Client, eventRecorder record.EventRecorder, reconcilers []reconcile.Reconciler) *ServiceWatcher {
	recorder := injectablerecorder.InjectableRecorder{Recorder: eventRecorder}
	return &ServiceWatcher{
		Client:             c,
		InjectableRecorder: recorder,
		reconcilers:        reconcilers,
	}
}

func (r *ServiceWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// We should reconcile everytime a service changes.
	// When a target port changes netpols should be updated, etc.
	service := corev1.Service{}
	err := r.Get(ctx, req.NamespacedName, &service)
	if err != nil && !k8serrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	var intentsList otterizev1alpha2.ClientIntentsList
	err = r.List(
		ctx, &intentsList,
		&client.MatchingFields{otterizev1alpha2.OtterizeTargetServerIndexField: fmt.Sprintf("svc:%s.%s", req.Name, req.Namespace)})

	if err != nil {
		return ctrl.Result{}, err
	}

	// Reconcile for any clientIntent in the cluster that points to the service enqueued in the request
	for _, clientIntent := range intentsList.Items {
		for _, reconciler := range r.reconcilers {
			res, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Namespace: clientIntent.Namespace,
				Name:      clientIntent.Name,
			}})
			if err != nil {
				return ctrl.Result{}, err
			}
			if !res.IsZero() {
				return res, nil
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *ServiceWatcher) SetupWithManager(mgr manager.Manager) error {
	recorder := mgr.GetEventRecorderFor("intents-operator")
	r.InjectRecorder(recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}
