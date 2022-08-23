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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// KafkaServerConfigSpec defines the desired state of KafkaServerConfig
type KafkaServerConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of KafkaServerConfig. Edit kafkaserverconfig_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// KafkaServerConfigStatus defines the observed state of KafkaServerConfig
type KafkaServerConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

type TLSSource struct {
	CertFile   string `json:"certFile,omitempty"`
	KeyFile    string `json:"keyFile,omitempty"`
	RootCAFile string `json:"rootCAFile,omitempty"`
}

type ResourcePatternType string

const (
	ResourcePatternTypeLiteral = "literal"
	ResourcePatternTypePrefix  = "prefix"
)

type TopicConfig struct {
	Topic                  string              `json:"topic"`
	Pattern                ResourcePatternType `json:"pattern"`
	ClientIdentityRequired bool                `json:"clientIdentityRequired"`
	IntentsRequired        bool                `json:"intentsRequired"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// KafkaServerConfig is the Schema for the kafkaserverconfigs API
type KafkaServerConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KafkaServerConfigSpec   `json:"spec,omitempty"`
	Status KafkaServerConfigStatus `json:"status,omitempty"`

	ServerName string        `json:"serverName,omitempty"`
	Addr       string        `json:"addr,omitempty"`
	TLS        TLSSource     `json:"tls"`
	Topics     []TopicConfig `json:"topics"`
}

//+kubebuilder:object:root=true

// KafkaServerConfigList contains a list of KafkaServerConfig
type KafkaServerConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KafkaServerConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KafkaServerConfig{}, &KafkaServerConfigList{})
}
