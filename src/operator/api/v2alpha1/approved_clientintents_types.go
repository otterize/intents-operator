package v2alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func init() {
	SchemeBuilder.Register(&ApprovedClientIntents{}, &ApprovedClientIntentsList{})
}

// +kubebuilder:validation:Enum=CREATED;PENDING;IN PROGRESS;FAILED
type PolicyStatus string

const (
	PolicySuccess    PolicyStatus = "CREATED"
	PolicyPending    PolicyStatus = "PENDING"
	PolicyInProgress PolicyStatus = "IN PROGRESS"
	PolicyFailed     PolicyStatus = "FAILED"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ApprovedClientIntents is the Schema for the intents API
type ApprovedClientIntents struct {
	metav1.TypeMeta   `json:",inline" yaml:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`

	Spec   *IntentsSpec                `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status ApprovedClientIntentsStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// ApprovedClientIntentsList contains a list of ApprovedClientIntents
type ApprovedClientIntentsList struct {
	metav1.TypeMeta `json:",inline" yaml:",inline"`
	metav1.ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items           []ApprovedClientIntents `json:"items" yaml:"items"`
}

func (in *ApprovedClientIntentsList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

type ApprovedClientIntentsStatus struct {
	PolicyStatus PolicyStatus `json:"policyStatus,omitempty" yaml:"policyStatus,omitempty"`
}

type ApprovedIntentsStatus struct {
	PolicyStatus PolicyStatus `json:"policyStatus,omitempty" yaml:"policyStatus,omitempty"`
}
