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

package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DBPermissionChange string

const (
	DBPermissionChangeApply  DBPermissionChange = "APPLY"
	DBPermissionChangeDelete DBPermissionChange = "DELETE"
)

type DatabaseCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// PostgreSQLServerConfigSpec defines the desired state of PostgreSQLServerConfig
type PostgreSQLServerConfigSpec struct {
	DatabaseName string              `json:"databaseName"`
	Address      string              `json:"address"`
	Credentials  DatabaseCredentials `json:"credentials"`
}

// PostgreSQLServerConfigStatus defines the observed state of PostgreSQLServerConfig
type PostgreSQLServerConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:storageversion

// PostgreSQLServerConfig is the Schema for the databaseserverconfig API
type PostgreSQLServerConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgreSQLServerConfigSpec   `json:"spec,omitempty"`
	Status PostgreSQLServerConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PostgreSQLServerConfigList contains a list of PostgreSQLServerConfig
type PostgreSQLServerConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgreSQLServerConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgreSQLServerConfig{}, &PostgreSQLServerConfig{})
}
