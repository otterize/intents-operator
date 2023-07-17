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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ProtectedService struct {
	Name string `json:"name,omitempty"`
}

// ProtectedServicesSpec defines the desired state of ProtectedServices
type ProtectedServicesSpec struct {
	ProtectedServices []ProtectedService `json:"protectedServices,omitempty"`
}

// ProtectedServicesStatus defines the observed state of ProtectedServices
type ProtectedServicesStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ProtectedServices is the Schema for the protectedservices API
type ProtectedServices struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProtectedServicesSpec   `json:"spec,omitempty"`
	Status ProtectedServicesStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ProtectedServicesList contains a list of ProtectedServices
type ProtectedServicesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProtectedServices `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProtectedServices{}, &ProtectedServicesList{})
}
