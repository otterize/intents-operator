package v2alpha1

import (
	"fmt"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func init() {
	SchemeBuilder.Register(&ApprovedClientIntents{}, &ApprovedClientIntentsList{})
}

// +kubebuilder:validation:Enum=CREATED;PENDING;IN PROGRESS;FAILED
type PolicyStatus string

const (
	PolicyStatusSuccess    PolicyStatus = "CREATED SUCCESSFULLY"
	PolicyStatusPending    PolicyStatus = "PENDING"
	PolicyStatusInProgress PolicyStatus = "IN PROGRESS"
	PolicyStatusFailed     PolicyStatus = "FAILED"
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

func (aci *ApprovedClientIntents) GetWorkloadName() string {
	return aci.Spec.Workload.Name
}

func (aci *ApprovedClientIntents) GetTargetList() []Target {
	return aci.Spec.Targets
}

func (aci *ApprovedClientIntents) GetFilteredTargetList(intentTypes ...IntentType) []Target {
	return lo.Filter(aci.GetTargetList(), func(item Target, index int) bool {
		for _, intentType := range intentTypes {
			if intentType == IntentTypeHTTP {
				if item.Kubernetes != nil && len(item.Kubernetes.HTTP) > 0 {
					return true
				}
				if item.Service != nil && len(item.Service.HTTP) > 0 {
					return true
				}
			}
			if intentType == IntentTypeKafka && item.Kafka != nil {
				return true
			}
			if intentType == IntentTypeDatabase && item.SQL != nil {
				return true
			}
			if intentType == IntentTypeAWS && item.AWS != nil {
				return true
			}
			if intentType == IntentTypeGCP && item.GCP != nil {
				return true
			}
			if intentType == IntentTypeAzure && item.Azure != nil {
				return true
			}
			if intentType == IntentTypeInternet && item.Internet != nil {
				return true
			}
		}
		return false
	})
}

func (aci *ApprovedClientIntents) GetClientKind() string {
	if aci.Spec.Workload.Kind == "" {
		return serviceidentity.KindOtterizeLegacy
	}
	return aci.Spec.Workload.Kind
}

func (aci *ApprovedClientIntents) GetIntentsLabelMapping(requestNamespace string) map[string]string {
	otterizeAccessLabels := make(map[string]string)

	for _, intent := range aci.GetTargetList() {
		if intent.IsTargetOutOfCluster() {
			continue
		}
		targetServiceIdentity := intent.ToServiceIdentity(requestNamespace)
		labelKey := fmt.Sprintf(OtterizeAccessLabelKey, targetServiceIdentity.GetFormattedOtterizeIdentityWithKind())
		if intent.IsTargetServerKubernetesService() {
			labelKey = fmt.Sprintf(OtterizeSvcAccessLabelKey, targetServiceIdentity.GetFormattedOtterizeIdentityWithKind())
		}
		otterizeAccessLabels[labelKey] = "true"
	}

	return otterizeAccessLabels
}

func (aci *ApprovedClientIntents) GetDatabaseIntents() []Target {
	return aci.GetFilteredTargetList(IntentTypeDatabase)
}

func (aci *ApprovedClientIntents) FromClientIntents(intents ClientIntents) {
	aci.Name = intents.ToApprovedIntentsName()
	aci.Namespace = intents.GetNamespace()
	aci.Spec.Targets = intents.GetTargetList()
	aci.Spec.Workload = intents.Spec.Workload
}
