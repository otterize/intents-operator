// Code generated by github.com/Khan/genqlient, DO NOT EDIT.

package graphqlclient

import (
	"context"

	"github.com/Khan/genqlient/graphql"
)

type AzureKeyVaultPolicyInput struct {
	CertificatePermissions []*string `json:"certificatePermissions"`
	KeyPermissions         []*string `json:"keyPermissions"`
	SecretPermissions      []*string `json:"secretPermissions"`
	StoragePermissions     []*string `json:"storagePermissions"`
}

// GetCertificatePermissions returns AzureKeyVaultPolicyInput.CertificatePermissions, and is useful for accessing the field via an interface.
func (v *AzureKeyVaultPolicyInput) GetCertificatePermissions() []*string {
	return v.CertificatePermissions
}

// GetKeyPermissions returns AzureKeyVaultPolicyInput.KeyPermissions, and is useful for accessing the field via an interface.
func (v *AzureKeyVaultPolicyInput) GetKeyPermissions() []*string { return v.KeyPermissions }

// GetSecretPermissions returns AzureKeyVaultPolicyInput.SecretPermissions, and is useful for accessing the field via an interface.
func (v *AzureKeyVaultPolicyInput) GetSecretPermissions() []*string { return v.SecretPermissions }

// GetStoragePermissions returns AzureKeyVaultPolicyInput.StoragePermissions, and is useful for accessing the field via an interface.
func (v *AzureKeyVaultPolicyInput) GetStoragePermissions() []*string { return v.StoragePermissions }

type ComponentType string

const (
	ComponentTypeIntentsOperator     ComponentType = "INTENTS_OPERATOR"
	ComponentTypeCredentialsOperator ComponentType = "CREDENTIALS_OPERATOR"
	ComponentTypeNetworkMapper       ComponentType = "NETWORK_MAPPER"
)

type DBPermissionChange string

const (
	DBPermissionChangeApply  DBPermissionChange = "APPLY"
	DBPermissionChangeDelete DBPermissionChange = "DELETE"
)

type DatabaseConfigInput struct {
	Dbname     *string              `json:"dbname"`
	Table      *string              `json:"table"`
	Operations []*DatabaseOperation `json:"operations"`
}

// GetDbname returns DatabaseConfigInput.Dbname, and is useful for accessing the field via an interface.
func (v *DatabaseConfigInput) GetDbname() *string { return v.Dbname }

// GetTable returns DatabaseConfigInput.Table, and is useful for accessing the field via an interface.
func (v *DatabaseConfigInput) GetTable() *string { return v.Table }

// GetOperations returns DatabaseConfigInput.Operations, and is useful for accessing the field via an interface.
func (v *DatabaseConfigInput) GetOperations() []*DatabaseOperation { return v.Operations }

type DatabaseOperation string

const (
	DatabaseOperationAll    DatabaseOperation = "ALL"
	DatabaseOperationSelect DatabaseOperation = "SELECT"
	DatabaseOperationInsert DatabaseOperation = "INSERT"
	DatabaseOperationUpdate DatabaseOperation = "UPDATE"
	DatabaseOperationDelete DatabaseOperation = "DELETE"
)

type HTTPConfigInput struct {
	Path    *string       `json:"path"`
	Methods []*HTTPMethod `json:"methods"`
}

// GetPath returns HTTPConfigInput.Path, and is useful for accessing the field via an interface.
func (v *HTTPConfigInput) GetPath() *string { return v.Path }

// GetMethods returns HTTPConfigInput.Methods, and is useful for accessing the field via an interface.
func (v *HTTPConfigInput) GetMethods() []*HTTPMethod { return v.Methods }

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
	HTTPMethodAll     HTTPMethod = "ALL"
)

// HandleDatabaseIntentsResponse is returned by HandleDatabaseIntents on success.
type HandleDatabaseIntentsResponse struct {
	HandleDatabaseIntents bool `json:"handleDatabaseIntents"`
}

// GetHandleDatabaseIntents returns HandleDatabaseIntentsResponse.HandleDatabaseIntents, and is useful for accessing the field via an interface.
func (v *HandleDatabaseIntentsResponse) GetHandleDatabaseIntents() bool {
	return v.HandleDatabaseIntents
}

type IntentInput struct {
	Namespace           *string                   `json:"namespace"`
	ClientName          *string                   `json:"clientName"`
	ServerName          *string                   `json:"serverName"`
	ServerNamespace     *string                   `json:"serverNamespace"`
	Type                *IntentType               `json:"type"`
	Topics              []*KafkaConfigInput       `json:"topics"`
	Resources           []*HTTPConfigInput        `json:"resources"`
	DatabaseResources   []*DatabaseConfigInput    `json:"databaseResources"`
	AwsActions          []*string                 `json:"awsActions"`
	AzureRoles          []*string                 `json:"azureRoles"`
	AzureKeyVaultPolicy *AzureKeyVaultPolicyInput `json:"azureKeyVaultPolicy"`
	GcpPermissions      []*string                 `json:"gcpPermissions"`
	Internet            *InternetConfigInput      `json:"internet"`
	Status              *IntentStatusInput        `json:"status"`
}

// GetNamespace returns IntentInput.Namespace, and is useful for accessing the field via an interface.
func (v *IntentInput) GetNamespace() *string { return v.Namespace }

// GetClientName returns IntentInput.ClientName, and is useful for accessing the field via an interface.
func (v *IntentInput) GetClientName() *string { return v.ClientName }

// GetServerName returns IntentInput.ServerName, and is useful for accessing the field via an interface.
func (v *IntentInput) GetServerName() *string { return v.ServerName }

// GetServerNamespace returns IntentInput.ServerNamespace, and is useful for accessing the field via an interface.
func (v *IntentInput) GetServerNamespace() *string { return v.ServerNamespace }

// GetType returns IntentInput.Type, and is useful for accessing the field via an interface.
func (v *IntentInput) GetType() *IntentType { return v.Type }

// GetTopics returns IntentInput.Topics, and is useful for accessing the field via an interface.
func (v *IntentInput) GetTopics() []*KafkaConfigInput { return v.Topics }

// GetResources returns IntentInput.Resources, and is useful for accessing the field via an interface.
func (v *IntentInput) GetResources() []*HTTPConfigInput { return v.Resources }

// GetDatabaseResources returns IntentInput.DatabaseResources, and is useful for accessing the field via an interface.
func (v *IntentInput) GetDatabaseResources() []*DatabaseConfigInput { return v.DatabaseResources }

// GetAwsActions returns IntentInput.AwsActions, and is useful for accessing the field via an interface.
func (v *IntentInput) GetAwsActions() []*string { return v.AwsActions }

// GetAzureRoles returns IntentInput.AzureRoles, and is useful for accessing the field via an interface.
func (v *IntentInput) GetAzureRoles() []*string { return v.AzureRoles }

// GetAzureKeyVaultPolicy returns IntentInput.AzureKeyVaultPolicy, and is useful for accessing the field via an interface.
func (v *IntentInput) GetAzureKeyVaultPolicy() *AzureKeyVaultPolicyInput {
	return v.AzureKeyVaultPolicy
}

// GetGcpPermissions returns IntentInput.GcpPermissions, and is useful for accessing the field via an interface.
func (v *IntentInput) GetGcpPermissions() []*string { return v.GcpPermissions }

// GetInternet returns IntentInput.Internet, and is useful for accessing the field via an interface.
func (v *IntentInput) GetInternet() *InternetConfigInput { return v.Internet }

// GetStatus returns IntentInput.Status, and is useful for accessing the field via an interface.
func (v *IntentInput) GetStatus() *IntentStatusInput { return v.Status }

type IntentStatusInput struct {
	IstioStatus *IstioStatusInput `json:"istioStatus"`
}

// GetIstioStatus returns IntentStatusInput.IstioStatus, and is useful for accessing the field via an interface.
func (v *IntentStatusInput) GetIstioStatus() *IstioStatusInput { return v.IstioStatus }

type IntentType string

const (
	IntentTypeHttp     IntentType = "HTTP"
	IntentTypeKafka    IntentType = "KAFKA"
	IntentTypeDatabase IntentType = "DATABASE"
	IntentTypeAws      IntentType = "AWS"
	IntentTypeGcp      IntentType = "GCP"
	IntentTypeAzure    IntentType = "AZURE"
	IntentTypeS3       IntentType = "S3"
	IntentTypeInternet IntentType = "INTERNET"
)

type IntentsOperatorConfigurationInput struct {
	GlobalEnforcementEnabled              bool `json:"globalEnforcementEnabled"`
	NetworkPolicyEnforcementEnabled       bool `json:"networkPolicyEnforcementEnabled"`
	KafkaACLEnforcementEnabled            bool `json:"kafkaACLEnforcementEnabled"`
	IstioPolicyEnforcementEnabled         bool `json:"istioPolicyEnforcementEnabled"`
	ProtectedServicesEnabled              bool `json:"protectedServicesEnabled"`
	EgressNetworkPolicyEnforcementEnabled bool `json:"egressNetworkPolicyEnforcementEnabled"`
	AwsIAMPolicyEnforcementEnabled        bool `json:"awsIAMPolicyEnforcementEnabled"`
	GcpIAMPolicyEnforcementEnabled        bool `json:"gcpIAMPolicyEnforcementEnabled"`
	AzureIAMPolicyEnforcementEnabled      bool `json:"azureIAMPolicyEnforcementEnabled"`
	DatabaseEnforcementEnabled            bool `json:"databaseEnforcementEnabled"`
}

// GetGlobalEnforcementEnabled returns IntentsOperatorConfigurationInput.GlobalEnforcementEnabled, and is useful for accessing the field via an interface.
func (v *IntentsOperatorConfigurationInput) GetGlobalEnforcementEnabled() bool {
	return v.GlobalEnforcementEnabled
}

// GetNetworkPolicyEnforcementEnabled returns IntentsOperatorConfigurationInput.NetworkPolicyEnforcementEnabled, and is useful for accessing the field via an interface.
func (v *IntentsOperatorConfigurationInput) GetNetworkPolicyEnforcementEnabled() bool {
	return v.NetworkPolicyEnforcementEnabled
}

// GetKafkaACLEnforcementEnabled returns IntentsOperatorConfigurationInput.KafkaACLEnforcementEnabled, and is useful for accessing the field via an interface.
func (v *IntentsOperatorConfigurationInput) GetKafkaACLEnforcementEnabled() bool {
	return v.KafkaACLEnforcementEnabled
}

// GetIstioPolicyEnforcementEnabled returns IntentsOperatorConfigurationInput.IstioPolicyEnforcementEnabled, and is useful for accessing the field via an interface.
func (v *IntentsOperatorConfigurationInput) GetIstioPolicyEnforcementEnabled() bool {
	return v.IstioPolicyEnforcementEnabled
}

// GetProtectedServicesEnabled returns IntentsOperatorConfigurationInput.ProtectedServicesEnabled, and is useful for accessing the field via an interface.
func (v *IntentsOperatorConfigurationInput) GetProtectedServicesEnabled() bool {
	return v.ProtectedServicesEnabled
}

// GetEgressNetworkPolicyEnforcementEnabled returns IntentsOperatorConfigurationInput.EgressNetworkPolicyEnforcementEnabled, and is useful for accessing the field via an interface.
func (v *IntentsOperatorConfigurationInput) GetEgressNetworkPolicyEnforcementEnabled() bool {
	return v.EgressNetworkPolicyEnforcementEnabled
}

// GetAwsIAMPolicyEnforcementEnabled returns IntentsOperatorConfigurationInput.AwsIAMPolicyEnforcementEnabled, and is useful for accessing the field via an interface.
func (v *IntentsOperatorConfigurationInput) GetAwsIAMPolicyEnforcementEnabled() bool {
	return v.AwsIAMPolicyEnforcementEnabled
}

// GetGcpIAMPolicyEnforcementEnabled returns IntentsOperatorConfigurationInput.GcpIAMPolicyEnforcementEnabled, and is useful for accessing the field via an interface.
func (v *IntentsOperatorConfigurationInput) GetGcpIAMPolicyEnforcementEnabled() bool {
	return v.GcpIAMPolicyEnforcementEnabled
}

// GetAzureIAMPolicyEnforcementEnabled returns IntentsOperatorConfigurationInput.AzureIAMPolicyEnforcementEnabled, and is useful for accessing the field via an interface.
func (v *IntentsOperatorConfigurationInput) GetAzureIAMPolicyEnforcementEnabled() bool {
	return v.AzureIAMPolicyEnforcementEnabled
}

// GetDatabaseEnforcementEnabled returns IntentsOperatorConfigurationInput.DatabaseEnforcementEnabled, and is useful for accessing the field via an interface.
func (v *IntentsOperatorConfigurationInput) GetDatabaseEnforcementEnabled() bool {
	return v.DatabaseEnforcementEnabled
}

type InternetConfigInput struct {
	Domains []*string `json:"domains"`
	Ips     []*string `json:"ips"`
	Ports   []*int    `json:"ports"`
}

// GetDomains returns InternetConfigInput.Domains, and is useful for accessing the field via an interface.
func (v *InternetConfigInput) GetDomains() []*string { return v.Domains }

// GetIps returns InternetConfigInput.Ips, and is useful for accessing the field via an interface.
func (v *InternetConfigInput) GetIps() []*string { return v.Ips }

// GetPorts returns InternetConfigInput.Ports, and is useful for accessing the field via an interface.
func (v *InternetConfigInput) GetPorts() []*int { return v.Ports }

type IstioStatusInput struct {
	ServiceAccountName     *string `json:"serviceAccountName"`
	IsServiceAccountShared *bool   `json:"isServiceAccountShared"`
	IsServerMissingSidecar *bool   `json:"isServerMissingSidecar"`
	IsClientMissingSidecar *bool   `json:"isClientMissingSidecar"`
}

// GetServiceAccountName returns IstioStatusInput.ServiceAccountName, and is useful for accessing the field via an interface.
func (v *IstioStatusInput) GetServiceAccountName() *string { return v.ServiceAccountName }

// GetIsServiceAccountShared returns IstioStatusInput.IsServiceAccountShared, and is useful for accessing the field via an interface.
func (v *IstioStatusInput) GetIsServiceAccountShared() *bool { return v.IsServiceAccountShared }

// GetIsServerMissingSidecar returns IstioStatusInput.IsServerMissingSidecar, and is useful for accessing the field via an interface.
func (v *IstioStatusInput) GetIsServerMissingSidecar() *bool { return v.IsServerMissingSidecar }

// GetIsClientMissingSidecar returns IstioStatusInput.IsClientMissingSidecar, and is useful for accessing the field via an interface.
func (v *IstioStatusInput) GetIsClientMissingSidecar() *bool { return v.IsClientMissingSidecar }

type KafkaConfigInput struct {
	Name       *string           `json:"name"`
	Operations []*KafkaOperation `json:"operations"`
}

// GetName returns KafkaConfigInput.Name, and is useful for accessing the field via an interface.
func (v *KafkaConfigInput) GetName() *string { return v.Name }

// GetOperations returns KafkaConfigInput.Operations, and is useful for accessing the field via an interface.
func (v *KafkaConfigInput) GetOperations() []*KafkaOperation { return v.Operations }

type KafkaOperation string

const (
	KafkaOperationAll             KafkaOperation = "ALL"
	KafkaOperationConsume         KafkaOperation = "CONSUME"
	KafkaOperationProduce         KafkaOperation = "PRODUCE"
	KafkaOperationCreate          KafkaOperation = "CREATE"
	KafkaOperationAlter           KafkaOperation = "ALTER"
	KafkaOperationDelete          KafkaOperation = "DELETE"
	KafkaOperationDescribe        KafkaOperation = "DESCRIBE"
	KafkaOperationClusterAction   KafkaOperation = "CLUSTER_ACTION"
	KafkaOperationDescribeConfigs KafkaOperation = "DESCRIBE_CONFIGS"
	KafkaOperationAlterConfigs    KafkaOperation = "ALTER_CONFIGS"
	KafkaOperationIdempotentWrite KafkaOperation = "IDEMPOTENT_WRITE"
)

type KafkaServerConfigInput struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Address   string            `json:"address"`
	Topics    []KafkaTopicInput `json:"topics"`
}

// GetName returns KafkaServerConfigInput.Name, and is useful for accessing the field via an interface.
func (v *KafkaServerConfigInput) GetName() string { return v.Name }

// GetNamespace returns KafkaServerConfigInput.Namespace, and is useful for accessing the field via an interface.
func (v *KafkaServerConfigInput) GetNamespace() string { return v.Namespace }

// GetAddress returns KafkaServerConfigInput.Address, and is useful for accessing the field via an interface.
func (v *KafkaServerConfigInput) GetAddress() string { return v.Address }

// GetTopics returns KafkaServerConfigInput.Topics, and is useful for accessing the field via an interface.
func (v *KafkaServerConfigInput) GetTopics() []KafkaTopicInput { return v.Topics }

type KafkaTopicInput struct {
	ClientIdentityRequired bool              `json:"clientIdentityRequired"`
	IntentsRequired        bool              `json:"intentsRequired"`
	Pattern                KafkaTopicPattern `json:"pattern"`
	Topic                  string            `json:"topic"`
}

// GetClientIdentityRequired returns KafkaTopicInput.ClientIdentityRequired, and is useful for accessing the field via an interface.
func (v *KafkaTopicInput) GetClientIdentityRequired() bool { return v.ClientIdentityRequired }

// GetIntentsRequired returns KafkaTopicInput.IntentsRequired, and is useful for accessing the field via an interface.
func (v *KafkaTopicInput) GetIntentsRequired() bool { return v.IntentsRequired }

// GetPattern returns KafkaTopicInput.Pattern, and is useful for accessing the field via an interface.
func (v *KafkaTopicInput) GetPattern() KafkaTopicPattern { return v.Pattern }

// GetTopic returns KafkaTopicInput.Topic, and is useful for accessing the field via an interface.
func (v *KafkaTopicInput) GetTopic() string { return v.Topic }

type KafkaTopicPattern string

const (
	KafkaTopicPatternLiteral KafkaTopicPattern = "LITERAL"
	KafkaTopicPatternPrefix  KafkaTopicPattern = "PREFIX"
)

type NetworkPolicyInput struct {
	Namespace                    string `json:"namespace"`
	Name                         string `json:"name"`
	ServerName                   string `json:"serverName"`
	ExternalNetworkTrafficPolicy bool   `json:"externalNetworkTrafficPolicy"`
}

// GetNamespace returns NetworkPolicyInput.Namespace, and is useful for accessing the field via an interface.
func (v *NetworkPolicyInput) GetNamespace() string { return v.Namespace }

// GetName returns NetworkPolicyInput.Name, and is useful for accessing the field via an interface.
func (v *NetworkPolicyInput) GetName() string { return v.Name }

// GetServerName returns NetworkPolicyInput.ServerName, and is useful for accessing the field via an interface.
func (v *NetworkPolicyInput) GetServerName() string { return v.ServerName }

// GetExternalNetworkTrafficPolicy returns NetworkPolicyInput.ExternalNetworkTrafficPolicy, and is useful for accessing the field via an interface.
func (v *NetworkPolicyInput) GetExternalNetworkTrafficPolicy() bool {
	return v.ExternalNetworkTrafficPolicy
}

type ProtectedServiceInput struct {
	Name string `json:"name"`
}

// GetName returns ProtectedServiceInput.Name, and is useful for accessing the field via an interface.
func (v *ProtectedServiceInput) GetName() string { return v.Name }

// ReportAppliedKubernetesIntentsResponse is returned by ReportAppliedKubernetesIntents on success.
type ReportAppliedKubernetesIntentsResponse struct {
	ReportAppliedKubernetesIntents *bool `json:"reportAppliedKubernetesIntents"`
}

// GetReportAppliedKubernetesIntents returns ReportAppliedKubernetesIntentsResponse.ReportAppliedKubernetesIntents, and is useful for accessing the field via an interface.
func (v *ReportAppliedKubernetesIntentsResponse) GetReportAppliedKubernetesIntents() *bool {
	return v.ReportAppliedKubernetesIntents
}

// ReportComponentStatusResponse is returned by ReportComponentStatus on success.
type ReportComponentStatusResponse struct {
	// Report integration components status
	ReportIntegrationComponentStatus bool `json:"reportIntegrationComponentStatus"`
}

// GetReportIntegrationComponentStatus returns ReportComponentStatusResponse.ReportIntegrationComponentStatus, and is useful for accessing the field via an interface.
func (v *ReportComponentStatusResponse) GetReportIntegrationComponentStatus() bool {
	return v.ReportIntegrationComponentStatus
}

// ReportIntentsOperatorConfigurationResponse is returned by ReportIntentsOperatorConfiguration on success.
type ReportIntentsOperatorConfigurationResponse struct {
	ReportIntentsOperatorConfiguration bool `json:"reportIntentsOperatorConfiguration"`
}

// GetReportIntentsOperatorConfiguration returns ReportIntentsOperatorConfigurationResponse.ReportIntentsOperatorConfiguration, and is useful for accessing the field via an interface.
func (v *ReportIntentsOperatorConfigurationResponse) GetReportIntentsOperatorConfiguration() bool {
	return v.ReportIntentsOperatorConfiguration
}

// ReportKafkaServerConfigResponse is returned by ReportKafkaServerConfig on success.
type ReportKafkaServerConfigResponse struct {
	ReportKafkaServerConfigs bool `json:"reportKafkaServerConfigs"`
}

// GetReportKafkaServerConfigs returns ReportKafkaServerConfigResponse.ReportKafkaServerConfigs, and is useful for accessing the field via an interface.
func (v *ReportKafkaServerConfigResponse) GetReportKafkaServerConfigs() bool {
	return v.ReportKafkaServerConfigs
}

// ReportNetworkPoliciesResponse is returned by ReportNetworkPolicies on success.
type ReportNetworkPoliciesResponse struct {
	ReportNetworkPolicies bool `json:"reportNetworkPolicies"`
}

// GetReportNetworkPolicies returns ReportNetworkPoliciesResponse.ReportNetworkPolicies, and is useful for accessing the field via an interface.
func (v *ReportNetworkPoliciesResponse) GetReportNetworkPolicies() bool {
	return v.ReportNetworkPolicies
}

// ReportProtectedServicesSnapshotResponse is returned by ReportProtectedServicesSnapshot on success.
type ReportProtectedServicesSnapshotResponse struct {
	ReportProtectedServicesSnapshot bool `json:"reportProtectedServicesSnapshot"`
}

// GetReportProtectedServicesSnapshot returns ReportProtectedServicesSnapshotResponse.ReportProtectedServicesSnapshot, and is useful for accessing the field via an interface.
func (v *ReportProtectedServicesSnapshotResponse) GetReportProtectedServicesSnapshot() bool {
	return v.ReportProtectedServicesSnapshot
}

type UserErrorType string

const (
	UserErrorTypeUnauthenticated     UserErrorType = "UNAUTHENTICATED"
	UserErrorTypeNotFound            UserErrorType = "NOT_FOUND"
	UserErrorTypeInternalServerError UserErrorType = "INTERNAL_SERVER_ERROR"
	UserErrorTypeBadRequest          UserErrorType = "BAD_REQUEST"
	UserErrorTypeForbidden           UserErrorType = "FORBIDDEN"
	UserErrorTypeConflict            UserErrorType = "CONFLICT"
	UserErrorTypeBadUserInput        UserErrorType = "BAD_USER_INPUT"
	UserErrorTypeAppliedIntentsError UserErrorType = "APPLIED_INTENTS_ERROR"
)

// __HandleDatabaseIntentsInput is used internally by genqlient
type __HandleDatabaseIntentsInput struct {
	Intents []IntentInput      `json:"intents"`
	Action  DBPermissionChange `json:"action"`
}

// GetIntents returns __HandleDatabaseIntentsInput.Intents, and is useful for accessing the field via an interface.
func (v *__HandleDatabaseIntentsInput) GetIntents() []IntentInput { return v.Intents }

// GetAction returns __HandleDatabaseIntentsInput.Action, and is useful for accessing the field via an interface.
func (v *__HandleDatabaseIntentsInput) GetAction() DBPermissionChange { return v.Action }

// __ReportAppliedKubernetesIntentsInput is used internally by genqlient
type __ReportAppliedKubernetesIntentsInput struct {
	Namespace *string        `json:"namespace"`
	Intents   []*IntentInput `json:"intents"`
}

// GetNamespace returns __ReportAppliedKubernetesIntentsInput.Namespace, and is useful for accessing the field via an interface.
func (v *__ReportAppliedKubernetesIntentsInput) GetNamespace() *string { return v.Namespace }

// GetIntents returns __ReportAppliedKubernetesIntentsInput.Intents, and is useful for accessing the field via an interface.
func (v *__ReportAppliedKubernetesIntentsInput) GetIntents() []*IntentInput { return v.Intents }

// __ReportComponentStatusInput is used internally by genqlient
type __ReportComponentStatusInput struct {
	Component ComponentType `json:"component"`
}

// GetComponent returns __ReportComponentStatusInput.Component, and is useful for accessing the field via an interface.
func (v *__ReportComponentStatusInput) GetComponent() ComponentType { return v.Component }

// __ReportIntentsOperatorConfigurationInput is used internally by genqlient
type __ReportIntentsOperatorConfigurationInput struct {
	Configuration IntentsOperatorConfigurationInput `json:"configuration"`
}

// GetConfiguration returns __ReportIntentsOperatorConfigurationInput.Configuration, and is useful for accessing the field via an interface.
func (v *__ReportIntentsOperatorConfigurationInput) GetConfiguration() IntentsOperatorConfigurationInput {
	return v.Configuration
}

// __ReportKafkaServerConfigInput is used internally by genqlient
type __ReportKafkaServerConfigInput struct {
	Namespace          string                   `json:"namespace"`
	KafkaServerConfigs []KafkaServerConfigInput `json:"kafkaServerConfigs"`
}

// GetNamespace returns __ReportKafkaServerConfigInput.Namespace, and is useful for accessing the field via an interface.
func (v *__ReportKafkaServerConfigInput) GetNamespace() string { return v.Namespace }

// GetKafkaServerConfigs returns __ReportKafkaServerConfigInput.KafkaServerConfigs, and is useful for accessing the field via an interface.
func (v *__ReportKafkaServerConfigInput) GetKafkaServerConfigs() []KafkaServerConfigInput {
	return v.KafkaServerConfigs
}

// __ReportNetworkPoliciesInput is used internally by genqlient
type __ReportNetworkPoliciesInput struct {
	Namespace string               `json:"namespace"`
	Policies  []NetworkPolicyInput `json:"policies"`
}

// GetNamespace returns __ReportNetworkPoliciesInput.Namespace, and is useful for accessing the field via an interface.
func (v *__ReportNetworkPoliciesInput) GetNamespace() string { return v.Namespace }

// GetPolicies returns __ReportNetworkPoliciesInput.Policies, and is useful for accessing the field via an interface.
func (v *__ReportNetworkPoliciesInput) GetPolicies() []NetworkPolicyInput { return v.Policies }

// __ReportProtectedServicesSnapshotInput is used internally by genqlient
type __ReportProtectedServicesSnapshotInput struct {
	Namespace string                  `json:"namespace"`
	Services  []ProtectedServiceInput `json:"services"`
}

// GetNamespace returns __ReportProtectedServicesSnapshotInput.Namespace, and is useful for accessing the field via an interface.
func (v *__ReportProtectedServicesSnapshotInput) GetNamespace() string { return v.Namespace }

// GetServices returns __ReportProtectedServicesSnapshotInput.Services, and is useful for accessing the field via an interface.
func (v *__ReportProtectedServicesSnapshotInput) GetServices() []ProtectedServiceInput {
	return v.Services
}

// dummyResponse is returned by dummy on success.
type dummyResponse struct {
	DummyError UserErrorType `json:"dummyError"`
}

// GetDummyError returns dummyResponse.DummyError, and is useful for accessing the field via an interface.
func (v *dummyResponse) GetDummyError() UserErrorType { return v.DummyError }

// The query or mutation executed by HandleDatabaseIntents.
const HandleDatabaseIntents_Operation = `
mutation HandleDatabaseIntents ($intents: [IntentInput!]!, $action: DBPermissionChange!) {
	handleDatabaseIntents(intents: $intents, action: $action)
}
`

func HandleDatabaseIntents(
	ctx_ context.Context,
	client_ graphql.Client,
	intents []IntentInput,
	action DBPermissionChange,
) (*HandleDatabaseIntentsResponse, error) {
	req_ := &graphql.Request{
		OpName: "HandleDatabaseIntents",
		Query:  HandleDatabaseIntents_Operation,
		Variables: &__HandleDatabaseIntentsInput{
			Intents: intents,
			Action:  action,
		},
	}
	var err_ error

	var data_ HandleDatabaseIntentsResponse
	resp_ := &graphql.Response{Data: &data_}

	err_ = client_.MakeRequest(
		ctx_,
		req_,
		resp_,
	)

	return &data_, err_
}

// The query or mutation executed by ReportAppliedKubernetesIntents.
const ReportAppliedKubernetesIntents_Operation = `
mutation ReportAppliedKubernetesIntents ($namespace: String!, $intents: [IntentInput!]!) {
	reportAppliedKubernetesIntents(namespace: $namespace, intents: $intents)
}
`

func ReportAppliedKubernetesIntents(
	ctx_ context.Context,
	client_ graphql.Client,
	namespace *string,
	intents []*IntentInput,
) (*ReportAppliedKubernetesIntentsResponse, error) {
	req_ := &graphql.Request{
		OpName: "ReportAppliedKubernetesIntents",
		Query:  ReportAppliedKubernetesIntents_Operation,
		Variables: &__ReportAppliedKubernetesIntentsInput{
			Namespace: namespace,
			Intents:   intents,
		},
	}
	var err_ error

	var data_ ReportAppliedKubernetesIntentsResponse
	resp_ := &graphql.Response{Data: &data_}

	err_ = client_.MakeRequest(
		ctx_,
		req_,
		resp_,
	)

	return &data_, err_
}

// The query or mutation executed by ReportComponentStatus.
const ReportComponentStatus_Operation = `
mutation ReportComponentStatus ($component: ComponentType!) {
	reportIntegrationComponentStatus(component: $component)
}
`

func ReportComponentStatus(
	ctx_ context.Context,
	client_ graphql.Client,
	component ComponentType,
) (*ReportComponentStatusResponse, error) {
	req_ := &graphql.Request{
		OpName: "ReportComponentStatus",
		Query:  ReportComponentStatus_Operation,
		Variables: &__ReportComponentStatusInput{
			Component: component,
		},
	}
	var err_ error

	var data_ ReportComponentStatusResponse
	resp_ := &graphql.Response{Data: &data_}

	err_ = client_.MakeRequest(
		ctx_,
		req_,
		resp_,
	)

	return &data_, err_
}

// The query or mutation executed by ReportIntentsOperatorConfiguration.
const ReportIntentsOperatorConfiguration_Operation = `
mutation ReportIntentsOperatorConfiguration ($configuration: IntentsOperatorConfigurationInput!) {
	reportIntentsOperatorConfiguration(configuration: $configuration)
}
`

func ReportIntentsOperatorConfiguration(
	ctx_ context.Context,
	client_ graphql.Client,
	configuration IntentsOperatorConfigurationInput,
) (*ReportIntentsOperatorConfigurationResponse, error) {
	req_ := &graphql.Request{
		OpName: "ReportIntentsOperatorConfiguration",
		Query:  ReportIntentsOperatorConfiguration_Operation,
		Variables: &__ReportIntentsOperatorConfigurationInput{
			Configuration: configuration,
		},
	}
	var err_ error

	var data_ ReportIntentsOperatorConfigurationResponse
	resp_ := &graphql.Response{Data: &data_}

	err_ = client_.MakeRequest(
		ctx_,
		req_,
		resp_,
	)

	return &data_, err_
}

// The query or mutation executed by ReportKafkaServerConfig.
const ReportKafkaServerConfig_Operation = `
mutation ReportKafkaServerConfig ($namespace: String!, $kafkaServerConfigs: [KafkaServerConfigInput!]!) {
	reportKafkaServerConfigs(namespace: $namespace, serverConfigs: $kafkaServerConfigs)
}
`

func ReportKafkaServerConfig(
	ctx_ context.Context,
	client_ graphql.Client,
	namespace string,
	kafkaServerConfigs []KafkaServerConfigInput,
) (*ReportKafkaServerConfigResponse, error) {
	req_ := &graphql.Request{
		OpName: "ReportKafkaServerConfig",
		Query:  ReportKafkaServerConfig_Operation,
		Variables: &__ReportKafkaServerConfigInput{
			Namespace:          namespace,
			KafkaServerConfigs: kafkaServerConfigs,
		},
	}
	var err_ error

	var data_ ReportKafkaServerConfigResponse
	resp_ := &graphql.Response{Data: &data_}

	err_ = client_.MakeRequest(
		ctx_,
		req_,
		resp_,
	)

	return &data_, err_
}

// The query or mutation executed by ReportNetworkPolicies.
const ReportNetworkPolicies_Operation = `
mutation ReportNetworkPolicies ($namespace: String!, $policies: [NetworkPolicyInput!]!) {
	reportNetworkPolicies(namespace: $namespace, policies: $policies)
}
`

func ReportNetworkPolicies(
	ctx_ context.Context,
	client_ graphql.Client,
	namespace string,
	policies []NetworkPolicyInput,
) (*ReportNetworkPoliciesResponse, error) {
	req_ := &graphql.Request{
		OpName: "ReportNetworkPolicies",
		Query:  ReportNetworkPolicies_Operation,
		Variables: &__ReportNetworkPoliciesInput{
			Namespace: namespace,
			Policies:  policies,
		},
	}
	var err_ error

	var data_ ReportNetworkPoliciesResponse
	resp_ := &graphql.Response{Data: &data_}

	err_ = client_.MakeRequest(
		ctx_,
		req_,
		resp_,
	)

	return &data_, err_
}

// The query or mutation executed by ReportProtectedServicesSnapshot.
const ReportProtectedServicesSnapshot_Operation = `
mutation ReportProtectedServicesSnapshot ($namespace: String!, $services: [ProtectedServiceInput!]!) {
	reportProtectedServicesSnapshot(namespace: $namespace, services: $services)
}
`

func ReportProtectedServicesSnapshot(
	ctx_ context.Context,
	client_ graphql.Client,
	namespace string,
	services []ProtectedServiceInput,
) (*ReportProtectedServicesSnapshotResponse, error) {
	req_ := &graphql.Request{
		OpName: "ReportProtectedServicesSnapshot",
		Query:  ReportProtectedServicesSnapshot_Operation,
		Variables: &__ReportProtectedServicesSnapshotInput{
			Namespace: namespace,
			Services:  services,
		},
	}
	var err_ error

	var data_ ReportProtectedServicesSnapshotResponse
	resp_ := &graphql.Response{Data: &data_}

	err_ = client_.MakeRequest(
		ctx_,
		req_,
		resp_,
	)

	return &data_, err_
}

// The query or mutation executed by dummy.
const dummy_Operation = `
query dummy {
	dummyError
}
`

func dummy(
	ctx_ context.Context,
	client_ graphql.Client,
) (*dummyResponse, error) {
	req_ := &graphql.Request{
		OpName: "dummy",
		Query:  dummy_Operation,
	}
	var err_ error

	var data_ dummyResponse
	resp_ := &graphql.Response{Data: &data_}

	err_ = client_.MakeRequest(
		ctx_,
		req_,
		resp_,
	)

	return &data_, err_
}
