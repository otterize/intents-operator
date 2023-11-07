// Code generated by github.com/Khan/genqlient, DO NOT EDIT.

package graphqlclient

import (
	"context"

	"github.com/Khan/genqlient/graphql"
)

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
	Table      *string              `json:"table"`
	Operations []*DatabaseOperation `json:"operations"`
}

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
	Namespace         *string                `json:"namespace"`
	ClientName        *string                `json:"clientName"`
	ServerName        *string                `json:"serverName"`
	ServerNamespace   *string                `json:"serverNamespace"`
	Type              *IntentType            `json:"type"`
	Topics            []*KafkaConfigInput    `json:"topics"`
	Resources         []*HTTPConfigInput     `json:"resources"`
	DatabaseResources []*DatabaseConfigInput `json:"databaseResources"`
	AWSActions        []*string              `json:"AWSActions"`
	Status            *IntentStatusInput     `json:"status"`
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

// GetAWSActions returns IntentInput.AWSActions, and is useful for accessing the field via an interface.
func (v *IntentInput) GetAWSActions() []*string { return v.AWSActions }

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
	IntentTypeS3       IntentType = "S3"
)

type IntentsOperatorConfigurationInput struct {
	GlobalEnforcementEnabled        bool `json:"globalEnforcementEnabled"`
	NetworkPolicyEnforcementEnabled bool `json:"networkPolicyEnforcementEnabled"`
	KafkaACLEnforcementEnabled      bool `json:"kafkaACLEnforcementEnabled"`
	IstioPolicyEnforcementEnabled   bool `json:"istioPolicyEnforcementEnabled"`
	ProtectedServicesEnabled        bool `json:"protectedServicesEnabled"`
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

func HandleDatabaseIntents(
	ctx context.Context,
	client graphql.Client,
	intents []IntentInput,
	action DBPermissionChange,
) (*HandleDatabaseIntentsResponse, error) {
	req := &graphql.Request{
		OpName: "HandleDatabaseIntents",
		Query: `
mutation HandleDatabaseIntents ($intents: [IntentInput!]!, $action: DBPermissionChange!) {
	handleDatabaseIntents(intents: $intents, action: $action)
}
`,
		Variables: &__HandleDatabaseIntentsInput{
			Intents: intents,
			Action:  action,
		},
	}
	var err error

	var data HandleDatabaseIntentsResponse
	resp := &graphql.Response{Data: &data}

	err = client.MakeRequest(
		ctx,
		req,
		resp,
	)

	return &data, err
}

func ReportAppliedKubernetesIntents(
	ctx context.Context,
	client graphql.Client,
	namespace *string,
	intents []*IntentInput,
) (*ReportAppliedKubernetesIntentsResponse, error) {
	req := &graphql.Request{
		OpName: "ReportAppliedKubernetesIntents",
		Query: `
mutation ReportAppliedKubernetesIntents ($namespace: String!, $intents: [IntentInput!]!) {
	reportAppliedKubernetesIntents(namespace: $namespace, intents: $intents)
}
`,
		Variables: &__ReportAppliedKubernetesIntentsInput{
			Namespace: namespace,
			Intents:   intents,
		},
	}
	var err error

	var data ReportAppliedKubernetesIntentsResponse
	resp := &graphql.Response{Data: &data}

	err = client.MakeRequest(
		ctx,
		req,
		resp,
	)

	return &data, err
}

func ReportComponentStatus(
	ctx context.Context,
	client graphql.Client,
	component ComponentType,
) (*ReportComponentStatusResponse, error) {
	req := &graphql.Request{
		OpName: "ReportComponentStatus",
		Query: `
mutation ReportComponentStatus ($component: ComponentType!) {
	reportIntegrationComponentStatus(component: $component)
}
`,
		Variables: &__ReportComponentStatusInput{
			Component: component,
		},
	}
	var err error

	var data ReportComponentStatusResponse
	resp := &graphql.Response{Data: &data}

	err = client.MakeRequest(
		ctx,
		req,
		resp,
	)

	return &data, err
}

func ReportIntentsOperatorConfiguration(
	ctx context.Context,
	client graphql.Client,
	configuration IntentsOperatorConfigurationInput,
) (*ReportIntentsOperatorConfigurationResponse, error) {
	req := &graphql.Request{
		OpName: "ReportIntentsOperatorConfiguration",
		Query: `
mutation ReportIntentsOperatorConfiguration ($configuration: IntentsOperatorConfigurationInput!) {
	reportIntentsOperatorConfiguration(configuration: $configuration)
}
`,
		Variables: &__ReportIntentsOperatorConfigurationInput{
			Configuration: configuration,
		},
	}
	var err error

	var data ReportIntentsOperatorConfigurationResponse
	resp := &graphql.Response{Data: &data}

	err = client.MakeRequest(
		ctx,
		req,
		resp,
	)

	return &data, err
}

func ReportKafkaServerConfig(
	ctx context.Context,
	client graphql.Client,
	namespace string,
	kafkaServerConfigs []KafkaServerConfigInput,
) (*ReportKafkaServerConfigResponse, error) {
	req := &graphql.Request{
		OpName: "ReportKafkaServerConfig",
		Query: `
mutation ReportKafkaServerConfig ($namespace: String!, $kafkaServerConfigs: [KafkaServerConfigInput!]!) {
	reportKafkaServerConfigs(namespace: $namespace, serverConfigs: $kafkaServerConfigs)
}
`,
		Variables: &__ReportKafkaServerConfigInput{
			Namespace:          namespace,
			KafkaServerConfigs: kafkaServerConfigs,
		},
	}
	var err error

	var data ReportKafkaServerConfigResponse
	resp := &graphql.Response{Data: &data}

	err = client.MakeRequest(
		ctx,
		req,
		resp,
	)

	return &data, err
}

func ReportNetworkPolicies(
	ctx context.Context,
	client graphql.Client,
	namespace string,
	policies []NetworkPolicyInput,
) (*ReportNetworkPoliciesResponse, error) {
	req := &graphql.Request{
		OpName: "ReportNetworkPolicies",
		Query: `
mutation ReportNetworkPolicies ($namespace: String!, $policies: [NetworkPolicyInput!]!) {
	reportNetworkPolicies(namespace: $namespace, policies: $policies)
}
`,
		Variables: &__ReportNetworkPoliciesInput{
			Namespace: namespace,
			Policies:  policies,
		},
	}
	var err error

	var data ReportNetworkPoliciesResponse
	resp := &graphql.Response{Data: &data}

	err = client.MakeRequest(
		ctx,
		req,
		resp,
	)

	return &data, err
}

func ReportProtectedServicesSnapshot(
	ctx context.Context,
	client graphql.Client,
	namespace string,
	services []ProtectedServiceInput,
) (*ReportProtectedServicesSnapshotResponse, error) {
	req := &graphql.Request{
		OpName: "ReportProtectedServicesSnapshot",
		Query: `
mutation ReportProtectedServicesSnapshot ($namespace: String!, $services: [ProtectedServiceInput!]!) {
	reportProtectedServicesSnapshot(namespace: $namespace, services: $services)
}
`,
		Variables: &__ReportProtectedServicesSnapshotInput{
			Namespace: namespace,
			Services:  services,
		},
	}
	var err error

	var data ReportProtectedServicesSnapshotResponse
	resp := &graphql.Response{Data: &data}

	err = client.MakeRequest(
		ctx,
		req,
		resp,
	)

	return &data, err
}
