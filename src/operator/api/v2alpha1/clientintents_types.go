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

package v2alpha1

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/podownerresolver"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"strings"

	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	OtterizeAccessLabelPrefix                    = "intents.otterize.com/access"
	OtterizeServiceAccessLabelPrefix             = "intents.otterize.com/svc-access"
	OtterizeAccessLabelKey                       = "intents.otterize.com/access-%s"
	OtterizeSvcAccessLabelKey                    = "intents.otterize.com/svc-access-%s"
	OtterizeClientLabelKey                       = "intents.otterize.com/client"
	OtterizeServiceLabelKey                      = "intents.otterize.com/service"
	OtterizeOwnerKindLabelKey                    = "intents.otterize.com/owner-kind"
	OtterizeServerLabelKeyDeprecated             = "intents.otterize.com/server"
	OtterizeKubernetesServiceLabelKeyPrefix      = "intents.otterize.com/k8s-svc"
	OtterizeKubernetesServiceLabelKey            = "intents.otterize.com/k8s-svc-%s"
	KubernetesStandardNamespaceNameLabelKey      = "kubernetes.io/metadata.name"
	AllIntentsRemovedAnnotation                  = "intents.otterize.com/all-intents-removed"
	OtterizeCreatedForServiceAnnotation          = "intents.otterize.com/created-for-service"
	OtterizeCreatedForIngressAnnotation          = "intents.otterize.com/created-for-ingress"
	OtterizeSingleNetworkPolicyNameTemplate      = "%s-access"
	OtterizeLinkerdMeshTLSAnnotationKey          = "intents.otterize.com/linkerd-authenticates"
	OtterizeLinkerdServerAnnotationKey           = "intents.otterize.com/linkerd-server"
	OtterizeNetworkPolicyIngressNameTemplate     = "%s-ingress"
	OtterizeNetworkPolicyEgressNameTemplate      = "%s-egress"
	OtterizeNetworkPolicy                        = "intents.otterize.com/network-policy"
	OtterizeSvcNetworkPolicy                     = "intents.otterize.com/svc-network-policy"
	OtterizeNetworkPolicyServiceDefaultDeny      = "intents.otterize.com/network-policy-service-default-deny"
	OtterizeNetworkPolicyExternalTraffic         = "intents.otterize.com/network-policy-external-traffic"
	ClientIntentsFinalizerName                   = "intents.otterize.com/client-intents-finalizer"
	ProtectedServicesFinalizerName               = "intents.otterize.com/protected-services-finalizer"
	OtterizeIstioClientAnnotationKeyDeprecated   = "intents.otterize.com/istio-client"
	OtterizeIstioClientWithKindLabelKey          = "intents.otterize.com/istio-client-with-kind"
	OtterizeClientServiceAccountAnnotation       = "intents.otterize.com/client-intents-service-account"
	OtterizeSharedServiceAccountAnnotation       = "intents.otterize.com/shared-service-account"
	OtterizeMissingSidecarAnnotation             = "intents.otterize.com/service-missing-sidecar"
	OtterizeServersWithoutSidecarAnnotation      = "intents.otterize.com/servers-without-sidecar"
	OtterizeTargetServerIndexField               = "spec.service.calls.server"
	OtterizeKafkaServerConfigServiceNameField    = "spec.service.name"
	OtterizeProtectedServiceNameIndexField       = "spec.name"
	OtterizeFormattedTargetServerIndexField      = "formattedTargetServer"
	OtterizePodByOwnerKindAndNameIndexField      = "podByOwnerKindAndName"
	EndpointsPodNamesIndexField                  = "endpointsPodNames"
	IngressServiceNamesIndexField                = "ingressServiceNames"
	OtterizeClientIntentsPendingReviewIndexField = "pendingReview"
	MaxOtterizeNameLength                        = 20
	MaxNamespaceLength                           = 20
	OtterizeSvcEgressNetworkPolicy               = "intents.otterize.com/svc-egress-network-policy"
	OtterizeEgressNetworkPolicy                  = "intents.otterize.com/egress-network-policy"
	OtterizeInternetNetworkPolicy                = "intents.otterize.com/egress-internet-network-policy"
	OtterizeInternetTargetName                   = "internet"
	KubernetesAPIServerName                      = "kubernetes"
	KubernetesAPIServerNamespace                 = "default"
)

// +kubebuilder:validation:Enum=http;kafka;database;aws;gcp;azure;internet
type IntentType string

const (
	IntentTypeHTTP     IntentType = "http"
	IntentTypeKafka    IntentType = "kafka"
	IntentTypeDatabase IntentType = "database"
	IntentTypeAWS      IntentType = "aws"
	IntentTypeGCP      IntentType = "gcp"
	IntentTypeAzure    IntentType = "azure"
	IntentTypeInternet IntentType = "internet"
)

// +kubebuilder:validation:Enum=all;consume;produce;create;alter;delete;describe;ClusterAction;DescribeConfigs;AlterConfigs;IdempotentWrite
type KafkaOperation string

const (
	KafkaOperationAll             KafkaOperation = "all"
	KafkaOperationConsume         KafkaOperation = "consume"
	KafkaOperationProduce         KafkaOperation = "produce"
	KafkaOperationCreate          KafkaOperation = "create"
	KafkaOperationAlter           KafkaOperation = "alter"
	KafkaOperationDelete          KafkaOperation = "delete"
	KafkaOperationDescribe        KafkaOperation = "describe"
	KafkaOperationClusterAction   KafkaOperation = "ClusterAction"
	KafkaOperationDescribeConfigs KafkaOperation = "DescribeConfigs"
	KafkaOperationAlterConfigs    KafkaOperation = "AlterConfigs"
	KafkaOperationIdempotentWrite KafkaOperation = "IdempotentWrite"
)

// +kubebuilder:validation:Enum=GET;POST;PUT;DELETE;OPTIONS;TRACE;PATCH;CONNECT
type HTTPMethod string

const (
	HTTPMethodGet     HTTPMethod = "GET"
	HTTPMethodPost    HTTPMethod = "POST"
	HTTPMethodPut     HTTPMethod = "PUT"
	HTTPMethodDelete  HTTPMethod = "DELETE"
	HTTPMethodOptions HTTPMethod = "OPTIONS"
	HTTPMethodTrace   HTTPMethod = "TRACE"
	HTTPMethodPatch   HTTPMethod = "PATCH"
	HTTPMethodConnect HTTPMethod = "CONNECT"
)

// +kubebuilder:validation:Enum=ALL;SELECT;INSERT;UPDATE;DELETE
type DatabaseOperation string

const (
	DatabaseOperationAll    DatabaseOperation = "ALL"
	DatabaseOperationSelect DatabaseOperation = "SELECT"
	DatabaseOperationInsert DatabaseOperation = "INSERT"
	DatabaseOperationUpdate DatabaseOperation = "UPDATE"
	DatabaseOperationDelete DatabaseOperation = "DELETE"
)

// +kubebuilder:validation:Enum=all;backup;create;delete;deleteissuers;get;getissuers;import;list;listissuers;managecontacts;manageissuers;purge;recover;restore;setissuers;update
type AzureKeyVaultCertificatePermission string

const (
	AzureKeyVaultCertificatePermissionAll            AzureKeyVaultCertificatePermission = "all"
	AzureKeyVaultCertificatePermissionBackup         AzureKeyVaultCertificatePermission = "backup"
	AzureKeyVaultCertificatePermissionCreate         AzureKeyVaultCertificatePermission = "create"
	AzureKeyVaultCertificatePermissionDelete         AzureKeyVaultCertificatePermission = "delete"
	AzureKeyVaultCertificatePermissionDeleteIssuers  AzureKeyVaultCertificatePermission = "deleteissuers"
	AzureKeyVaultCertificatePermissionGet            AzureKeyVaultCertificatePermission = "get"
	AzureKeyVaultCertificatePermissionGetIssuers     AzureKeyVaultCertificatePermission = "getissuers"
	AzureKeyVaultCertificatePermissionImport         AzureKeyVaultCertificatePermission = "import"
	AzureKeyVaultCertificatePermissionList           AzureKeyVaultCertificatePermission = "list"
	AzureKeyVaultCertificatePermissionListIssuers    AzureKeyVaultCertificatePermission = "listissuers"
	AzureKeyVaultCertificatePermissionManageContacts AzureKeyVaultCertificatePermission = "managecontacts"
	AzureKeyVaultCertificatePermissionManageIssuers  AzureKeyVaultCertificatePermission = "manageissuers"
	AzureKeyVaultCertificatePermissionPurge          AzureKeyVaultCertificatePermission = "purge"
	AzureKeyVaultCertificatePermissionRecover        AzureKeyVaultCertificatePermission = "recover"
	AzureKeyVaultCertificatePermissionRestore        AzureKeyVaultCertificatePermission = "restore"
	AzureKeyVaultCertificatePermissionSetIssuers     AzureKeyVaultCertificatePermission = "setissuers"
	AzureKeyVaultCertificatePermissionUpdate         AzureKeyVaultCertificatePermission = "update"
)

// +kubebuilder:validation:Enum=all;backup;create;decrypt;delete;encrypt;get;getrotationpolicy;import;list;purge;recover;release;restore;rotate;setrotationpolicy;sign;unwrapkey;update;verify;wrapkey
type AzureKeyVaultKeyPermission string

const (
	AzureKeyVaultKeyPermissionAll               AzureKeyVaultKeyPermission = "all"
	AzureKeyVaultKeyPermissionBackup            AzureKeyVaultKeyPermission = "backup"
	AzureKeyVaultKeyPermissionCreate            AzureKeyVaultKeyPermission = "create"
	AzureKeyVaultKeyPermissionDecrypt           AzureKeyVaultKeyPermission = "decrypt"
	AzureKeyVaultKeyPermissionDelete            AzureKeyVaultKeyPermission = "delete"
	AzureKeyVaultKeyPermissionEncrypt           AzureKeyVaultKeyPermission = "encrypt"
	AzureKeyVaultKeyPermissionGet               AzureKeyVaultKeyPermission = "get"
	AzureKeyVaultKeyPermissionGetRotationPolicy AzureKeyVaultKeyPermission = "getrotationpolicy"
	AzureKeyVaultKeyPermissionImport            AzureKeyVaultKeyPermission = "import"
	AzureKeyVaultKeyPermissionList              AzureKeyVaultKeyPermission = "list"
	AzureKeyVaultKeyPermissionPurge             AzureKeyVaultKeyPermission = "purge"
	AzureKeyVaultKeyPermissionRecover           AzureKeyVaultKeyPermission = "recover"
	AzureKeyVaultKeyPermissionRelease           AzureKeyVaultKeyPermission = "release"
	AzureKeyVaultKeyPermissionRestore           AzureKeyVaultKeyPermission = "restore"
	AzureKeyVaultKeyPermissionRotate            AzureKeyVaultKeyPermission = "rotate"
	AzureKeyVaultKeyPermissionSetRotationPolicy AzureKeyVaultKeyPermission = "setrotationpolicy"
	AzureKeyVaultKeyPermissionSign              AzureKeyVaultKeyPermission = "sign"
	AzureKeyVaultKeyPermissionUnwrapKey         AzureKeyVaultKeyPermission = "unwrapkey"
	AzureKeyVaultKeyPermissionUpdate            AzureKeyVaultKeyPermission = "update"
	AzureKeyVaultKeyPermissionVerify            AzureKeyVaultKeyPermission = "verify"
	AzureKeyVaultKeyPermissionWrapKey           AzureKeyVaultKeyPermission = "wrapkey"
)

// +kubebuilder:validation:Enum=all;backup;delete;get;list;purge;recover;restore;set
type AzureKeyVaultSecretPermission string

const (
	AzureKeyVaultSecretPermissionAll     AzureKeyVaultSecretPermission = "all"
	AzureKeyVaultSecretPermissionBackup  AzureKeyVaultSecretPermission = "backup"
	AzureKeyVaultSecretPermissionDelete  AzureKeyVaultSecretPermission = "delete"
	AzureKeyVaultSecretPermissionGet     AzureKeyVaultSecretPermission = "get"
	AzureKeyVaultSecretPermissionList    AzureKeyVaultSecretPermission = "list"
	AzureKeyVaultSecretPermissionPurge   AzureKeyVaultSecretPermission = "purge"
	AzureKeyVaultSecretPermissionRecover AzureKeyVaultSecretPermission = "recover"
	AzureKeyVaultSecretPermissionRestore AzureKeyVaultSecretPermission = "restore"
	AzureKeyVaultSecretPermissionSet     AzureKeyVaultSecretPermission = "set"
)

// +kubebuilder:validation:Enum=all;backup;delete;deletesas;get;getsas;list;listsas;purge;recover;regeneratekey;restore;set;setsas;update
type AzureKeyVaultStoragePermission string

const (
	AzureKeyVaultStoragePermissionAll           AzureKeyVaultStoragePermission = "all"
	AzureKeyVaultStoragePermissionBackup        AzureKeyVaultStoragePermission = "backup"
	AzureKeyVaultStoragePermissionDelete        AzureKeyVaultStoragePermission = "delete"
	AzureKeyVaultStoragePermissionDeleteSas     AzureKeyVaultStoragePermission = "deletesas"
	AzureKeyVaultStoragePermissionGet           AzureKeyVaultStoragePermission = "get"
	AzureKeyVaultStoragePermissionGetSas        AzureKeyVaultStoragePermission = "getsas"
	AzureKeyVaultStoragePermissionList          AzureKeyVaultStoragePermission = "list"
	AzureKeyVaultStoragePermissionListSas       AzureKeyVaultStoragePermission = "listsas"
	AzureKeyVaultStoragePermissionPurge         AzureKeyVaultStoragePermission = "purge"
	AzureKeyVaultStoragePermissionRecover       AzureKeyVaultStoragePermission = "recover"
	AzureKeyVaultStoragePermissionRegenerateKey AzureKeyVaultStoragePermission = "regeneratekey"
	AzureKeyVaultStoragePermissionRestore       AzureKeyVaultStoragePermission = "restore"
	AzureKeyVaultStoragePermissionSet           AzureKeyVaultStoragePermission = "set"
	AzureKeyVaultStoragePermissionSetSas        AzureKeyVaultStoragePermission = "setsas"
	AzureKeyVaultStoragePermissionUpdate        AzureKeyVaultStoragePermission = "update"
)

// IntentsSpec defines the desired state of ClientIntents
type IntentsSpec struct {
	Workload Workload `json:"workload" yaml:"workload"`
	Targets  []Target `json:"targets" yaml:"targets"`
}

type Workload struct {
	Name string `json:"name" yaml:"name"`
	//+optional
	Kind string `json:"kind,omitempty" yaml:"kind,omitempty"`
}

type Target struct {
	//+optional
	Kubernetes *KubernetesTarget `json:"kubernetes,omitempty" yaml:"kubernetes,omitempty"`

	//+optional
	Service *ServiceTarget `json:"service,omitempty" yaml:"service,omitempty"`

	//+optional
	Kafka *KafkaTarget `json:"kafka,omitempty" yaml:"kafka,omitempty"`

	//+optional
	SQL *SQLTarget `json:"sql,omitempty" yaml:"sql,omitempty"`

	//+optional
	AWS *AWSTarget `json:"aws,omitempty" yaml:"aws,omitempty"`

	//+optional
	GCP *GCPTarget `json:"gcp,omitempty" yaml:"gcp,omitempty"`

	//+optional
	Azure *AzureTarget `json:"azure,omitempty" yaml:"azure,omitempty"`

	//+optional
	Internet *Internet `json:"internet,omitempty" yaml:"internet,omitempty"`
}

type AWSTarget struct {
	ARN string `json:"arn,omitempty" yaml:"arn,omitempty"`
	//+optional
	Actions []string `json:"actions,omitempty" yaml:"actions,omitempty"`
}

type GCPTarget struct {
	Resource    string   `json:"resource,omitempty" yaml:"resource,omitempty"`
	Permissions []string `json:"permissions,omitempty" yaml:"permissions,omitempty"`
}

type AzureTarget struct {
	Scope string `json:"scope,omitempty" yaml:"scope,omitempty"`
	//+optional
	Roles []string `json:"roles,omitempty" yaml:"roles,omitempty"`
	//+optional
	KeyVaultPolicy *AzureKeyVaultPolicy `json:"keyVaultPolicy,omitempty" yaml:"keyVaultPolicy,omitempty"`
	//+optional
	Actions []AzureAction `json:"actions,omitempty" yaml:"actions,omitempty"`
	//+optional
	DataActions []AzureDataAction `json:"dataActions,omitempty" yaml:"dataActions,omitempty"`
}

type AzureAction string

type AzureDataAction string

type KubernetesTarget struct {
	Name string `json:"name" yaml:"name"`
	//+optional
	Kind string `json:"kind,omitempty" yaml:"kind,omitempty"`
	//+optional
	HTTP []HTTPTarget `json:"http,omitempty" yaml:"http,omitempty"`
}

type ServiceTarget struct {
	Name string       `json:"name" yaml:"name"`
	HTTP []HTTPTarget `json:"http,omitempty" yaml:"http,omitempty"`
}

type Internet struct {
	//+optional
	Domains []string `json:"domains,omitempty" yaml:"domains,omitempty"`
	//+optional
	Ips []string `json:"ips,omitempty" yaml:"ips,omitempty"`
	//+optional
	Ports []int `json:"ports,omitempty" yaml:"ports,omitempty"`
}

type SQLTarget struct {
	Name string `json:"name" yaml:"name"`
	//+optional
	Privileges []SQLPrivileges `json:"privileges,omitempty" yaml:"privileges,omitempty"`
}

type SQLPrivileges struct {
	DatabaseName string `json:"databaseName" yaml:"databaseName"`
	//+optional
	Table string `json:"table" yaml:"table"`
	//+optional
	Operations []DatabaseOperation `json:"operations" yaml:"operations"`
}

type HTTPTarget struct {
	Path    string       `json:"path"`
	Methods []HTTPMethod `json:"methods" yaml:"methods"`
}

type KafkaTarget struct {
	Name string `json:"name" yaml:"name"`
	//+optional
	Topics []KafkaTopic `json:"topics,omitempty" yaml:"topics,omitempty"`
}

type KafkaTopic struct {
	Name       string           `json:"name" yaml:"name"`
	Operations []KafkaOperation `json:"operations" yaml:"operations"`
}

type ResolvedIPs struct {
	DNS string   `json:"dns,omitempty" yaml:"dns,omitempty"`
	IPs []string `json:"ips,omitempty" yaml:"ips,omitempty"`
}

type ReviewStatus string

const (
	ReviewStatusApproved ReviewStatus = "APPROVED"
	ReviewStatusPending  ReviewStatus = "PENDING"
	ReviewStatusDenied   ReviewStatus = "DENIED"
)

type AzureKeyVaultPolicy struct {
	//+optional
	CertificatePermissions []AzureKeyVaultCertificatePermission `json:"certificatePermissions,omitempty" yaml:"certificatePermissions,omitempty"`
	//+optional
	KeyPermissions []AzureKeyVaultKeyPermission `json:"keyPermissions,omitempty" yaml:"keyPermissions,omitempty"`
	//+optional
	SecretPermissions []AzureKeyVaultSecretPermission `json:"secretPermissions,omitempty" yaml:"secretPermissions,omitempty"`
	//+optional
	StoragePermissions []AzureKeyVaultStoragePermission `json:"storagePermissions,omitempty" yaml:"storagePermissions,omitempty"`
}

// IntentsStatus defines the observed state of ClientIntents
type IntentsStatus struct {
	// upToDate field reflects whether the client intents have successfully been applied
	// to the cluster to the state specified
	// Deprecated: Use ApprovedClientIntents.Status.UpToDate instead
	// +optional
	UpToDate bool `json:"upToDate"`

	// The last generation of the intents that was observed by the operator.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration"`

	// ResolvedIPs stores resolved IPs for a domain name - the network mapper populates it when DNS internetTarget is used
	// Deprecated: Use ApprovedClientIntents.Status.ResolvedIPs instead
	// +optional
	ResolvedIPs []ResolvedIPs `json:"resolvedIPs,omitempty" yaml:"resolvedIPs,omitempty"`

	// ReviewStatus stores the review status for these ClientIntents (whether they were approved, denied, or pending)
	ReviewStatus ReviewStatus `json:"reviewStatus,omitempty" yaml:"reviewStatus,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:storageversion

// ClientIntents is the Schema for the intents API
type ClientIntents struct {
	metav1.TypeMeta   `json:",inline" yaml:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`

	Spec   *IntentsSpec  `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status IntentsStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ClientIntentsList contains a list of ClientIntents
type ClientIntentsList struct {
	metav1.TypeMeta `json:",inline" yaml:",inline"`
	metav1.ListMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Items           []ClientIntents `json:"items" yaml:"items"`
}

func init() {
	SchemeBuilder.Register(&ClientIntents{}, &ClientIntentsList{})
}

func (in *ClientIntents) GetResourceGeneration() graphqlclient.IntentRequestResourceGeneration {
	return graphqlclient.IntentRequestResourceGeneration{
		ResourceId: string(in.UID),
		Generation: int(in.Generation),
	}
}

func (in *ClientIntents) GetWorkloadName() string {
	return in.Spec.Workload.Name
}

func (in *ClientIntents) GetTargetList() []Target {
	return in.Spec.Targets
}

func (in *ClientIntents) GetFilteredTargetList(intentTypes ...IntentType) []Target {
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

func (in *ClientIntents) GetClientKind() string {
	if in.Spec.Workload.Kind == "" {
		return serviceidentity.KindOtterizeLegacy
	}
	return in.Spec.Workload.Kind
}

func (in *ClientIntents) GetIntentsLabelMapping(requestNamespace string) map[string]string {
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

func (in *ClientIntents) GetDatabaseIntents() []Target {
	return in.GetFilteredTargetList(IntentTypeDatabase)
}

// GetTargetServerNamespace returns target namespace for intent if exists
// or the entire resource's namespace if the specific intent has no target namespace, as it's optional
func (in *Target) GetTargetServerNamespace(intentsObjNamespace string) string {
	nameWithNamespace := strings.Split(in.GetTargetServerNameAsWritten(), ".")
	if len(nameWithNamespace) == 1 {
		return intentsObjNamespace
	}
	// serverName.namespace --> "namespace"
	return nameWithNamespace[1]
}

func (in *Target) IsTargetServerKubernetesService() bool {
	return in.Service != nil || (in.Kubernetes != nil && in.Kubernetes.Kind == serviceidentity.KindService)
}

func (in *Target) GetIntentType() IntentType {
	if in.Kubernetes != nil && len(in.Kubernetes.HTTP) > 0 {
		return IntentTypeHTTP
	}
	if in.Service != nil && len(in.Service.HTTP) > 0 {
		return IntentTypeHTTP
	}
	if in.Kafka != nil {
		return IntentTypeKafka
	}
	if in.SQL != nil {
		return IntentTypeDatabase
	}
	if in.AWS != nil {
		return IntentTypeAWS
	}
	if in.GCP != nil {
		return IntentTypeGCP
	}
	if in.Azure != nil {
		return IntentTypeAzure
	}
	if in.Internet != nil {
		return IntentTypeInternet
	}
	return ""
}

func (in *Target) IsTargetTheKubernetesAPIServer(objectNamespace string) bool {
	return in.GetTargetServerName() == KubernetesAPIServerName &&
		in.GetTargetServerNamespace(objectNamespace) == KubernetesAPIServerNamespace
}

func (in *Target) IsTargetInCluster() bool {
	if in.Service != nil || in.Kubernetes != nil || in.Kafka != nil {
		return true
	}
	return false
}

func (in *Target) IsTargetOutOfCluster() bool {
	return !in.IsTargetInCluster()
}

// GetTargetServerName returns server's service name, without namespace, or the Kubernetes service without the `svc:` prefix
func (in *Target) GetTargetServerName() string {
	name := in.GetTargetServerNameAsWritten()
	if in.IsTargetOutOfCluster() {
		return name
	}
	nameWithNamespace := strings.Split(name, ".")
	return nameWithNamespace[0]
}

func (in *Target) GetTargetServerNameAsWritten() string {
	if in.Internet != nil {
		return OtterizeInternetTargetName
	}

	if in.Azure != nil {
		return in.Azure.Scope
	}

	if in.AWS != nil {
		return in.AWS.ARN
	}

	if in.GCP != nil {
		return in.GCP.Resource
	}

	if in.SQL != nil {
		return in.SQL.Name
	}

	if in.Kafka != nil {
		return in.Kafka.Name
	}

	if in.Service != nil {
		return in.Service.Name
	}

	if in.Kubernetes != nil {
		return in.Kubernetes.Name
	}

	panic("target server name not found")
}

func (in *Target) GetTargetServerKind() string {
	if in.Kubernetes != nil && in.Kubernetes.Kind != "" {
		return in.Kubernetes.Kind
	}
	if in.Service != nil {
		return serviceidentity.KindService
	}
	return serviceidentity.KindOtterizeLegacy
}

func (in *Target) GetServerFullyQualifiedName(intentsObjNamespace string) string {
	fullyQualifiedName := fmt.Sprintf("%s.%s", in.GetTargetServerName(), in.GetTargetServerNamespace(intentsObjNamespace))
	return fullyQualifiedName
}

func (in *Target) GetK8sServiceFullyQualifiedName(intentsObjNamespace string) (string, bool) {
	fullyQualifiedName := fmt.Sprintf("%s.%s", in.GetTargetServerName(), in.GetTargetServerNamespace(intentsObjNamespace))
	if in.IsTargetServerKubernetesService() {
		fullyQualifiedName = fmt.Sprintf("svc:%s", fullyQualifiedName)
		return fullyQualifiedName, true
	}
	return "", false
}

func (in *Target) typeAsGQLType() graphqlclient.IntentType {
	switch in.GetIntentType() {
	case IntentTypeHTTP:
		return graphqlclient.IntentTypeHttp
	case IntentTypeKafka:
		return graphqlclient.IntentTypeKafka
	case IntentTypeDatabase:
		return graphqlclient.IntentTypeDatabase
	case IntentTypeAWS:
		return graphqlclient.IntentTypeAws
	case IntentTypeGCP:
		return graphqlclient.IntentTypeGcp
	case IntentTypeAzure:
		return graphqlclient.IntentTypeAzure
	case IntentTypeInternet:
		return graphqlclient.IntentTypeInternet
	default:
		return ""
	}
}

func (in *ClientIntents) GetServersWithoutSidecar() (sets.Set[string], error) {
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

func (in *ClientIntents) IsServerMissingSidecar(intent Target) (bool, error) {
	serversSet, err := in.GetServersWithoutSidecar()
	if err != nil {
		return false, errors.Wrap(err)
	}
	identity := intent.ToServiceIdentity(in.Namespace)
	serverIdentity := identity.GetFormattedOtterizeIdentityWithoutKind()
	return serversSet.Has(serverIdentity), nil
}

func (in *ClientIntentsList) FormatAsOtterizeIntents(ctx context.Context, k8sClient client.Client) ([]*graphqlclient.IntentInput, error) {
	otterizeIntents := make([]*graphqlclient.IntentInput, 0)
	for _, clientIntents := range in.Items {
		for _, intent := range clientIntents.GetTargetList() {
			clientServiceIdentity := clientIntents.ToServiceIdentity()
			input, err := intent.ConvertToCloudFormat(ctx, k8sClient, clientServiceIdentity)
			if err != nil {
				return nil, errors.Wrap(err)
			}
			statusInput, ok, err := clientIntentsStatusToCloudFormat(clientIntents, intent)
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

func (in *ClientIntentsList) FormatAsOtterizeIntentsRequests(ctx context.Context, k8sClient client.Client) ([]*graphqlclient.IntentRequestInput, error) {
	otterizeIntents := make([]*graphqlclient.IntentRequestInput, 0)
	for _, clientIntents := range in.Items {
		for _, intent := range clientIntents.GetTargetList() {
			clientServiceIdentity := clientIntents.ToServiceIdentity()
			input, err := intent.ConvertToCloudFormat(ctx, k8sClient, clientServiceIdentity)
			if err != nil {
				return nil, errors.Wrap(err)
			}
			statusInput, ok, err := clientIntentsStatusToCloudFormat(clientIntents, intent)
			if err != nil {
				return nil, errors.Wrap(err)
			}

			input.Status = nil
			if ok {
				input.Status = statusInput
			}

			otterizeIntents = append(otterizeIntents, &graphqlclient.IntentRequestInput{
				Intent:             input,
				ResourceGeneration: clientIntents.GetResourceGeneration(),
			})
		}
	}

	return otterizeIntents, nil
}

func clientIntentsStatusToCloudFormat(clientIntents ClientIntents, intent Target) (*graphqlclient.IntentStatusInput, bool, error) {
	status := graphqlclient.IntentStatusInput{
		IstioStatus: &graphqlclient.IstioStatusInput{},
	}

	serviceAccountName, ok := clientIntents.Annotations[OtterizeClientServiceAccountAnnotation]
	if !ok {
		// Status is not set, nothing to do
		return nil, false, nil
	}

	status.IstioStatus.ServiceAccountName = toPtrOrNil(serviceAccountName)
	isSharedValue, ok := clientIntents.Annotations[OtterizeSharedServiceAccountAnnotation]
	isShared := false
	if ok {
		parsedIsShared, err := strconv.ParseBool(isSharedValue)
		if err != nil {
			return nil, false, errors.Errorf("failed to parse shared service account annotation for client intents %s", clientIntents.Name)
		}
		isShared = parsedIsShared
	}
	status.IstioStatus.IsServiceAccountShared = lo.ToPtr(isShared)

	clientMissingSidecarValue, ok := clientIntents.Annotations[OtterizeMissingSidecarAnnotation]
	if !ok {
		return nil, false, errors.Errorf("missing annotation missing sidecar for client intents %s", clientIntents.Name)
	}

	clientMissingSidecar, err := strconv.ParseBool(clientMissingSidecarValue)
	if err != nil {
		return nil, false, errors.Errorf("failed to parse missing sidecar annotation for client intents %s", clientIntents.Name)
	}
	status.IstioStatus.IsClientMissingSidecar = lo.ToPtr(clientMissingSidecar)
	isServerMissingSidecar, err := clientIntents.IsServerMissingSidecar(intent)
	if err != nil {
		return nil, false, errors.Wrap(err)
	}
	status.IstioStatus.IsServerMissingSidecar = lo.ToPtr(isServerMissingSidecar)
	return &status, true, nil
}

func toPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return lo.ToPtr(s)
}

func kafkaOperationK8sToCloud(op KafkaOperation) graphqlclient.KafkaOperation {
	switch op {
	case KafkaOperationAll:
		return graphqlclient.KafkaOperationAll
	case KafkaOperationConsume:
		return graphqlclient.KafkaOperationConsume
	case KafkaOperationProduce:
		return graphqlclient.KafkaOperationProduce
	case KafkaOperationCreate:
		return graphqlclient.KafkaOperationCreate
	case KafkaOperationAlter:
		return graphqlclient.KafkaOperationAlter
	case KafkaOperationDelete:
		return graphqlclient.KafkaOperationDelete
	case KafkaOperationDescribe:
		return graphqlclient.KafkaOperationDescribe
	case KafkaOperationClusterAction:
		return graphqlclient.KafkaOperationClusterAction
	case KafkaOperationDescribeConfigs:
		return graphqlclient.KafkaOperationDescribeConfigs
	case KafkaOperationAlterConfigs:
		return graphqlclient.KafkaOperationAlterConfigs
	case KafkaOperationIdempotentWrite:
		return graphqlclient.KafkaOperationIdempotentWrite
	default:
		logrus.Panic(fmt.Sprintf("Unknown KafkaOperation: %s", op))
		return "" // We won't reach here
	}
}

func databaseOperationToCloud(op DatabaseOperation) graphqlclient.DatabaseOperation {
	switch op {
	case DatabaseOperationAll:
		return graphqlclient.DatabaseOperationAll
	case DatabaseOperationDelete:
		return graphqlclient.DatabaseOperationDelete
	case DatabaseOperationInsert:
		return graphqlclient.DatabaseOperationInsert
	case DatabaseOperationSelect:
		return graphqlclient.DatabaseOperationSelect
	case DatabaseOperationUpdate:
		return graphqlclient.DatabaseOperationUpdate
	default:
		logrus.Panic(fmt.Sprintf("Unknown DatabaseOperation: %s", op))
		return ""
	}
}

func enumSliceToStrPtrSlice[T ~string](enumSlice []T) []*string {
	return lo.Map(enumSlice, func(s T, i int) *string {
		return lo.ToPtr(string(s))
	})
}

func (in *Target) GetHTTPResources() []HTTPTarget {
	if in.Kubernetes != nil && len(in.Kubernetes.HTTP) > 0 {
		return in.Kubernetes.HTTP
	}
	if in.Service != nil && len(in.Service.HTTP) > 0 {
		return in.Service.HTTP
	}
	return make([]HTTPTarget, 0)
}

func (in *Target) ConvertToCloudFormat(ctx context.Context, k8sClient client.Client, clientServiceIdentity serviceidentity.ServiceIdentity) (graphqlclient.IntentInput, error) {
	serverServiceIdentity := in.ToServiceIdentity(clientServiceIdentity.Namespace)
	var alias *graphqlclient.ServerAliasInput
	if in.IsTargetTheKubernetesAPIServer(clientServiceIdentity.Namespace) {
		alias = &graphqlclient.ServerAliasInput{
			Name: lo.ToPtr(serverServiceIdentity.GetNameAsServer()),
			Kind: lo.ToPtr(serviceidentity.KindService),
		}
	} else if in.IsTargetServerKubernetesService() {
		// alias should be the kubernetes service
		alias = &graphqlclient.ServerAliasInput{
			Name: lo.ToPtr(serverServiceIdentity.GetNameAsServer()),
			Kind: lo.ToPtr(serverServiceIdentity.Kind),
		}
		labelSelector, ok, err := ServiceIdentityToLabelsForWorkloadSelection(ctx, k8sClient, serverServiceIdentity)
		if err != nil {
			return graphqlclient.IntentInput{}, errors.Wrap(err)
		}
		if ok {
			podList := &corev1.PodList{}
			err := k8sClient.List(ctx, podList, client.InNamespace(serverServiceIdentity.Namespace), client.MatchingLabels(labelSelector))
			if err != nil {
				return graphqlclient.IntentInput{}, errors.Wrap(err)
			}
			if len(podList.Items) > 0 {
				si, err := podownerresolver.ResolvePodToServiceIdentity(ctx, k8sClient, &podList.Items[0])
				if err != nil {
					return graphqlclient.IntentInput{}, errors.Wrap(err)
				}
				// The server service Identity should be the workload (Deployment, statefulset, etc)
				serverServiceIdentity = si
			}
		}
	}

	intentInput := graphqlclient.IntentInput{
		ClientName:         lo.ToPtr(clientServiceIdentity.Name),
		ClientWorkloadKind: lo.Ternary(clientServiceIdentity.Kind != serviceidentity.KindOtterizeLegacy, lo.ToPtr(clientServiceIdentity.Kind), nil),
		ServerName:         lo.ToPtr(serverServiceIdentity.Name),
		ServerWorkloadKind: lo.Ternary(serverServiceIdentity.Kind != serviceidentity.KindOtterizeLegacy, lo.ToPtr(serverServiceIdentity.Kind), nil),
		Namespace:          lo.ToPtr(clientServiceIdentity.Namespace),
		ServerNamespace:    toPtrOrNil(in.GetTargetServerNamespace(clientServiceIdentity.Namespace)),
		ServerAlias:        alias,
	}
	if gqlType := in.typeAsGQLType(); gqlType != "" {
		intentInput.Type = lo.ToPtr(gqlType)
	}

	if in.Kafka != nil {
		otterizeTopics := lo.Map(in.Kafka.Topics, func(topic KafkaTopic, i int) *graphqlclient.KafkaConfigInput {
			return lo.ToPtr(graphqlclient.KafkaConfigInput{
				Name: lo.ToPtr(topic.Name),
				Operations: lo.Map(topic.Operations, func(op KafkaOperation, i int) *graphqlclient.KafkaOperation {
					operation := kafkaOperationK8sToCloud(op)
					return &operation
				}),
			})
		})
		intentInput.Topics = otterizeTopics
	}

	if in.typeAsGQLType() == graphqlclient.IntentTypeHttp {
		intentInput.Resources = lo.Map(in.GetHTTPResources(), intentsHTTPResourceToCloud)
	}

	if in.SQL != nil {
		intentInput.DatabaseResources = lo.Map(in.SQL.Privileges, func(resource SQLPrivileges, _ int) *graphqlclient.DatabaseConfigInput {
			databaseConfigInput := graphqlclient.DatabaseConfigInput{
				Table:  lo.ToPtr(resource.Table),
				Dbname: lo.ToPtr(resource.DatabaseName),
				Operations: lo.Map(resource.Operations, func(operation DatabaseOperation, _ int) *graphqlclient.DatabaseOperation {
					cloudOperation := databaseOperationToCloud(operation)
					return &cloudOperation
				}),
			}
			return &databaseConfigInput
		})
	}

	if in.Internet != nil {
		intentInput.Internet = &graphqlclient.InternetConfigInput{}
		if len(in.Internet.Domains) != 0 {
			intentInput.Internet.Domains = lo.ToSlicePtr(in.Internet.Domains)
		}
		if len(in.Internet.Ips) != 0 {
			intentInput.Internet.Ips = lo.ToSlicePtr(in.Internet.Ips)
		}
		if in.Internet.Ports != nil && len(in.Internet.Ports) != 0 {
			intentInput.Internet.Ports = lo.ToSlicePtr(in.Internet.Ports)
		}
	}

	if in.AWS != nil {
		intentInput.AwsActions = lo.ToSlicePtr(in.AWS.Actions)
	}

	if in.Azure != nil {
		intentInput.AzureRoles = lo.ToSlicePtr(in.Azure.Roles)
		intentInput.AzureActions = lo.Map(in.Azure.Actions, func(action AzureAction, _ int) *string {
			return lo.ToPtr(string(action))
		})
		intentInput.AzureDataActions = lo.Map(in.Azure.DataActions, func(action AzureDataAction, _ int) *string {
			return lo.ToPtr(string(action))
		})

		if in.Azure.KeyVaultPolicy != nil {
			intentInput.AzureKeyVaultPolicy = &graphqlclient.AzureKeyVaultPolicyInput{
				CertificatePermissions: enumSliceToStrPtrSlice(in.Azure.KeyVaultPolicy.CertificatePermissions),
				KeyPermissions:         enumSliceToStrPtrSlice(in.Azure.KeyVaultPolicy.KeyPermissions),
				SecretPermissions:      enumSliceToStrPtrSlice(in.Azure.KeyVaultPolicy.SecretPermissions),
				StoragePermissions:     enumSliceToStrPtrSlice(in.Azure.KeyVaultPolicy.StoragePermissions),
			}
		}
	}

	if in.GCP != nil {
		intentInput.GcpPermissions = lo.ToSlicePtr(in.GCP.Permissions)
	}

	return intentInput, nil
}

func intentsHTTPResourceToCloud(resource HTTPTarget, _ int) *graphqlclient.HTTPConfigInput {
	methods := lo.Map(resource.Methods, func(method HTTPMethod, _ int) *graphqlclient.HTTPMethod {
		return lo.ToPtr(graphqlclient.HTTPMethod(method))
	})

	httpConfig := graphqlclient.HTTPConfigInput{
		Path:    lo.ToPtr(resource.Path),
		Methods: methods,
	}

	return &httpConfig
}

func (in *ClientIntents) HasKafkaTypeInCallList() bool {
	for _, intent := range in.GetTargetList() {
		if intent.Kafka != nil {
			return true
		}
	}
	return false
}

func (in *ClientIntents) HasDatabaseTypeInCallList() bool {
	for _, intent := range in.GetTargetList() {
		if intent.SQL != nil {
			return true
		}
	}
	return false
}
