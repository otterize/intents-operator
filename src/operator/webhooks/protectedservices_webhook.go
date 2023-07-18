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
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"strings"
)

type ProtectedServicesValidator struct {
	client.Client
}

func (v *ProtectedServicesValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&otterizev1alpha2.ProtectedServices{}).
		WithValidator(v).
		Complete()
}

func NewProtectedServicesValidator(c client.Client) *ProtectedServicesValidator {
	return &ProtectedServicesValidator{
		Client: c,
	}
}

//+kubebuilder:webhook:path=/validate-k8s-otterize-com-v1alpha2-protectedservices,mutating=false,failurePolicy=fail,sideEffects=None,groups=k8s.otterize.com,resources=protectedservices,verbs=create;update,versions=v1alpha2,name=protectedservices.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &ProtectedServicesValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *ProtectedServicesValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	var allErrs field.ErrorList
	intentsObj := obj.(*otterizev1alpha2.ProtectedServices)

	if err := v.validateSpec(intentsObj); err != nil {
		allErrs = append(allErrs, err)
	}

	if len(allErrs) == 0 {
		return nil
	}

	gvk := intentsObj.GroupVersionKind()
	return errors.NewInvalid(
		schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
		intentsObj.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (v *ProtectedServicesValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	var allErrs field.ErrorList
	protectedServices := newObj.(*otterizev1alpha2.ProtectedServices)

	if err := v.validateSpec(protectedServices); err != nil {
		allErrs = append(allErrs, err)
	}

	if len(allErrs) == 0 {
		return nil
	}

	gvk := protectedServices.GroupVersionKind()
	return errors.NewInvalid(
		schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
		protectedServices.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *ProtectedServicesValidator) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	return nil
}

// validateSpec
func (v *ProtectedServicesValidator) validateSpec(intents *otterizev1alpha2.ProtectedServices) *field.Error {
	for _, service := range intents.Spec.ProtectedServices {
		serviceName := strings.Trim(service.Name, "-_")
		// Validate Service Name contains only lowercase alphanumeric characters
		if !govalidator.IsAlphanumeric(serviceName) {
			message := fmt.Sprintf("Invalid Name: %s. Service name must contain only lowercase alphanumeric characters, '-' or '_'", service.Name)
			return &field.Error{
				Type:   field.ErrorTypeForbidden,
				Field:  "Name",
				Detail: message,
			}
		}
	}

	return nil
}
