package v2alpha1

import (
	"encoding/json"
	"fmt"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
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

func init() {
	SchemeBuilder.Register(&ApprovedClientIntents{}, &ApprovedClientIntentsList{})
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

func (in *ApprovedClientIntents) GetWorkloadName() string {
	return in.Spec.Workload.Name
}

func (in *ApprovedClientIntents) GetTargetList() []Target {
	return in.Spec.Targets
}

func (in *ApprovedClientIntents) GetFilteredTargetList(intentTypes ...IntentType) []Target {
	return lo.Filter(in.GetTargetList(), func(item Target, index int) bool {
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

func (in *ApprovedClientIntents) GetClientKind() string {
	if in.Spec.Workload.Kind == "" {
		return serviceidentity.KindOtterizeLegacy
	}
	return in.Spec.Workload.Kind
}

func (in *ApprovedClientIntents) GetIntentsLabelMapping(requestNamespace string) map[string]string {
	otterizeAccessLabels := make(map[string]string)

	for _, intent := range in.GetTargetList() {
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

func (in *ApprovedClientIntents) GetServersWithoutSidecar() (sets.Set[string], error) {
	if in.Annotations == nil {
		return sets.New[string](), nil
	}

	servers, ok := in.Annotations[OtterizeServersWithoutSidecarAnnotation]
	if !ok {
		return sets.New[string](), nil
	}

	serversList := make([]string, 0)
	err := json.Unmarshal([]byte(servers), &serversList)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	return sets.New[string](serversList...), nil
}

func (in *ApprovedClientIntents) GetDatabaseIntents() []Target {
	return in.GetFilteredTargetList(IntentTypeDatabase)
}

func (in *ApprovedClientIntents) FromClientIntents(intents ClientIntents) {
	in.Name = intents.ToApprovedIntentsName()
	in.Namespace = intents.GetNamespace()
	in.Spec = intents.Spec
}
