package intents_reconcilers

import (
	"context"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ServiceEffectivePolicyIntentsReconciler struct {
	client.Client
	Scheme                           *runtime.Scheme
	serviceEffectivePolicyReconciler *effectivepolicy.Syncer
	injectablerecorder.InjectableRecorder
}

func NewServiceEffectiveIntentsReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	serviceEffectivePolicySyncer *effectivepolicy.Syncer) *ServiceEffectivePolicyIntentsReconciler {

	return &ServiceEffectivePolicyIntentsReconciler{
		Client:                           client,
		Scheme:                           scheme,
		serviceEffectivePolicyReconciler: serviceEffectivePolicySyncer,
	}
}

func (r *ServiceEffectivePolicyIntentsReconciler) Reconcile(ctx context.Context, _ reconcile.Request) (ctrl.Result, error) {
	errors := r.serviceEffectivePolicyReconciler.Sync(ctx)
	if len(errors) > 0 {
		return ctrl.Result{}, errors[0]
	}

	return ctrl.Result{}, nil
}

func (r *ServiceEffectivePolicyIntentsReconciler) InjectRecorder(recorder record.EventRecorder) {
	r.Recorder = recorder
	if r.serviceEffectivePolicyReconciler != nil {
		r.serviceEffectivePolicyReconciler.InjectRecorder(recorder)
	}
}
