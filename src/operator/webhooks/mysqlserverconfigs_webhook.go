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

package webhooks

import (
	"context"
	goerrors "errors"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type MySQLConfValidator struct {
	client.Client
}

func (v *MySQLConfValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&otterizev1alpha3.MySQLServerConfig{}).
		WithValidator(v).
		Complete()
}

func NewMySQLConfValidator(c client.Client) *MySQLConfValidator {
	return &MySQLConfValidator{
		Client: c,
	}
}

//+kubebuilder:webhook:matchPolicy=Exact,path=/validate-k8s-otterize-com-v1alpha3-mysqlserverconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=k8s.otterize.com,resources=mysqlserverconfigs,verbs=create;update,versions=v1alpha3,name=mysqlserverconfig.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &MySQLConfValidator{}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *MySQLConfValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *MySQLConfValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	allErrs := field.ErrorList{}
	mysqlServerConf := obj.(*otterizev1alpha3.MySQLServerConfig)
	gvk := mysqlServerConf.GroupVersionKind()

	if err := validateCredentialsNotEmptyV1alpha3(mysqlServerConf.Spec.Credentials); err != nil {
		allErrs = append(allErrs, err)
	}

	err := validateNoDuplicateForCreate(ctx, v.Client, mysqlServerConf.Name)
	if fieldErr := (&field.Error{}); goerrors.As(err, &fieldErr) {
		allErrs = append(allErrs, fieldErr)
	} else if err != nil {
		return nil, errors.Wrap(err)
	}

	if len(allErrs) > 0 {
		return nil, k8serrors.NewInvalid(
			schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
			mysqlServerConf.Name, allErrs)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (v *MySQLConfValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	allErrs := field.ErrorList{}
	mysqlServerConf := newObj.(*otterizev1alpha3.MySQLServerConfig)
	gvk := mysqlServerConf.GroupVersionKind()

	if err := validateCredentialsNotEmptyV1alpha3(mysqlServerConf.Spec.Credentials); err != nil {
		allErrs = append(allErrs, err)
	}

	err := validateNoDuplicateForUpdate(ctx, v.Client, DatabaseServerConfig{
		Name:      mysqlServerConf.Name,
		Namespace: mysqlServerConf.Namespace,
		Type:      MySQL,
	})
	if fieldErr := (&field.Error{}); goerrors.As(err, &fieldErr) {
		allErrs = append(allErrs, fieldErr)
	} else if err != nil {
		return nil, errors.Wrap(err)
	}

	if len(allErrs) > 0 {
		return nil, k8serrors.NewInvalid(
			schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
			mysqlServerConf.Name, allErrs)
	}

	return nil, nil
}
