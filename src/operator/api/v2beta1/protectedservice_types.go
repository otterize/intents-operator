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

package v2beta1

import (
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProtectedServiceSpec defines the desired state of ProtectedService
type ProtectedServiceSpec struct {
	Name string `json:"name,omitempty"`

	//+optional
	Kind string `json:"kind,omitempty" yaml:"kind,omitempty"`
}

// ProtectedServiceStatus defines the observed state of ProtectedService
type ProtectedServiceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ProtectedService is the Schema for the protectedservice API
type ProtectedService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProtectedServiceSpec   `json:"spec,omitempty"`
	Status ProtectedServiceStatus `json:"status,omitempty"`
}

func (in *ProtectedService) GetKind() string {
	if in.Spec.Kind == "" {
		return serviceidentity.KindOtterizeLegacy
	}
	return in.Spec.Kind
}

//+kubebuilder:object:root=true

// ProtectedServiceList contains a list of ProtectedService
type ProtectedServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProtectedService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProtectedService{}, &ProtectedServiceList{})
}
