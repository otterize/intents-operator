package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MySQLServerConfigSpec defines the desired state of MySQLServerConfig
type MySQLServerConfigSpec struct {
	Address     string              `json:"address"`
	Credentials DatabaseCredentials `json:"credentials"`
}

// MySQLServerConfigStatus defines the observed state of MySQLServerConfig
type MySQLServerConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// MySQLServerConfig is the Schema for the mysqlserverconfig API
type MySQLServerConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MySQLServerConfigSpec   `json:"spec,omitempty"`
	Status MySQLServerConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MySQLServerConfigList contains a list of MySQLServerConfig
type MySQLServerConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MySQLServerConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MySQLServerConfig{}, &MySQLServerConfigList{})
}
