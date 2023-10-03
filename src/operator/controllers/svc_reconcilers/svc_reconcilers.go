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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
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
	// We reconcile everytime a service changes, when a service is deleted netpols should get removed,
	// when a target port changes netpols should be updated, etc.
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

// InitIntentsServerIndices indexes intents by target server name
// This is used in finalizers to determine whether a network policy should be removed from the target namespace
func (p *ServiceWatcher) InitIntentsServerIndices(mgr ctrl.Manager) error {
	err := mgr.GetCache().IndexField(
		context.Background(),
		&otterizev1alpha2.ClientIntents{},
		otterizev1alpha2.OtterizeTargetServerIndexField,
		func(object client.Object) []string {
			var res []string
			intents := object.(*otterizev1alpha2.ClientIntents)
			if intents.Spec == nil {
				return nil
			}

			for _, intent := range intents.GetCallsList() {
				res = append(res, intent.GetServerFullyQualifiedName(intents.Namespace))
			}

			return res
		})
	if err != nil {
		return err
	}
	return nil
}

func (p *ServiceWatcher) Register(mgr manager.Manager) error {
	watcher, err := controller.New("otterize-service-watcher", mgr, controller.Options{
		Reconciler:   p,
		RecoverPanic: lo.ToPtr(true),
	})
	if err != nil {
		return fmt.Errorf("unable to set up service controller: %p", err)
	}

	if err = watcher.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return fmt.Errorf("unable to watch Services: %p", err)
	}

	return nil
}
