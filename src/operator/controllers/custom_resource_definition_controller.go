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
	"fmt"
	"github.com/otterize/intents-operator/src/shared/filters"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// CustomResourceDefinitionsReconciler reconciles webhook configurations
type CustomResourceDefinitionsReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	certPem []byte
}

//+kubebuilder:rbac:groups="apiextensions.k8s.io",resources=customresourcedefinitions,verbs=get;list;watch;update;create;patch

func NewCustomResourceDefinitionsReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	certPem []byte,
) *CustomResourceDefinitionsReconciler {

	return &CustomResourceDefinitionsReconciler{
		Client:  client,
		Scheme:  scheme,
		certPem: certPem,
	}
}

func (r *CustomResourceDefinitionsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logrus.Infof("Reconciling due to CustomResourceDefinition change: %s", req.Name)

	// Fetch the validating webhook configuration object
	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := r.Get(ctx, req.NamespacedName, crd); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Set the new CA bundle for the custom resource definition
	resourceCopy := crd.DeepCopy()
	if resourceCopy.Spec.Conversion == nil || resourceCopy.Spec.Conversion.Webhook == nil || resourceCopy.Spec.Conversion.Webhook.ClientConfig == nil {
		return ctrl.Result{}, fmt.Errorf("CRD does not contain a proper conversion webhook definition")
	}
	resourceCopy.Spec.Conversion.Webhook.ClientConfig.CABundle = r.certPem

	if err := r.Patch(ctx, resourceCopy, client.MergeFrom(crd)); err != nil {
		logrus.WithError(err).Errorf("Failed to patch CustomResourceDefinition %s", crd.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CustomResourceDefinitionsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiextensionsv1.CustomResourceDefinition{}).
		WithEventFilter(filters.FilterByOtterizeLabelPredicate).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}
