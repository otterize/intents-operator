package svc_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type ServiceWatcher struct {
	client.Client
	networkPolicyReconciler *intents_reconcilers.NetworkPolicyReconciler
	injectablerecorder.InjectableRecorder
}

func NewServiceWatcher(c client.Client, eventRecorder record.EventRecorder, networkPolicyReconciler *intents_reconcilers.NetworkPolicyReconciler) *ServiceWatcher {
	recorder := injectablerecorder.InjectableRecorder{Recorder: eventRecorder}
	return &ServiceWatcher{
		Client:                  c,
		InjectableRecorder:      recorder,
		networkPolicyReconciler: networkPolicyReconciler,
	}
}

func (p *ServiceWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// We reconcile everytime a service changes.
	// When a target port changes netpols should be updated, etc.
	targetServerName := fmt.Sprintf("svc:%s.%s", req.Name, req.Namespace)
	var intentsList otterizev1alpha2.ClientIntentsList
	err := p.List(
		ctx, &intentsList,
		&client.MatchingFields{otterizev1alpha2.OtterizeTargetServerIndexField: targetServerName})

	if err != nil {
		return ctrl.Result{}, err
	}

	// Reconcile for any clientIntent in the cluster that points to the service enqueued in the request
	for _, clientIntent := range intentsList.Items {
		res, err := p.networkPolicyReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
			Namespace: clientIntent.Namespace,
			Name:      clientIntent.Name,
		}})
		if err != nil {
			return ctrl.Result{}, err
		}
		if res.Requeue {
			return res, nil
		}
	}
	return ctrl.Result{}, nil
}

func (p *ServiceWatcher) SetupWithManager(mgr manager.Manager) error {
	recorder := mgr.GetEventRecorderFor("intents-operator")
	p.InjectRecorder(recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(p)
}
