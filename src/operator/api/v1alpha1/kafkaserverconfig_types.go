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

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type TLSSource struct {
	CertFile   string `json:"certFile,omitempty" yaml:"certFile,omitempty"`
	KeyFile    string `json:"keyFile,omitempty" yaml:"keyFile,omitempty"`
	RootCAFile string `json:"rootCAFile,omitempty" yaml:"rootCAFile,omitempty"`
}

type ResourcePatternType string

const (
	ResourcePatternTypeLiteral = "literal"
	ResourcePatternTypePrefix  = "prefix"
	UsernameMappingDefault     = "CN=$ServiceName.$Namespace,O=SPIRE,C=US"
)

type TopicConfig struct {
	Topic                  string              `json:"topic" yaml:"topic"`
	Pattern                ResourcePatternType `json:"pattern" yaml:"pattern"`
	ClientIdentityRequired bool                `json:"clientIdentityRequired" yaml:"clientIdentityRequired"`
	IntentsRequired        bool                `json:"intentsRequired" yaml:"intentsRequired"`
}

// KafkaServerConfigSpec defines the desired state of KafkaServerConfig
type KafkaServerConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	ServerName string        `json:"serverName,omitempty" yaml:"serverName,omitempty"`
	Addr       string        `json:"addr,omitempty" yaml:"addr,omitempty"`
	TLS        TLSSource     `json:"tls" yaml:"tls"`
	UsernameMapping string        `json:"usernameMapping,omitempty" yaml:"usernameMapping,omitempty"`
	Topics     []TopicConfig `json:"topics,omitempty" yaml:"topics,omitempty"`
}

// KafkaServerConfigStatus defines the observed state of KafkaServerConfig
type KafkaServerConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// KafkaServerConfig is the Schema for the kafkaserverconfigs API
type KafkaServerConfig struct {
	metav1.TypeMeta   `json:",inline" yaml:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`

	Spec   KafkaServerConfigSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status KafkaServerConfigStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KafkaServerConfigList contains a list of KafkaServerConfig
type KafkaServerConfigList struct {
	metav1.TypeMeta `json:",inline" yaml:",inline"`
	metav1.ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items           []KafkaServerConfig `json:"items" yaml:"items"`
}

func init() {
	SchemeBuilder.Register(&KafkaServerConfig{}, &KafkaServerConfigList{})
}
