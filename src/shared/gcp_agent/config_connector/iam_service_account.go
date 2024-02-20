package config_connector

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type IAMServiceAccountSpec struct {
	Description *string `json:"description,omitempty"`
	Disabled    *bool   `json:"disabled,omitempty"`
	DisplayName *string `json:"displayName,omitempty"`
	ResourceID  *string `json:"resourceID,omitempty"`
}

type IAMServiceAccount struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec IAMServiceAccountSpec `json:"spec,omitempty"`
}
