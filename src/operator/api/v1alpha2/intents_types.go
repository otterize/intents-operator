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
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"strings"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	OtterizeAccessLabelPrefix              = "intents.otterize.com/access"
	OtterizeAccessLabelKey                 = "intents.otterize.com/access-%s"
	OtterizeClientLabelKey                 = "intents.otterize.com/client"
	OtterizeServerLabelKey                 = "intents.otterize.com/server"
	OtterizeNamespaceLabelKey              = "intents.otterize.com/namespace-name"
	AllIntentsRemovedAnnotation            = "intents.otterize.com/all-intents-removed"
	OtterizeCreatedForServiceAnnotation    = "intents.otterize.com/created-for-service"
	OtterizeCreatedForIngressAnnotation    = "intents.otterize.com/created-for-ingress"
	OtterizeNetworkPolicyNameTemplate      = "access-to-%s-from-%s"
	OtterizeNetworkPolicy                  = "intents.otterize.com/network-policy"
	OtterizeNetworkPolicyExternalTraffic   = "intents.otterize.com/network-policy-external-traffic"
	NetworkPolicyFinalizerName             = "intents.otterize.com/network-policy-finalizer"
	OtterizeIstioClientAnnotationKey       = "intents.otterize.com/istio-client"
	OtterizeClientServiceAccountAnnotation = "intents.otterize.com/client-intents-service-account"
	OtterizeSharedServiceAccountAnnotation = "intents.otterize.com/shared-service-account"
	OtterizeTargetServerIndexField         = "spec.service.calls.server"
	EndpointsPodNamesIndexField            = "endpointsPodNames"
	IngressServiceNamesIndexField          = "ingressServiceNames"
	NetworkPoliciesByIngressNameIndexField = "networkPoliciesByIngressName"
	MaxOtterizeNameLength                  = 20
	MaxNamespaceLength                     = 20
)

// +kubebuilder:validation:Enum=http;kafka
type IntentType string

const (
	IntentTypeHTTP  IntentType = "http"
	IntentTypeKafka IntentType = "kafka"
)

// +kubebuilder:validation:Enum=all;consume;produce;create;alter;delete;describe;ClusterAction;DescribeConfigs;AlterConfigs;IdempotentWrite
type KafkaOperation string

const (
	KafkaOperationAll             KafkaOperation = "all"
	KafkaOperationConsume         KafkaOperation = "consume"
	KafkaOperationProduce         KafkaOperation = "produce"
	KafkaOperationCreate          KafkaOperation = "create"
	KafkaOperationAlter           KafkaOperation = "alter"
	KafkaOperationDelete          KafkaOperation = "delete"
	KafkaOperationDescribe        KafkaOperation = "describe"
	KafkaOperationClusterAction   KafkaOperation = "ClusterAction"
	KafkaOperationDescribeConfigs KafkaOperation = "DescribeConfigs"
	KafkaOperationAlterConfigs    KafkaOperation = "AlterConfigs"
	KafkaOperationIdempotentWrite KafkaOperation = "IdempotentWrite"
)

// +kubebuilder:validation:Enum=GET;POST;PUT;DELETE;OPTIONS;TRACE;PATCH;CONNECT
type HTTPMethod string

const (
	HTTPMethodGet     HTTPMethod = "GET"
	HTTPMethodPost    HTTPMethod = "POST"
	HTTPMethodPut     HTTPMethod = "PUT"
	HTTPMethodDelete  HTTPMethod = "DELETE"
	HTTPMethodOptions HTTPMethod = "OPTIONS"
	HTTPMethodTrace   HTTPMethod = "TRACE"
	HTTPMethodPatch   HTTPMethod = "PATCH"
	HTTPMethodConnect HTTPMethod = "CONNECT"
)

// IntentsSpec defines the desired state of ClientIntents
type IntentsSpec struct {
	Service Service  `json:"service" yaml:"service"`
	Calls   []Intent `json:"calls" yaml:"calls"`
}

type Service struct {
	Name string `json:"name" yaml:"name"`
}

type Intent struct {
	Name string `json:"name" yaml:"name"`

	//+optional
	Type IntentType `json:"type,omitempty" yaml:"type,omitempty"`

	//+optional
	Topics []KafkaTopic `json:"topics,omitempty" yaml:"topics,omitempty"`

	//+optional
	HTTPResources []HTTPResource `json:"resources"`
}

type HTTPResource struct {
	Path    string       `json:"path"`
	Methods []HTTPMethod `json:"methods" yaml:"methods"`
}

type KafkaTopic struct {
	Name       string           `json:"name" yaml:"name"`
	Operations []KafkaOperation `json:"operations" yaml:"operations"`
}

// IntentsStatus defines the observed state of ClientIntents
type IntentsStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ClientIntents is the Schema for the intents API
type ClientIntents struct {
	metav1.TypeMeta   `json:",inline" yaml:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`

	Spec   *IntentsSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status *IntentsStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ClientIntentsList contains a list of ClientIntents
type ClientIntentsList struct {
	metav1.TypeMeta `json:",inline" yaml:",inline"`
	metav1.ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items           []ClientIntents `json:"items" yaml:"items"`
}

func init() {
	SchemeBuilder.Register(&ClientIntents{}, &ClientIntentsList{})
}

func (in *ClientIntents) GetServiceName() string {
	return in.Spec.Service.Name
}

func (in *ClientIntents) GetCallsList() []Intent {
	return in.Spec.Calls
}

func (in *ClientIntents) GetIntentsLabelMapping(requestNamespace string) map[string]string {
	otterizeAccessLabels := map[string]string{}

	for _, intent := range in.GetCallsList() {
		ns := intent.GetServerNamespace(requestNamespace)
		formattedOtterizeIdentity := GetFormattedOtterizeIdentity(intent.GetServerName(), ns)
		otterizeAccessLabels[fmt.Sprintf(OtterizeAccessLabelKey, formattedOtterizeIdentity)] = "true"
	}

	return otterizeAccessLabels
}

// GetServerNamespace returns target namespace for intent if exists
// or the entire resource's namespace if the specific intent has no target namespace, as it's optional
func (in *Intent) GetServerNamespace(intentsObjNamespace string) string {
	nameWithNamespace := strings.Split(in.Name, ".")
	if len(nameWithNamespace) == 1 {
		return intentsObjNamespace
	}

	// serverName.namespace --> "namespace"
	return nameWithNamespace[1]
}

// GetServerNamespace returns target namespace for intent if exists
// or the entire resource's namespace if the specific intent has no target namespace, as it's optional
func (in *Intent) GetServerName() string {
	nameWithNamespace := strings.Split(in.Name, ".")
	if len(nameWithNamespace) == 1 {
		return in.Name
	}

	return nameWithNamespace[0]
}

func (in *Intent) typeAsGQLType() graphqlclient.IntentType {
	switch in.Type {
	case IntentTypeHTTP:
		return graphqlclient.IntentTypeHttp
	case IntentTypeKafka:
		return graphqlclient.IntentTypeKafka
	default:
		panic("Not supposed to reach here")
	}
}

func (in *ClientIntentsList) FormatAsOtterizeIntents() ([]*graphqlclient.IntentInput, error) {
	otterizeIntents := make([]*graphqlclient.IntentInput, 0)
	for _, clientIntents := range in.Items {
		for _, intent := range clientIntents.GetCallsList() {
			input := intent.ConvertToCloudFormat(clientIntents.Namespace, clientIntents.GetServiceName())
			input.Status = clientIntentsStatusToCloudFormat(clientIntents)

			otterizeIntents = append(otterizeIntents, lo.ToPtr(input))
		}
	}

	return otterizeIntents, nil
}

func clientIntentsStatusToCloudFormat(clientIntents ClientIntents) *graphqlclient.IntentStatusInput {
	var status graphqlclient.IntentStatusInput

	serviceAccountName, ok := clientIntents.Annotations[OtterizeClientServiceAccountAnnotation]
	if !ok {
		return nil
	}

	status.ServiceAccountName = toPtrOrNil(serviceAccountName)
	value, ok := clientIntents.Annotations[OtterizeSharedServiceAccountAnnotation]
	if ok {
		status.IsServiceAccountShared = lo.ToPtr(value == "true")
	}

	return &status
}

func toPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return lo.ToPtr(s)
}

func kafkaOperationK8sToCloud(op KafkaOperation) graphqlclient.KafkaOperation {
	switch op {
	case KafkaOperationAll:
		return graphqlclient.KafkaOperationAll
	case KafkaOperationConsume:
		return graphqlclient.KafkaOperationConsume
	case KafkaOperationProduce:
		return graphqlclient.KafkaOperationProduce
	case KafkaOperationCreate:
		return graphqlclient.KafkaOperationCreate
	case KafkaOperationAlter:
		return graphqlclient.KafkaOperationAlter
	case KafkaOperationDelete:
		return graphqlclient.KafkaOperationDelete
	case KafkaOperationDescribe:
		return graphqlclient.KafkaOperationDescribe
	case KafkaOperationClusterAction:
		return graphqlclient.KafkaOperationClusterAction
	case KafkaOperationDescribeConfigs:
		return graphqlclient.KafkaOperationDescribeConfigs
	case KafkaOperationAlterConfigs:
		return graphqlclient.KafkaOperationAlterConfigs
	case KafkaOperationIdempotentWrite:
		return graphqlclient.KafkaOperationIdempotentWrite
	default:
		logrus.Panic(fmt.Sprintf("Unknown KafkaOperation: %s", op))
		return "" // We won't reach here
	}
}

func (in *Intent) ConvertToCloudFormat(resourceNamespace string, clientName string) graphqlclient.IntentInput {
	otterizeTopics := lo.Map(in.Topics, func(topic KafkaTopic, i int) *graphqlclient.KafkaConfigInput {
		return lo.ToPtr(graphqlclient.KafkaConfigInput{
			Name: lo.ToPtr(topic.Name),
			Operations: lo.Map(topic.Operations, func(op KafkaOperation, i int) *graphqlclient.KafkaOperation {
				operation := kafkaOperationK8sToCloud(op)
				return &operation
			}),
		})
	})

	intentInput := graphqlclient.IntentInput{
		ClientName:      lo.ToPtr(clientName),
		ServerName:      lo.ToPtr(in.GetServerName()),
		Namespace:       lo.ToPtr(resourceNamespace),
		ServerNamespace: toPtrOrNil(in.GetServerNamespace(resourceNamespace)),
	}

	if in.Type != "" {
		intentInput.Type = lo.ToPtr(in.typeAsGQLType())
	}

	if in.HTTPResources != nil {
		intentInput.Resources = lo.Map(in.HTTPResources, intentsHTTPResourceToCloud)
	}

	if len(otterizeTopics) != 0 {
		intentInput.Topics = otterizeTopics
	}

	return intentInput
}

func intentsHTTPResourceToCloud(resource HTTPResource, index int) *graphqlclient.HTTPConfigInput {
	methods := lo.Map(resource.Methods, func(method HTTPMethod, _ int) *graphqlclient.HTTPMethod {
		return lo.ToPtr(graphqlclient.HTTPMethod(method))
	})

	httpConfig := graphqlclient.HTTPConfigInput{
		Path:    lo.ToPtr(resource.Path),
		Methods: methods,
	}

	return &httpConfig
}

// GetFormattedOtterizeIdentity truncates names and namespaces to a 20 char len string (if required)
// It also adds a short md5 hash of the full name+ns string and returns the formatted string
// This is due to Kubernetes' limit on 63 char label keys/values
func GetFormattedOtterizeIdentity(name, ns string) string {
	// Get MD5 for full length "name-namespace" string
	hash := md5.Sum([]byte(fmt.Sprintf("%s-%s", name, ns)))

	// Truncate name and namespace to 20 chars each
	if len(name) > MaxOtterizeNameLength {
		name = name[:MaxOtterizeNameLength]
	}

	if len(ns) > MaxNamespaceLength {
		ns = ns[:MaxNamespaceLength]
	}
	// A 6 char hash, even although truncated, leaves 2 ^ 48 combinations which should be enough
	// for unique identities in a k8s cluster
	hashSuffix := hex.EncodeToString(hash[:])[:6]

	return fmt.Sprintf("%s-%s-%s", name, ns, hashSuffix)

}

// BuildPodLabelSelector returns a label selector to match the otterize server labels for an intents resource
func (in *ClientIntents) BuildPodLabelSelector() (labels.Selector, error) {
	labelSelector, err := labels.Parse(
		fmt.Sprintf("%s=%s",
			OtterizeServerLabelKey,
			// Since all pods are also labeled with their server identity, we can use the Otterize server label
			// To find all pods for this specific service
			GetFormattedOtterizeIdentity(in.Spec.Service.Name, in.Namespace)))
	if err != nil {
		return nil, nil
	}

	return labelSelector, nil
}

func (in *ClientIntents) HasKafkaTypeInCallList() bool {
	for _, intent := range in.GetCallsList() {
		if intent.Type == IntentTypeKafka {
			return true
		}
	}
	return false
}
