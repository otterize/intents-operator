package webhook_traffic

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type WebhookTrafficReconciler interface {
	SetupWithManager(mgr ctrl.Manager) error
	InjectRecorder(recorder record.EventRecorder)
	Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
}

type WebhookTrafficReconcilerManager struct {
	client.Client
	injectablerecorder.InjectableRecorder
	handler *NetworkPolicyHandler

	reconcilers []WebhookTrafficReconciler
}

func NewWebhookTrafficReconcilerManager(client client.Client, handler *NetworkPolicyHandler) *WebhookTrafficReconcilerManager {
	return &WebhookTrafficReconcilerManager{
		Client:  client,
		handler: handler,
		reconcilers: []WebhookTrafficReconciler{
			NewMutatingWebhookReconciler(client, handler),
			NewValidatingWebhookReconciler(client, handler),
			NewCustomResourceDefinitionReconciler(client, handler),
			NewEndpointReconciler(client, handler),
			NewServicesReconciler(client, handler),
			NewNetworkPoliciesReconciler(client, handler),
		},
	}
}

func (r *WebhookTrafficReconcilerManager) SetupWithManager(mgr ctrl.Manager) error {
	recorder := mgr.GetEventRecorderFor("intents-operator")
	r.InjectRecorder(recorder)
	r.handler.InjectRecorder(recorder)

	for _, reconciler := range r.reconcilers {
		if err := reconciler.SetupWithManager(mgr); err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (r *WebhookTrafficReconcilerManager) InjectRecorder(recorder record.EventRecorder) {
	r.Recorder = recorder
}
