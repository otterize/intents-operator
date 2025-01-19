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
	"strings"
)

type ProtectedServiceValidatorV2alpha1 struct {
	client.Client
}

func (v *ProtectedServiceValidatorV2alpha1) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&otterizev2alpha1.ProtectedService{}).
		WithValidator(v).
		Complete()
}

func NewProtectedServiceValidatorV2alpha1(c client.Client) *ProtectedServiceValidatorV2alpha1 {
	return &ProtectedServiceValidatorV2alpha1{
		Client: c,
	}
}

//+kubebuilder:webhook:matchPolicy=Exact,path=/validate-k8s-otterize-com-v2alpha1-protectedservice,mutating=false,failurePolicy=fail,sideEffects=None,groups=k8s.otterize.com,resources=protectedservice,verbs=create;update,versions=v2alpha1,name=protectedservicev2alpha1.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &ProtectedServiceValidatorV2alpha1{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *ProtectedServiceValidatorV2alpha1) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList
	protectedService := obj.(*otterizev2alpha1.ProtectedService)

	protectedServicesList := &otterizev2alpha1.ProtectedServiceList{}
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
func (v *ProtectedServiceValidatorV2alpha1) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList
	protectedService := newObj.(*otterizev2alpha1.ProtectedService)

	protectedServicesList := &otterizev2alpha1.ProtectedServiceList{}
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
func (v *ProtectedServiceValidatorV2alpha1) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *ProtectedServiceValidatorV2alpha1) validateNoDuplicateClients(
	protectedService *otterizev2alpha1.ProtectedService, protectedServicesList *otterizev2alpha1.ProtectedServiceList) *field.Error {

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
func (v *ProtectedServiceValidatorV2alpha1) validateSpec(protectedService *otterizev2alpha1.ProtectedService) *field.Error {
	serviceName := strings.ReplaceAll(protectedService.Spec.Name, "-", "")
	serviceName = strings.ReplaceAll(serviceName, "_", "")
	// Validate Workload Name contains only lowercase alphanumeric characters
	// Workload name should be a valid RFC 1123 subdomain name
	// It's a namespaced resource, we do not expect resources in other namespaces
	if !govalidator.IsAlphanumeric(serviceName) || !govalidator.IsLowerCase(serviceName) {
		message := fmt.Sprintf("Invalid Name: %s. Workload name must contain only lowercase alphanumeric characters, '-' or '_'", protectedService.Spec.Name)
		return &field.Error{
			Type:   field.ErrorTypeForbidden,
			Field:  "Name",
			Detail: message,
		}
	}

	return nil
}
