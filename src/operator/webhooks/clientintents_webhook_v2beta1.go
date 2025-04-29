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
	otterizev2beta1 "github.com/otterize/intents-operator/src/operator/api/v2beta1"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/enforcement"
	"github.com/spf13/viper"
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

type IntentsValidatorV2beta1 struct {
	client.Client
}

func (v *IntentsValidatorV2beta1) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&otterizev2beta1.ClientIntents{}).
		WithValidator(v).
		Complete()
}

func NewIntentsValidatorV2beta1(c client.Client) *IntentsValidatorV2beta1 {
	return &IntentsValidatorV2beta1{
		Client: c,
	}
}

//+kubebuilder:webhook:matchPolicy=Exact,path=/validate-k8s-otterize-com-v2beta1-clientintents,mutating=false,failurePolicy=fail,sideEffects=None,groups=k8s.otterize.com,resources=clientintents,verbs=create;update,versions=v2beta1,name=clientintentsv2beta1.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &IntentsValidatorV2beta1{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *IntentsValidatorV2beta1) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList
	intentsObj := obj.(*otterizev2beta1.ClientIntents)
	intentsList := &otterizev2beta1.ClientIntentsList{}
	if err := v.List(ctx, intentsList, &client.ListOptions{Namespace: intentsObj.Namespace}); err != nil {
		return nil, errors.Wrap(err)
	}

	if viper.GetBool(enforcement.StrictModeIntentsKey) {
		if err := v.enforceIntentsAbideStrictMode(intentsObj); err != nil {
			allErrs = append(allErrs, err)
		}
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
func (v *IntentsValidatorV2beta1) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList
	intentsObj := newObj.(*otterizev2beta1.ClientIntents)
	intentsList := &otterizev2beta1.ClientIntentsList{}
	if err := v.List(ctx, intentsList, &client.ListOptions{Namespace: intentsObj.Namespace}); err != nil {
		return nil, errors.Wrap(err)
	}

	if viper.GetBool(enforcement.StrictModeIntentsKey) {
		if err := v.enforceIntentsAbideStrictMode(intentsObj); err != nil {
			allErrs = append(allErrs, err)
		}
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
func (v *IntentsValidatorV2beta1) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *IntentsValidatorV2beta1) validateNoDuplicateClients(
	intentsObj *otterizev2beta1.ClientIntents,
	intentsList *otterizev2beta1.ClientIntentsList) *field.Error {

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
func (v *IntentsValidatorV2beta1) validateKubernetesKind(kind string) *field.Error {
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
func (v *IntentsValidatorV2beta1) validateSpec(intents *otterizev2beta1.ClientIntents) *field.Error {
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

	for _, target := range intents.GetTargetList() {
		err := v.validateIntentTarget(target)
		if err != nil {
			return err
		}
	}
	return nil
}

// Validate intent target
func (v *IntentsValidatorV2beta1) validateIntentTarget(intentTarget otterizev2beta1.Target) *field.Error {
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
	if err := v.validateSQLTarget(intentTarget.SQL); err != nil {
		return err
	}

	if err := v.validateAWSTarget(intentTarget.AWS); err != nil {
		return err
	}

	if err := v.validateGCPTarget(intentTarget.GCP); err != nil {
		return err
	}

	if err := v.validateAzureTarget(intentTarget.Azure); err != nil {
		return err
	}

	if err := v.validateInternetTarget(intentTarget.Internet); err != nil {
		return err
	}

	return nil
}

// Validate that one and only one of the target's fields is set. We do it by marshal the target to json and check there's only one key
func (v *IntentsValidatorV2beta1) validateOnlyOneTargetFieldSet(target otterizev2beta1.Target) *field.Error {
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
func (v *IntentsValidatorV2beta1) validateKubernetesTarget(kubernetesTarget *otterizev2beta1.KubernetesTarget) *field.Error {
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
			Detail: "invalid intent format, field name is required",
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
func (v *IntentsValidatorV2beta1) validateServiceTarget(serviceTarget *otterizev2beta1.ServiceTarget) *field.Error {
	if serviceTarget == nil {
		return nil
	}
	if serviceTarget.Name == "" {
		return &field.Error{
			Type:   field.ErrorTypeRequired,
			Field:  "name",
			Detail: "invalid intent format, field name is required",
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
func (v *IntentsValidatorV2beta1) validateHTTP(httpTarget otterizev2beta1.HTTPTarget) *field.Error {
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
func (v *IntentsValidatorV2beta1) validateKafkaTarget(kafkaTarget *otterizev2beta1.KafkaTarget) *field.Error {
	if kafkaTarget == nil {
		return nil
	}
	if kafkaTarget.Name == "" {
		return &field.Error{
			Type:   field.ErrorTypeRequired,
			Field:  "name",
			Detail: "invalid intent format, field name is required",
		}
	}
	return nil
}

// validate SQLTarget
func (v *IntentsValidatorV2beta1) validateSQLTarget(sqlTarget *otterizev2beta1.SQLTarget) *field.Error {
	if sqlTarget == nil {
		return nil
	}
	if sqlTarget.Name == "" {
		return &field.Error{
			Type:   field.ErrorTypeRequired,
			Field:  "name",
			Detail: "invalid intent format, field name is required",
		}
	}
	return nil

}

// validate AWSTarget
func (v *IntentsValidatorV2beta1) validateAWSTarget(awsTarget *otterizev2beta1.AWSTarget) *field.Error {
	if awsTarget == nil {
		return nil
	}
	if awsTarget.ARN == "" {
		return &field.Error{
			Type:   field.ErrorTypeRequired,
			Field:  "ARN",
			Detail: "invalid intent format, field ARN is required",
		}
	}
	return nil

}

// validate GCPTarget
func (v *IntentsValidatorV2beta1) validateGCPTarget(gcpTarget *otterizev2beta1.GCPTarget) *field.Error {
	if gcpTarget == nil {
		return nil
	}
	if gcpTarget.Resource == "" {
		return &field.Error{
			Type:   field.ErrorTypeRequired,
			Field:  "resource",
			Detail: "invalid intent format, field resource is required",
		}
	}
	return nil

}

// validate AzureTarget
func (v *IntentsValidatorV2beta1) validateAzureTarget(azureTarget *otterizev2beta1.AzureTarget) *field.Error {
	if azureTarget == nil {
		return nil
	}
	if azureTarget.Scope == "" {
		return &field.Error{
			Type:   field.ErrorTypeRequired,
			Field:  "scope",
			Detail: "invalid intent format, field scope is required",
		}
	}
	// check that at least one of the optional fields is set
	if len(azureTarget.Actions) == 0 && len(azureTarget.DataActions) == 0 && len(azureTarget.Roles) == 0 && azureTarget.KeyVaultPolicy == nil {
		return &field.Error{
			Type:   field.ErrorTypeRequired,
			Field:  "actions",
			Detail: "invalid intent format, at least one of [actions, dataActions, roles, keyVaultPolicy] must be set",
		}
	}

	// check that that if intents uses actions/dataActions then roles must be empty (and vice versa)
	if (len(azureTarget.Actions) > 0 || len(azureTarget.DataActions) > 0) && len(azureTarget.Roles) > 0 {
		return &field.Error{
			Type:   field.ErrorTypeRequired,
			Field:  "roles",
			Detail: "invalid intent format, if actions or dataActions are set, roles must be empty",
		}
	}

	return nil

}

// validate internet target
func (v *IntentsValidatorV2beta1) validateInternetTarget(internetTarget *otterizev2beta1.Internet) *field.Error {
	if internetTarget == nil {
		return nil
	}
	hasIPs := len(internetTarget.Ips) > 0
	hasDNS := len(internetTarget.Domains) > 0
	if !hasIPs && !hasDNS {
		return &field.Error{
			Type:   field.ErrorTypeRequired,
			Field:  "ips",
			Detail: fmt.Sprintf("invalid target format. type %s must contain ips or domanin names", otterizev2beta1.IntentTypeInternet),
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
		if err != nil && !strings.HasPrefix(dns, "*") {
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
				Detail: fmt.Sprintf("invalid target format. type %s must contain ips", otterizev2beta1.IntentTypeInternet),
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

func (v *IntentsValidatorV2beta1) enforceIntentsAbideStrictMode(intents *otterizev2beta1.ClientIntents) *field.Error {
	for _, target := range intents.GetTargetList() {
		intentType := target.GetIntentType()
		switch intentType {
		case otterizev2beta1.IntentTypeInternet:
			if hasWildcardDomain(target.Internet.Domains) {
				return &field.Error{
					Type:   field.ErrorTypeForbidden,
					Field:  "domains",
					Detail: fmt.Sprintf("invalid target format. type %s must not contain wildcard domains while in strict mode", intentType),
				}
			}
		case otterizev2beta1.IntentTypeHTTP:
			if nonServiceTarget(target) {
				return &field.Error{
					Type:   field.ErrorTypeForbidden,
					Field:  "service",
					Detail: fmt.Sprintf("invalid target format. type %s must not contain service without port while in strict mode", intentType),
				}
			}
		default:
			continue
		}
	}
	return nil
}
