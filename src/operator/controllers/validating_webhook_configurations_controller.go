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

package controllers

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/filters"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// ValidatingWebhookConfigsReconciler reconciles webhook configurations
type ValidatingWebhookConfigsReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	certPEM []byte
}

//+kubebuilder:rbac:groups="admissionregistration.k8s.io",resources=validatingwebhookconfigurations,verbs=get;update;patch;list

func NewValidatingWebhookConfigsReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	certPem []byte,
) *ValidatingWebhookConfigsReconciler {

	return &ValidatingWebhookConfigsReconciler{
		Client:  client,
		Scheme:  scheme,
		certPEM: certPem,
	}
}

func (r *ValidatingWebhookConfigsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logrus.Debugf("Reconciling due to ValidatingWebhookConfiguration change: %s", req.Name)

	// Fetch the validating webhook configuration object
	webhookConfig := &admissionregistrationv1.ValidatingWebhookConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, webhookConfig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Set the new CA bundle for the validating webhooks
	resourceCopy := webhookConfig.DeepCopy()
	for i := range resourceCopy.Webhooks {
		resourceCopy.Webhooks[i].ClientConfig.CABundle = r.certPEM
	}

	if err := r.Patch(ctx, resourceCopy, client.MergeFrom(webhookConfig)); err != nil {
		return ctrl.Result{}, errors.Errorf("Failed to patch ValidatingWebhookConfiguration: %w", err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ValidatingWebhookConfigsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&admissionregistrationv1.ValidatingWebhookConfiguration{}).
		WithEventFilter(filters.FilterByOtterizeLabelPredicate).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}
