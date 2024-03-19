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
	"fmt"
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

type PostgresConfValidator struct {
	client.Client
}

func (v *PostgresConfValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&otterizev1alpha3.PostgreSQLServerConfig{}).
		WithValidator(v).
		Complete()
}

func NewPostgresConfValidator(c client.Client) *PostgresConfValidator {
	return &PostgresConfValidator{
		Client: c,
	}
}

//+kubebuilder:webhook:path=/validate-k8s-otterize-com-v1alpha3-postgresqlserverconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=k8s.otterize.com,resources=postgresqlserverconfig,verbs=create;update,versions=v1alpha3,name=postgresqlserverconfig.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &PostgresConfValidator{}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *PostgresConfValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *PostgresConfValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	// We validate no other PGServerConf with the same name exists in the cluster
	// It's a little hacky since it is a namespace scoped resource but we enforce name uniqueness across the cluster
	allErrs := field.ErrorList{}
	pgServerConf := obj.(*otterizev1alpha3.PostgreSQLServerConfig)
	pgServerConfList := otterizev1alpha3.PostgreSQLServerConfigList{}
	if err := v.List(ctx, &pgServerConfList); err != nil {
		return nil, errors.Wrap(err)
	}

	if err := v.ValidateNoDuplicateConfNames(pgServerConf.Name, pgServerConfList.Items); err != nil {
		allErrs = append(allErrs, err)
		gvk := pgServerConf.GroupVersionKind()
		return nil, k8serrors.NewInvalid(
			schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
			pgServerConf.Name, allErrs)
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (v *PostgresConfValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	allErrs := field.ErrorList{}
	pgServerConf := newObj.(*otterizev1alpha3.PostgreSQLServerConfig)
	pgServerConfList := otterizev1alpha3.PostgreSQLServerConfigList{}
	if err := v.List(ctx, &pgServerConfList); err != nil {
		return nil, errors.Wrap(err)
	}

	if err := v.ValidateNoDuplicateConfNames(pgServerConf.Name, pgServerConfList.Items); err != nil {
		allErrs = append(allErrs, err)
		gvk := pgServerConf.GroupVersionKind()
		return nil, k8serrors.NewInvalid(
			schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
			pgServerConf.Name, allErrs)
	}

	return nil, nil
}

func (v *PostgresConfValidator) ValidateNoDuplicateConfNames(
	currName string,
	confList []otterizev1alpha3.PostgreSQLServerConfig) *field.Error {

	for _, conf := range confList {
		if currName == conf.Name {
			return &field.Error{
				Type:     field.ErrorTypeDuplicate,
				Field:    "name",
				BadValue: currName,
				Detail: fmt.Sprintf(
					"Postgres server config already exists with name %s. Existing resource: %s.%s",
					currName, conf.Name, conf.Namespace),
			}
		}
	}
	return nil
}
