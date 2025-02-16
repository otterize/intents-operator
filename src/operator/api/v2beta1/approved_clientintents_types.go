package v2beta1

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
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
	// upToDate field reflects whether the client intents have successfully been applied
	// to the cluster to the state specified
	// +optional
	UpToDate bool `json:"upToDate"`
	// The last generation of the intents that was successfully reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration"`
	// ResolvedIPs stores resolved IPs for a domain name - the network mapper populates it when DNS internetTarget is used
	// +optional
	ResolvedIPs []ResolvedIPs `json:"resolvedIPs,omitempty" yaml:"resolvedIPs,omitempty"`
	// ReviewStatus stores the review status for these ClientIntents (whether they were approved, denied, or pending)
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

func (in *ApprovedClientIntentsList) FormatAsOtterizeIntents(ctx context.Context, k8sClient client.Client) ([]*graphqlclient.IntentInput, error) {
	otterizeIntents := make([]*graphqlclient.IntentInput, 0)
	for _, clientIntents := range in.Items {
		for _, intent := range clientIntents.GetTargetList() {
			clientServiceIdentity := clientIntents.ToServiceIdentity()
			input, err := intent.ConvertToCloudFormat(ctx, k8sClient, clientServiceIdentity)
			if err != nil {
				return nil, errors.Wrap(err)
			}
			statusInput, ok, err := approvedClientIntentsStatusToCloudFormat(clientIntents, intent)
			if err != nil {
				return nil, errors.Wrap(err)
			}

			input.Status = nil
			if ok {
				input.Status = statusInput
			}
			otterizeIntents = append(otterizeIntents, lo.ToPtr(input))
		}
	}

	return otterizeIntents, nil
}

func approvedClientIntentsStatusToCloudFormat(approvedClientIntents ApprovedClientIntents, intent Target) (*graphqlclient.IntentStatusInput, bool, error) {
	status := graphqlclient.IntentStatusInput{
		IstioStatus: &graphqlclient.IstioStatusInput{},
	}

	serviceAccountName, ok := approvedClientIntents.Annotations[OtterizeClientServiceAccountAnnotation]
	if !ok {
		// Status is not set, nothing to do
		return nil, false, nil
	}

	status.IstioStatus.ServiceAccountName = toPtrOrNil(serviceAccountName)
	isSharedValue, ok := approvedClientIntents.Annotations[OtterizeSharedServiceAccountAnnotation]
	if !ok {
		return nil, false, errors.Errorf("missing annotation shared service account for client intents %s", approvedClientIntents.Name)
	}

	isShared, err := strconv.ParseBool(isSharedValue)
	if err != nil {
		return nil, false, errors.Errorf("failed to parse shared service account annotation for client intents %s", approvedClientIntents.Name)
	}
	status.IstioStatus.IsServiceAccountShared = lo.ToPtr(isShared)

	clientMissingSidecarValue, ok := approvedClientIntents.Annotations[OtterizeMissingSidecarAnnotation]
	if !ok {
		return nil, false, errors.Errorf("missing annotation missing sidecar for client intents %s", approvedClientIntents.Name)
	}

	clientMissingSidecar, err := strconv.ParseBool(clientMissingSidecarValue)
	if err != nil {
		return nil, false, errors.Errorf("failed to parse missing sidecar annotation for client intents %s", approvedClientIntents.Name)
	}
	status.IstioStatus.IsClientMissingSidecar = lo.ToPtr(clientMissingSidecar)
	isServerMissingSidecar, err := approvedClientIntents.IsServerMissingSidecar(intent)
	if err != nil {
		return nil, false, errors.Wrap(err)
	}
	status.IstioStatus.IsServerMissingSidecar = lo.ToPtr(isServerMissingSidecar)
	return &status, true, nil
}

func (in *ApprovedClientIntents) IsServerMissingSidecar(intent Target) (bool, error) {
	serversSet, err := in.GetServersWithoutSidecar()
	if err != nil {
		return false, errors.Wrap(err)
	}
	identity := intent.ToServiceIdentity(in.Namespace)
	serverIdentity := identity.GetFormattedOtterizeIdentityWithoutKind()
	return serversSet.Has(serverIdentity), nil
}
