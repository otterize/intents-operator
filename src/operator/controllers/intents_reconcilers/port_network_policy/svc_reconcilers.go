package port_network_policy

import (
	"context"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ServiceWatcher struct {
	client.Client
	injectablerecorder.InjectableRecorder
	serviceEffectivePolicyReconciler *effectivepolicy.GroupReconciler
}

func NewServiceWatcher(c client.Client, eventRecorder record.EventRecorder, serviceEffectivePolicyReconciler *effectivepolicy.GroupReconciler) *ServiceWatcher {
	recorder := injectablerecorder.InjectableRecorder{Recorder: eventRecorder}
	return &ServiceWatcher{
		Client:                           c,
		InjectableRecorder:               recorder,
		serviceEffectivePolicyReconciler: serviceEffectivePolicyReconciler,
	}
}

func (r *ServiceWatcher) Reconcile(ctx context.Context, _ reconcile.Request) (ctrl.Result, error) {
	err := r.serviceEffectivePolicyReconciler.Reconcile(ctx)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	return ctrl.Result{}, nil
}

func (r *ServiceWatcher) InjectRecorder(recorder record.EventRecorder) {
	r.Recorder = recorder
	if r.serviceEffectivePolicyReconciler != nil {
		r.serviceEffectivePolicyReconciler.InjectRecorder(recorder)
	}
}

func (r *ServiceWatcher) SetupWithManager(mgr manager.Manager) error {
	recorder := mgr.GetEventRecorderFor("intents-operator")
	r.InjectRecorder(recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}
