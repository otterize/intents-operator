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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type ProtectedService struct {
	Name string `json:"name,omitempty"`
}

// NetworkPolicyProtectedServicesSpec defines the desired state of NetworkPolicyProtectedServices
type NetworkPolicyProtectedServicesSpec struct {
	ProtectedServices []ProtectedService `json:"protectedServices,omitempty"`
}

// NetworkPolicyProtectedServicesStatus defines the observed state of NetworkPolicyProtectedServices
type NetworkPolicyProtectedServicesStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// NetworkPolicyProtectedServices is the Schema for the networkpolicyprotectedservices API
type NetworkPolicyProtectedServices struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkPolicyProtectedServicesSpec   `json:"spec,omitempty"`
	Status NetworkPolicyProtectedServicesStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NetworkPolicyProtectedServicesList contains a list of NetworkPolicyProtectedServices
type NetworkPolicyProtectedServicesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkPolicyProtectedServices `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkPolicyProtectedServices{}, &NetworkPolicyProtectedServicesList{})
}
