package port_network_policy

import (
	"context"
	"github.com/otterize/intents-operator/src/operator/controllers/protected_service_reconcilers"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/operator/mirrorevents"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
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
	injectablerecorder.InjectableRecorder
	serviceEffectivePolicyReconciler *effectivepolicy.GroupReconciler
	defaultDenyReconciler            *protected_service_reconcilers.DefaultDenyReconciler
}

func NewServiceWatcher(c client.Client, eventRecorder record.EventRecorder, serviceEffectivePolicyReconciler *effectivepolicy.GroupReconciler, netpolEnabled bool) *ServiceWatcher {
	recorder := injectablerecorder.InjectableRecorder{Recorder: eventRecorder}
	sw := &ServiceWatcher{
		Client:                           c,
		InjectableRecorder:               recorder,
		serviceEffectivePolicyReconciler: serviceEffectivePolicyReconciler,
	}
	if netpolEnabled {
		sw.defaultDenyReconciler = protected_service_reconcilers.NewDefaultDenyReconciler(c, netpolEnabled)
	}
	return sw
}

func (r *ServiceWatcher) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	err := r.serviceEffectivePolicyReconciler.Reconcile(ctx)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	if r.defaultDenyReconciler != nil {
		res, err := r.defaultDenyReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: req.Namespace}})
		if err != nil || res.Requeue {
			return res, errors.Wrap(err)
		}
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
	recorder := mirrorevents.GetMirrorToClientIntentsEventRecorderFor(mgr, "intents-operator")
	r.InjectRecorder(recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}
