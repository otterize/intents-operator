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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"strings"
)

type IntentsValidatorV1alpha3 struct {
	client.Client
}

func (v *IntentsValidatorV1alpha3) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&otterizev1alpha3.ClientIntents{}).
		WithValidator(v).
		Complete()
}

func NewIntentsValidatorV1alpha3(c client.Client) *IntentsValidatorV1alpha3 {
	return &IntentsValidatorV1alpha3{
		Client: c,
	}
}

//+kubebuilder:webhook:path=/validate-k8s-otterize-com-v1alpha3-clientintents,mutating=false,failurePolicy=fail,sideEffects=None,groups=k8s.otterize.com,resources=clientintents,verbs=create;update,versions=v1alpha3,name=clientintentsv1alpha3.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &IntentsValidatorV1alpha3{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *IntentsValidatorV1alpha3) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList
	intentsObj := obj.(*otterizev1alpha3.ClientIntents)
	intentsList := &otterizev1alpha3.ClientIntentsList{}
	if err := v.List(ctx, intentsList, &client.ListOptions{Namespace: intentsObj.Namespace}); err != nil {
		return nil, err
	}
	if err := v.validateNoDuplicateClients(intentsObj, intentsList); err != nil {
		allErrs = append(allErrs, err)
	}

	if err := v.validateSpec(intentsObj); err != nil {
		allErrs = append(allErrs, err)
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	gvk := intentsObj.GroupVersionKind()
	return nil, errors.NewInvalid(
		schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
		intentsObj.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (v *IntentsValidatorV1alpha3) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList
	intentsObj := newObj.(*otterizev1alpha3.ClientIntents)
	intentsList := &otterizev1alpha3.ClientIntentsList{}
	if err := v.List(ctx, intentsList, &client.ListOptions{Namespace: intentsObj.Namespace}); err != nil {
		return nil, err
	}
	if err := v.validateNoDuplicateClients(intentsObj, intentsList); err != nil {
		allErrs = append(allErrs, err)
	}

	if err := v.validateSpec(intentsObj); err != nil {
		allErrs = append(allErrs, err)
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	gvk := intentsObj.GroupVersionKind()
	return nil, errors.NewInvalid(
		schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
		intentsObj.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *IntentsValidatorV1alpha3) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *IntentsValidatorV1alpha3) validateNoDuplicateClients(
	intentsObj *otterizev1alpha3.ClientIntents,
	intentsList *otterizev1alpha3.ClientIntentsList) *field.Error {

	desiredClientName := intentsObj.GetServiceName()
	for _, existingIntent := range intentsList.Items {
		// Deny admission if intents already exist for this client, and it's not the same object being updated
		if existingIntent.GetServiceName() == desiredClientName && existingIntent.Name != intentsObj.Name {
			return &field.Error{
				Type:     field.ErrorTypeDuplicate,
				Field:    "name",
				BadValue: desiredClientName,
				Detail: fmt.Sprintf(
					"Intents for client %s already exist in resource %s", desiredClientName, existingIntent.Name),
			}
		}
	}
	return nil
}

// validateSpec
func (v *IntentsValidatorV1alpha3) validateSpec(intents *otterizev1alpha3.ClientIntents) *field.Error {
	for _, intent := range intents.GetCallsList() {
		if intent.Type == otterizev1alpha3.IntentTypeHTTP {
			if intent.Topics != nil {
				return &field.Error{
					Type:   field.ErrorTypeForbidden,
					Field:  "topics",
					Detail: fmt.Sprintf("invalid intent format. type %s cannot contain kafka topics", otterizev1alpha3.IntentTypeHTTP),
				}
			}
		}
		if strings.Count(intent.Name, ".") > 1 {
			return &field.Error{
				Type:   field.ErrorTypeForbidden,
				Field:  "Name",
				Detail: "Target server name should not contain more than one '.' character",
			}
		}
	}
	return nil
}
