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
	"bytes"
	"context"
	"github.com/otterize/intents-operator/src/operator/otterizecrds"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/filters"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// CustomResourceDefinitionsReconciler reconciles webhook configurations
type CustomResourceDefinitionsReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	certPem   []byte
	namespace string
}

//+kubebuilder:rbac:groups="apiextensions.k8s.io",resources=customresourcedefinitions,verbs=get;list;watch;update;create;patch

func NewCustomResourceDefinitionsReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	certPem []byte,
	namespace string,
) *CustomResourceDefinitionsReconciler {

	return &CustomResourceDefinitionsReconciler{
		Client:    client,
		Scheme:    scheme,
		certPem:   certPem,
		namespace: namespace,
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
	baseCRD, err := otterizecrds.GetCRDDefinitionByName(crd.Name)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}
	resourceCopy := crd.DeepCopy()
	resourceCopy.Spec = baseCRD.Spec
	if resourceCopy.Spec.Conversion == nil || resourceCopy.Spec.Conversion.Webhook == nil || resourceCopy.Spec.Conversion.Webhook.ClientConfig == nil || resourceCopy.Spec.Conversion.Webhook.ClientConfig.Service == nil {
		return ctrl.Result{}, errors.Errorf("CRD does not contain a proper conversion webhook definition")
	}
	if bytes.Equal(crd.Spec.Conversion.Webhook.ClientConfig.CABundle, r.certPem) && crd.Spec.Conversion.Webhook.ClientConfig.Service.Namespace == r.namespace {
		logrus.Infof("CustomResourceDefinition %s already has the correct CA bundle and namespace", resourceCopy.Name)
		return ctrl.Result{}, nil

	}
	resourceCopy.Spec.Conversion.Webhook.ClientConfig.CABundle = r.certPem
	resourceCopy.Spec.Conversion.Webhook.ClientConfig.Service.Namespace = r.namespace

	// Use optimistic locking to avoid using "mergeFrom" with an outdated resource
	if err := r.Patch(ctx, resourceCopy, client.MergeFromWithOptions(crd, client.MergeFromWithOptimisticLock{})); err != nil {
		if k8serrors.IsConflict(err) || k8serrors.IsNotFound(err) || k8serrors.IsForbidden(err) {
			logrus.Debugf("Conflict while updating CustomResourceDefinition: %s", resourceCopy.Name)
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, errors.Errorf("Failed to patch CustomResourceDefinition: %w", err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CustomResourceDefinitionsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logrus.Info("Registered CustomResourceDefinitionsReconciler")
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiextensionsv1.CustomResourceDefinition{}).
		WithEventFilter(filters.PartOfOtterizeLabelPredicate()).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}
