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

package v1alpha2

import (
	"github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func (in *ClientIntents) SetupWebhookWithManager(mgr ctrl.Manager, validator admission.CustomValidator) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(in).WithValidator(validator).
		Complete()
}

// ConvertTo converts this ClientIntents to the Hub version (v1alpha3).
func (in *ClientIntents) ConvertTo(dstRaw conversion.Hub) error {
	dst := &v1alpha3.ClientIntents{}
	dst.Spec = &v1alpha3.IntentsSpec{}
	dst.ObjectMeta = in.ObjectMeta
	dst.Spec.Service.Name = in.Spec.Service.Name
	dst.Spec.Calls = make([]v1alpha3.Intent, len(in.Spec.Calls))
	for i, call := range in.Spec.Calls {
		dst.Spec.Calls[i].Name = call.Name
		dst.Spec.Calls[i].Type = v1alpha3.IntentType(call.Type)
		dst.Spec.Calls[i].Topics = convertTopicsV1alpha2toV1alpha3(call.Topics)
		dst.Spec.Calls[i].HTTPResources = convertHTTPResourcesV1alpha2toV1alpha3(call.HTTPResources)
		dst.Spec.Calls[i].DatabaseResources = convertDatabaseResourcesV1alpha2toV1alpha3(call.DatabaseResources)
	}
	return dst.ConvertTo(dstRaw)
}

func convertDatabaseResourcesV1alpha2toV1alpha3(srcResources []DatabaseResource) []v1alpha3.DatabaseResource {
	dstResources := make([]v1alpha3.DatabaseResource, len(srcResources))
	for i, resource := range srcResources {
		dstResources[i].Table = resource.Table
		dstResources[i].DatabaseName = resource.DatabaseName
		dstResources[i].Operations = lo.Map(resource.Operations, func(operation DatabaseOperation, _ int) v1alpha3.DatabaseOperation {
			return v1alpha3.DatabaseOperation(operation)
		})
	}
	return dstResources
}

func convertHTTPResourcesV1alpha2toV1alpha3(srcResources []HTTPResource) []v1alpha3.HTTPResource {
	dstResources := make([]v1alpha3.HTTPResource, len(srcResources))
	for i, resource := range srcResources {
		dstResources[i].Path = resource.Path
		dstResources[i].Methods = lo.Map(resource.Methods, func(operation HTTPMethod, _ int) v1alpha3.HTTPMethod { return v1alpha3.HTTPMethod(operation) })
	}
	return dstResources
}

func convertTopicsV1alpha2toV1alpha3(srcTopics []KafkaTopic) []v1alpha3.KafkaTopic {
	dstTopics := make([]v1alpha3.KafkaTopic, len(srcTopics))
	for i, topic := range srcTopics {
		dstTopics[i].Name = topic.Name
		dstTopics[i].Operations = lo.Map(topic.Operations, func(operation KafkaOperation, _ int) v1alpha3.KafkaOperation {
			return v1alpha3.KafkaOperation(operation)
		})
	}
	return dstTopics
}

// ConvertFrom converts the Hub version (v2beta1) to this ClientIntents.
func (in *ClientIntents) ConvertFrom(srcRaw conversion.Hub) error {
	src := &v1alpha3.ClientIntents{}
	if err := src.ConvertFrom(srcRaw); err != nil {
		return errors.Wrap(err)
	}
	in.ObjectMeta = src.ObjectMeta
	if in.Spec == nil {
		in.Spec = &IntentsSpec{}
	}
	in.Spec.Service.Name = src.Spec.Service.Name
	in.Spec.Calls = make([]Intent, len(src.Spec.Calls))
	for i, call := range src.Spec.Calls {
		in.Spec.Calls[i].Name = call.Name
		in.Spec.Calls[i].Type = IntentType(call.Type)
		in.Spec.Calls[i].Topics = convertTopicsV1alpha3toV1alpha2(call.Topics)
		in.Spec.Calls[i].HTTPResources = convertHTTPResourcesV1alpha3toV1alpha2(call.HTTPResources)
		in.Spec.Calls[i].DatabaseResources = convertDatabaseResourcesV1alpha3toV1alpha2(call.DatabaseResources)
	}
	return nil
}

func convertDatabaseResourcesV1alpha3toV1alpha2(srcResources []v1alpha3.DatabaseResource) []DatabaseResource {
	dstResources := make([]DatabaseResource, len(srcResources))
	for i, resource := range srcResources {
		dstResources[i].DatabaseName = resource.DatabaseName
		dstResources[i].Table = resource.Table
		dstResources[i].Operations = lo.Map(resource.Operations, func(operation v1alpha3.DatabaseOperation, _ int) DatabaseOperation {
			return DatabaseOperation(operation)
		})
	}
	return dstResources
}

func convertHTTPResourcesV1alpha3toV1alpha2(srcResources []v1alpha3.HTTPResource) []HTTPResource {
	dstResources := make([]HTTPResource, len(srcResources))
	for i, resource := range srcResources {
		dstResources[i].Path = resource.Path
		dstResources[i].Methods = lo.Map(resource.Methods, func(operation v1alpha3.HTTPMethod, _ int) HTTPMethod { return HTTPMethod(operation) })
	}
	return dstResources
}

func convertTopicsV1alpha3toV1alpha2(srcTopics []v1alpha3.KafkaTopic) []KafkaTopic {
	dstTopics := make([]KafkaTopic, len(srcTopics))
	for i, topic := range srcTopics {
		dstTopics[i].Name = topic.Name
		dstTopics[i].Operations = lo.Map(topic.Operations, func(operation v1alpha3.KafkaOperation, _ int) KafkaOperation { return KafkaOperation(operation) })
	}
	return dstTopics
}
