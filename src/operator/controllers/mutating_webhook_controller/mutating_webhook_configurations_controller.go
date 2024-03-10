/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mutatingwebhookconfiguration

import (
	"bytes"
	"context"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// MutatingWebhookConfigsReconciler reconciles webhook configurations
type MutatingWebhookConfigsReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	certPEM   []byte
	predicate predicate.Predicate
}

//+kubebuilder:rbac:groups="admissionregistration.k8s.io",resources=mutatingwebhookconfigurations,verbs=get;update;patch;list;watch

func NewMutatingWebhookConfigsReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	certPem []byte,
	predicate predicate.Predicate,
) *MutatingWebhookConfigsReconciler {

	return &MutatingWebhookConfigsReconciler{
		Client:    client,
		Scheme:    scheme,
		certPEM:   certPem,
		predicate: predicate,
	}
}

func (r *MutatingWebhookConfigsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logrus.Debugf("Reconciling due to MutatingWebhookConfiguration change: %s", req.Name)

	// Fetch the validating webhook configuration object
	webhookConfig := &admissionregistrationv1.MutatingWebhookConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, webhookConfig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// If all certs match, don't reconcile.
	if lo.EveryBy(webhookConfig.Webhooks, func(item admissionregistrationv1.MutatingWebhook) bool {
		return bytes.Equal(item.ClientConfig.CABundle, r.certPEM)
	}) {
		return ctrl.Result{}, nil
	}

	// Set the new CA bundle for the mutating webhooks
	resourceCopy := webhookConfig.DeepCopy()
	for i := range resourceCopy.Webhooks {
		resourceCopy.Webhooks[i].ClientConfig.CABundle = r.certPEM
	}

	if err := r.Patch(ctx, resourceCopy, client.MergeFrom(webhookConfig)); err != nil {
		return ctrl.Result{}, errors.Errorf("failed to patch MutatingWebhookConfiguration: %w", err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MutatingWebhookConfigsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&admissionregistrationv1.MutatingWebhookConfiguration{}).
		WithEventFilter(r.predicate).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}
