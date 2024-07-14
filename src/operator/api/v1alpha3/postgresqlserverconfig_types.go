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

type DatabaseCredentialsSecretRef struct {
	// Name is the name of he k8s secret storing the credentials
	Name string `json:"name,omitempty"`
	// Namespace is the namespace in which the secret is stored.
	// If not provided, the operator will look for the secret in the same namespace as the database ServerConfig.
	//+optional
	Namespace string `json:"namespace,omitempty"`
	// UsernameKey is the key in the secret that stores the username
	//+optional
	//+kubebuilder:default="username"
	UsernameKey string `json:"usernameKey,omitempty"`
	// PasswordKey is the key in the secret that stores the password
	//+optional
	//+kubebuilder:default="password"
	PasswordKey string `json:"passwordKey,omitempty"`
}

// DatabaseCredentials defines the credentials to access the database
type DatabaseCredentials struct {
	// Username is the plaintext username to access the database
	//+optional
	Username string `json:"username,omitempty"`
	// Password is the plaintext password to access the database
	//+optional
	Password string `json:"password,omitempty"`
	// SecretRef is a reference to a k8s secret storing the credentials
	//+optional
	SecretRef *DatabaseCredentialsSecretRef `json:"secretRef,omitempty"`
}

// PostgreSQLServerConfigSpec defines the desired state of PostgreSQLServerConfig
type PostgreSQLServerConfigSpec struct {
	Address     string              `json:"address"`
	Credentials DatabaseCredentials `json:"credentials"`
}

// PostgreSQLServerConfigStatus defines the observed state of PostgreSQLServerConfig
type PostgreSQLServerConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// PostgreSQLServerConfig is the Schema for the postgresqlserverconfig API
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
	SchemeBuilder.Register(&PostgreSQLServerConfig{}, &PostgreSQLServerConfigList{})
}
