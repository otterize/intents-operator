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
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
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

type PostgresConfValidatorV2alpha1 struct {
	client.Client
}

func (v *PostgresConfValidatorV2alpha1) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&otterizev2alpha1.PostgreSQLServerConfig{}).
		WithValidator(v).
		Complete()
}

func NewPostgresConfValidatorV2alpha1(c client.Client) *PostgresConfValidatorV2alpha1 {
	return &PostgresConfValidatorV2alpha1{
		Client: c,
	}
}

//+kubebuilder:webhook:matchPolicy=Exact,path=/validate-k8s-otterize-com-v2alpha1-postgresqlserverconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=k8s.otterize.com,resources=postgresqlserverconfigs,verbs=create;update,versions=v2alpha1,name=postgresqlserverconfigv2alpha1.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &PostgresConfValidatorV2alpha1{}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *PostgresConfValidatorV2alpha1) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *PostgresConfValidatorV2alpha1) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	allErrs := field.ErrorList{}
	pgServerConf := obj.(*otterizev2alpha1.PostgreSQLServerConfig)

	err := validateNoDuplicateForCreate(ctx, v.Client, pgServerConf.Name)
	if err != nil {
		var fieldErr *field.Error
		if goerrors.As(err, &fieldErr) {
			allErrs = append(allErrs, fieldErr)
			gvk := pgServerConf.GroupVersionKind()
			return nil, k8serrors.NewInvalid(
				schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
				pgServerConf.Name, allErrs)
		}

		return nil, errors.Wrap(err)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (v *PostgresConfValidatorV2alpha1) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	allErrs := field.ErrorList{}
	pgServerConf := newObj.(*otterizev2alpha1.PostgreSQLServerConfig)

	err := validateNoDuplicateForUpdate(ctx, v.Client, DatabaseServerConfig{
		Name:      pgServerConf.Name,
		Namespace: pgServerConf.Namespace,
		Type:      Postgres,
	})

	if err != nil {
		var fieldErr *field.Error
		if goerrors.As(err, &fieldErr) {
			allErrs = append(allErrs, fieldErr)
			gvk := pgServerConf.GroupVersionKind()
			return nil, k8serrors.NewInvalid(
				schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
				pgServerConf.Name, allErrs)
		}

		return nil, errors.Wrap(err)
	}
	return nil, nil
}
