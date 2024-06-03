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
	"encoding/json"
	"fmt"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/errors"
	"golang.org/x/net/idna"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"net/netip"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"strings"
)

type IntentsValidatorV2alpha1 struct {
	client.Client
}

func (v *IntentsValidatorV2alpha1) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&otterizev2alpha1.ClientIntents{}).
		WithValidator(v).
		Complete()
}

func NewIntentsValidatorV2alpha1(c client.Client) *IntentsValidatorV2alpha1 {
	return &IntentsValidatorV2alpha1{
		Client: c,
	}
}

//+kubebuilder:webhook:path=/validate-k8s-otterize-com-v2alpha1-clientintents,mutating=false,failurePolicy=fail,sideEffects=None,groups=k8s.otterize.com,resources=clientintents,verbs=create;update,versions=v2alpha1,name=clientintentsv2alpha1.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &IntentsValidatorV2alpha1{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *IntentsValidatorV2alpha1) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList
	intentsObj := obj.(*otterizev2alpha1.ClientIntents)
	intentsList := &otterizev2alpha1.ClientIntentsList{}
	if err := v.List(ctx, intentsList, &client.ListOptions{Namespace: intentsObj.Namespace}); err != nil {
		return nil, errors.Wrap(err)
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
	return nil, k8serrors.NewInvalid(
		schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
		intentsObj.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (v *IntentsValidatorV2alpha1) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList
	intentsObj := newObj.(*otterizev2alpha1.ClientIntents)
	intentsList := &otterizev2alpha1.ClientIntentsList{}
	if err := v.List(ctx, intentsList, &client.ListOptions{Namespace: intentsObj.Namespace}); err != nil {
		return nil, errors.Wrap(err)
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
	return nil, k8serrors.NewInvalid(
		schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
		intentsObj.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *IntentsValidatorV2alpha1) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *IntentsValidatorV2alpha1) validateNoDuplicateClients(
	intentsObj *otterizev2alpha1.ClientIntents,
	intentsList *otterizev2alpha1.ClientIntentsList) *field.Error {

	desiredClientName := intentsObj.GetWorkloadName()
	for _, existingIntent := range intentsList.Items {
		// Deny admission if intents already exist for this client, and it's not the same object being updated
		if existingIntent.GetWorkloadName() == desiredClientName && existingIntent.Name != intentsObj.Name && existingIntent.GetClientKind() == intentsObj.GetClientKind() {
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

// validate kubernetes kind
func (v *IntentsValidatorV2alpha1) validateKubernetesKind(kind string) *field.Error {
	if kind != "" && strings.ToUpper(string(kind[0])) != string(kind[0]) {
		return &field.Error{
			Type:   field.ErrorTypeInvalid,
			Field:  "kind",
			Detail: "Kubernetes Kinds must start with an uppercase letter",
		}
	}
	return nil
}

// validateSpec
func (v *IntentsValidatorV2alpha1) validateSpec(intents *otterizev2alpha1.ClientIntents) *field.Error {
	// validate that if kind is specified, it starts with an uppercase letter
	if err := v.validateKubernetesKind(intents.Spec.Workload.Kind); err != nil {
		return err
	}
	if strings.Contains(intents.Spec.Workload.Name, ".") {
		return &field.Error{
			Type:   field.ErrorTypeForbidden,
			Field:  "workload.name",
			Detail: "Workload name should not contain '.' character",
		}
	}

	for _, target := range intents.GetCallsList() {
		err := v.validateIntentTarget(target)
		if err != nil {
			return err
		}
	}
	return nil
}

// Validate intent target
func (v *IntentsValidatorV2alpha1) validateIntentTarget(intentTarget otterizev2alpha1.Target) *field.Error {
	err := v.validateOnlyOneTargetFieldSet(intentTarget)
	if err != nil {
		return err
	}
	if err := v.validateKubernetesTarget(intentTarget.Kubernetes); err != nil {
		return err
	}

	if err := v.validateServiceTarget(intentTarget.Service); err != nil {
		return err
	}

	if err := v.validateKafkaTarget(intentTarget.Kafka); err != nil {
		return err
	}

	if err := v.validateInternetTarget(intentTarget.Internet); err != nil {
		return err
	}

	return nil
}

// Validate that one and only one of the target's fields is set. Wwe do it by marshal the target to json and check there's only one key
func (v *IntentsValidatorV2alpha1) validateOnlyOneTargetFieldSet(target otterizev2alpha1.Target) *field.Error {
	// Marshal the target to json:
	jsonString, err := json.Marshal(target)
	if err != nil {
		return &field.Error{
			Type:   field.ErrorTypeInvalid,
			Field:  "target",
			Detail: "could not marshal target to json",
		}
	}
	// Unmarshal the json to a map
	var targetMap map[string]interface{}
	err = json.Unmarshal(jsonString, &targetMap)
	if err != nil {
		return &field.Error{
			Type:   field.ErrorTypeInvalid,
			Field:  "target",
			Detail: "could not unmarshal target json",
		}
	}
	// Check that the map has only one key
	if len(targetMap) != 1 {
		return &field.Error{
			Type:   field.ErrorTypeInvalid,
			Field:  "target",
			Detail: "each target must have exactly one field set",
		}
	}
	return nil
}

// validate KubernetesTarget
func (v *IntentsValidatorV2alpha1) validateKubernetesTarget(kubernetesTarget *otterizev2alpha1.KubernetesTarget) *field.Error {
	if kubernetesTarget == nil {
		return nil
	}
	if err := v.validateKubernetesKind(kubernetesTarget.Kind); err != nil {
		return err
	}
	if kubernetesTarget.Name == "" {
		return &field.Error{
			Type:   field.ErrorTypeRequired,
			Field:  "name",
			Detail: "name is required",
		}
	}
	if strings.Count(kubernetesTarget.Name, ".") > 1 {
		return &field.Error{
			Type:   field.ErrorTypeForbidden,
			Field:  "Table",
			Detail: "Target server name should not contain more than one '.' character",
		}
	}

	for _, httpTarget := range kubernetesTarget.HTTP {
		if err := v.validateHTTP(httpTarget); err != nil {
			return err
		}
	}
	return nil
}

// validate ServiceTarget
func (v *IntentsValidatorV2alpha1) validateServiceTarget(serviceTarget *otterizev2alpha1.ServiceTarget) *field.Error {
	if serviceTarget == nil {
		return nil
	}
	if serviceTarget.Name == "" {
		return &field.Error{
			Type:   field.ErrorTypeRequired,
			Field:  "name",
			Detail: "name is required",
		}
	}
	if strings.Count(serviceTarget.Name, ".") > 1 {
		return &field.Error{
			Type:   field.ErrorTypeForbidden,
			Field:  "Table",
			Detail: "Target server name should not contain more than one '.' character",
		}
	}
	for _, httpTarget := range serviceTarget.HTTP {
		if err := v.validateHTTP(httpTarget); err != nil {
			return err
		}
	}
	return nil
}

// validate HTTPTarget
func (v *IntentsValidatorV2alpha1) validateHTTP(httpTarget otterizev2alpha1.HTTPTarget) *field.Error {
	if httpTarget.Path == "" {
		return &field.Error{
			Type:   field.ErrorTypeRequired,
			Field:  "path",
			Detail: "path is required",
		}
	}
	return nil
}

// validate KafkaTarget
func (v *IntentsValidatorV2alpha1) validateKafkaTarget(kafkaTarget *otterizev2alpha1.KafkaTarget) *field.Error {
	if kafkaTarget == nil {
		return nil
	}
	if kafkaTarget.Name == "" {
		return &field.Error{
			Type:   field.ErrorTypeRequired,
			Field:  "name",
			Detail: "name is required",
		}
	}
	return nil
}

// validate internet target
func (v *IntentsValidatorV2alpha1) validateInternetTarget(internetTarget *otterizev2alpha1.Internet) *field.Error {
	if internetTarget == nil {
		return nil
	}
	hasIPs := len(internetTarget.Ips) > 0
	hasDNS := len(internetTarget.Domains) > 0
	if !hasIPs && !hasDNS {
		return &field.Error{
			Type:   field.ErrorTypeRequired,
			Field:  "ips",
			Detail: fmt.Sprintf("invalid target format. type %s must contain ips or domanin names", otterizev2alpha1.IntentTypeInternet),
		}
	}
	if len(internetTarget.Ips) == 0 && len(internetTarget.Domains) == 0 {
		return &field.Error{
			Type:   field.ErrorTypeRequired,
			Field:  "ips",
			Detail: "ips or domains are required",
		}
	}
	for _, dns := range internetTarget.Domains {
		_, err := idna.Lookup.ToASCII(dns)
		if err != nil {
			return &field.Error{
				Type:     field.ErrorTypeInvalid,
				Field:    "domains",
				Detail:   "should be valid DNS name",
				BadValue: dns,
			}
		}
	}
	for _, ip := range internetTarget.Ips {
		if ip == "" {
			return &field.Error{
				Type:   field.ErrorTypeRequired,
				Field:  "ips",
				Detail: fmt.Sprintf("invalid target format. type %s must contain ips", otterizev2alpha1.IntentTypeInternet),
			}
		}
		var err error
		if strings.Contains(ip, "/") {
			_, err = netip.ParsePrefix(ip)
		} else {
			_, err = netip.ParseAddr(ip)
		}
		if err != nil {
			return &field.Error{
				Type:     field.ErrorTypeInvalid,
				Field:    "ips",
				Detail:   "should be value IP address or CIDR",
				BadValue: ip,
			}
		}
	}
	return nil
}
