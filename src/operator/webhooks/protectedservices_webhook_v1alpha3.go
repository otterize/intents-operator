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
	"github.com/asaskevich/govalidator"
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
	"strings"
)

type ProtectedServiceValidatorV1alpha3 struct {
	client.Client
}

func (v *ProtectedServiceValidatorV1alpha3) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&otterizev1alpha3.ProtectedService{}).
		WithValidator(v).
		Complete()
}

func NewProtectedServiceValidatorV1alpha3(c client.Client) *ProtectedServiceValidatorV1alpha3 {
	return &ProtectedServiceValidatorV1alpha3{
		Client: c,
	}
}

//+kubebuilder:webhook:path=/validate-k8s-otterize-com-v1alpha3-protectedservice,mutating=false,failurePolicy=fail,sideEffects=None,groups=k8s.otterize.com,resources=protectedservice,verbs=create;update,versions=v1alpha3,name=protectedservicev1alpha3.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &ProtectedServiceValidatorV1alpha3{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *ProtectedServiceValidatorV1alpha3) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList
	protectedService := obj.(*otterizev1alpha3.ProtectedService)

	protectedServicesList := &otterizev1alpha3.ProtectedServiceList{}
	if err := v.List(ctx, protectedServicesList, &client.ListOptions{Namespace: protectedService.Namespace}); err != nil {
		return nil, errors.Wrap(err)
	}

	if err := v.validateNoDuplicateClients(protectedService, protectedServicesList); err != nil {
		allErrs = append(allErrs, err)
	}

	if err := v.validateSpec(protectedService); err != nil {
		allErrs = append(allErrs, err)
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	gvk := protectedService.GroupVersionKind()
	return nil, k8serrors.NewInvalid(
		schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
		protectedService.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (v *ProtectedServiceValidatorV1alpha3) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList
	protectedService := newObj.(*otterizev1alpha3.ProtectedService)

	protectedServicesList := &otterizev1alpha3.ProtectedServiceList{}
	if err := v.List(ctx, protectedServicesList, &client.ListOptions{Namespace: protectedService.Namespace}); err != nil {
		return nil, errors.Wrap(err)
	}

	if err := v.validateNoDuplicateClients(protectedService, protectedServicesList); err != nil {
		allErrs = append(allErrs, err)
	}

	if err := v.validateSpec(protectedService); err != nil {
		allErrs = append(allErrs, err)
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	gvk := protectedService.GroupVersionKind()
	return nil, k8serrors.NewInvalid(
		schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
		protectedService.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *ProtectedServiceValidatorV1alpha3) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *ProtectedServiceValidatorV1alpha3) validateNoDuplicateClients(
	protectedService *otterizev1alpha3.ProtectedService, protectedServicesList *otterizev1alpha3.ProtectedServiceList) *field.Error {

	protectedServiceName := protectedService.Spec.Name
	for _, protectedServiceFromList := range protectedServicesList.Items {
		// Deny admission if intents already exist for this client, and it's not the same object being updated
		if protectedServiceFromList.Spec.Name == protectedServiceName && protectedServiceFromList.Name != protectedService.Spec.Name {
			return &field.Error{
				Type:     field.ErrorTypeDuplicate,
				Field:    "name",
				BadValue: protectedServiceName,
				Detail: fmt.Sprintf(
					"Protected service for service %s already exist in resource %s", protectedServiceName, protectedServiceFromList.Name),
			}
		}
	}
	return nil
}

// validateSpec
func (v *ProtectedServiceValidatorV1alpha3) validateSpec(protectedService *otterizev1alpha3.ProtectedService) *field.Error {
	serviceName := strings.ReplaceAll(protectedService.Spec.Name, "-", "")
	serviceName = strings.ReplaceAll(serviceName, "_", "")
	// Validate Service Name contains only lowercase alphanumeric characters
	// Service name should be a valid RFC 1123 subdomain name
	// It's a namespaced resource, we do not expect resources in other namespaces
	if !govalidator.IsAlphanumeric(serviceName) || !govalidator.IsLowerCase(serviceName) {
		message := fmt.Sprintf("Invalid Name: %s. Service name must contain only lowercase alphanumeric characters, '-' or '_'", protectedService.Spec.Name)
		return &field.Error{
			Type:   field.ErrorTypeForbidden,
			Field:  "Name",
			Detail: message,
		}
	}

	return nil
}
