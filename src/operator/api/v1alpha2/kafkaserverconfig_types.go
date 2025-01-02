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
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type TLSSource struct {
	// +kubebuilder:validation:Required
	CertFile string `json:"certFile" yaml:"certFile"`
	// +kubebuilder:validation:Required
	KeyFile string `json:"keyFile" yaml:"keyFile"`
	// +kubebuilder:validation:Required
	RootCAFile string `json:"rootCAFile" yaml:"rootCAFile"`
}

// +kubebuilder:validation:Enum=literal;prefix
type ResourcePatternType string

const (
	ResourcePatternTypeLiteral = "literal"
	ResourcePatternTypePrefix  = "prefix"
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

	Service Service `json:"service,omitempty" yaml:"service,omitempty"`
	// If Intents for network policies are enabled, and there are other Intents to this Kafka server,
	// will automatically create an Intent so that the Intents Operator can connect. Set to true to disable.
	NoAutoCreateIntentsForOperator bool   `json:"noAutoCreateIntentsForOperator,omitempty" yaml:"noAutoCreateIntentsForOperator,omitempty"`
	Addr                           string `json:"addr,omitempty" yaml:"addr,omitempty"`
	// +kubebuilder:validation:Optional
	TLS    TLSSource     `json:"tls,omitempty" yaml:"tls,omitempty"`
	Topics []TopicConfig `json:"topics,omitempty" yaml:"topics,omitempty"`
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

// ConvertTo converts this ProtectedService to the Hub version (v1alpha3).
func (ksc *KafkaServerConfig) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v2alpha1.KafkaServerConfig)
	dst.ObjectMeta = ksc.ObjectMeta
	dst.Spec = v2alpha1.KafkaServerConfigSpec{}
	dst.Spec.Addr = ksc.Spec.Addr
	dst.Spec.Workload = v2alpha1.Workload{Name: ksc.Spec.Service.Name}
	dst.Spec.NoAutoCreateIntentsForOperator = ksc.Spec.NoAutoCreateIntentsForOperator
	dst.Spec.TLS = v2alpha1.TLSSource{
		CertFile:   ksc.Spec.TLS.CertFile,
		KeyFile:    ksc.Spec.TLS.KeyFile,
		RootCAFile: ksc.Spec.TLS.RootCAFile,
	}
	for _, topic := range ksc.Spec.Topics {
		dst.Spec.Topics = append(dst.Spec.Topics, v2alpha1.TopicConfig{
			Topic:                  topic.Topic,
			Pattern:                v2alpha1.ResourcePatternType(topic.Pattern), // this casting is fine as v1alpha2 == v1alpha3
			ClientIdentityRequired: topic.ClientIdentityRequired,
			IntentsRequired:        topic.IntentsRequired,
		})
	}
	return nil
}

// ConvertFrom converts the Hub version (v1alpha3) to this KafkaServerConfig.
func (ksc *KafkaServerConfig) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v2alpha1.KafkaServerConfig)
	ksc.ObjectMeta = src.ObjectMeta
	ksc.Spec = KafkaServerConfigSpec{}
	ksc.Spec.Addr = src.Spec.Addr
	ksc.Spec.Service = Service{Name: src.Spec.Workload.Name}
	ksc.Spec.NoAutoCreateIntentsForOperator = src.Spec.NoAutoCreateIntentsForOperator
	ksc.Spec.TLS = TLSSource{
		CertFile:   src.Spec.TLS.CertFile,
		KeyFile:    src.Spec.TLS.KeyFile,
		RootCAFile: src.Spec.TLS.RootCAFile,
	}
	for _, topic := range src.Spec.Topics {
		ksc.Spec.Topics = append(ksc.Spec.Topics, TopicConfig{
			Topic:                  topic.Topic,
			Pattern:                ResourcePatternType(topic.Pattern), // this casting is fine as v1alpha2 == v1alpha3
			ClientIdentityRequired: topic.ClientIdentityRequired,
			IntentsRequired:        topic.IntentsRequired,
		})
	}
	return nil
}
