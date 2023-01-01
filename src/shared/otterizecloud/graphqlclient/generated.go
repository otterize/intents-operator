// Code generated by github.com/Khan/genqlient, DO NOT EDIT.

package graphqlclient

import (
	"context"

	"github.com/Khan/genqlient/graphql"
)

type HTTPConfigInput struct {
	Path   string     `json:"path"`
	Method HTTPMethod `json:"method"`
}

// GetPath returns HTTPConfigInput.Path, and is useful for accessing the field via an interface.
func (v *HTTPConfigInput) GetPath() string { return v.Path }

// GetMethod returns HTTPConfigInput.Method, and is useful for accessing the field via an interface.
func (v *HTTPConfigInput) GetMethod() HTTPMethod { return v.Method }

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

type IntentBody struct {
	Type      IntentType         `json:"type"`
	Topics    []KafkaConfigInput `json:"topics"`
	Resources []HTTPConfigInput  `json:"resources"`
}

// GetType returns IntentBody.Type, and is useful for accessing the field via an interface.
func (v *IntentBody) GetType() IntentType { return v.Type }

// GetTopics returns IntentBody.Topics, and is useful for accessing the field via an interface.
func (v *IntentBody) GetTopics() []KafkaConfigInput { return v.Topics }

// GetResources returns IntentBody.Resources, and is useful for accessing the field via an interface.
func (v *IntentBody) GetResources() []HTTPConfigInput { return v.Resources }

type IntentInput struct {
	Namespace       string     `json:"namespace"`
	ClientName      string     `json:"clientName"`
	ServerName      string     `json:"serverName"`
	ServerNamespace string     `json:"serverNamespace"`
	Body            IntentBody `json:"body"`
}

// GetNamespace returns IntentInput.Namespace, and is useful for accessing the field via an interface.
func (v *IntentInput) GetNamespace() string { return v.Namespace }

// GetClientName returns IntentInput.ClientName, and is useful for accessing the field via an interface.
func (v *IntentInput) GetClientName() string { return v.ClientName }

// GetServerName returns IntentInput.ServerName, and is useful for accessing the field via an interface.
func (v *IntentInput) GetServerName() string { return v.ServerName }

// GetServerNamespace returns IntentInput.ServerNamespace, and is useful for accessing the field via an interface.
func (v *IntentInput) GetServerNamespace() string { return v.ServerNamespace }

// GetBody returns IntentInput.Body, and is useful for accessing the field via an interface.
func (v *IntentInput) GetBody() IntentBody { return v.Body }

type IntentType string

const (
	IntentTypeHttp  IntentType = "HTTP"
	IntentTypeKafka IntentType = "KAFKA"
	IntentTypeGrpc  IntentType = "GRPC"
	IntentTypeRedis IntentType = "REDIS"
)

type KafkaConfigInput struct {
	Name       string           `json:"name"`
	Operations []KafkaOperation `json:"operations"`
}

// GetName returns KafkaConfigInput.Name, and is useful for accessing the field via an interface.
func (v *KafkaConfigInput) GetName() string { return v.Name }

// GetOperations returns KafkaConfigInput.Operations, and is useful for accessing the field via an interface.
func (v *KafkaConfigInput) GetOperations() []KafkaOperation { return v.Operations }

type KafkaOperation string

const (
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
	Name    string            `json:"name"`
	Address string            `json:"address"`
	Topics  []KafkaTopicInput `json:"topics"`
}

// GetName returns KafkaServerConfigInput.Name, and is useful for accessing the field via an interface.
func (v *KafkaServerConfigInput) GetName() string { return v.Name }

// GetAddress returns KafkaServerConfigInput.Address, and is useful for accessing the field via an interface.
func (v *KafkaServerConfigInput) GetAddress() string { return v.Address }

// GetTopics returns KafkaServerConfigInput.Topics, and is useful for accessing the field via an interface.
func (v *KafkaServerConfigInput) GetTopics() []KafkaTopicInput { return v.Topics }

type KafkaTopicInput struct {
	ClientIdentityRequired bool   `json:"clientIdentityRequired"`
	IntentsRequired        bool   `json:"intentsRequired"`
	Pattern                string `json:"pattern"`
	Topic                  string `json:"topic"`
}

// GetClientIdentityRequired returns KafkaTopicInput.ClientIdentityRequired, and is useful for accessing the field via an interface.
func (v *KafkaTopicInput) GetClientIdentityRequired() bool { return v.ClientIdentityRequired }

// GetIntentsRequired returns KafkaTopicInput.IntentsRequired, and is useful for accessing the field via an interface.
func (v *KafkaTopicInput) GetIntentsRequired() bool { return v.IntentsRequired }

// GetPattern returns KafkaTopicInput.Pattern, and is useful for accessing the field via an interface.
func (v *KafkaTopicInput) GetPattern() string { return v.Pattern }

// GetTopic returns KafkaTopicInput.Topic, and is useful for accessing the field via an interface.
func (v *KafkaTopicInput) GetTopic() string { return v.Topic }

// ReportAppliedKubernetesIntentsResponse is returned by ReportAppliedKubernetesIntents on success.
type ReportAppliedKubernetesIntentsResponse struct {
	ReportAppliedKubernetesIntents bool `json:"reportAppliedKubernetesIntents"`
}

// GetReportAppliedKubernetesIntents returns ReportAppliedKubernetesIntentsResponse.ReportAppliedKubernetesIntents, and is useful for accessing the field via an interface.
func (v *ReportAppliedKubernetesIntentsResponse) GetReportAppliedKubernetesIntents() bool {
	return v.ReportAppliedKubernetesIntents
}

// ReportKafkaServerConfigResponse is returned by ReportKafkaServerConfig on success.
type ReportKafkaServerConfigResponse struct {
	ReportKafkaServerConfig bool `json:"reportKafkaServerConfig"`
}

// GetReportKafkaServerConfig returns ReportKafkaServerConfigResponse.ReportKafkaServerConfig, and is useful for accessing the field via an interface.
func (v *ReportKafkaServerConfigResponse) GetReportKafkaServerConfig() bool {
	return v.ReportKafkaServerConfig
}

// __ReportAppliedKubernetesIntentsInput is used internally by genqlient
type __ReportAppliedKubernetesIntentsInput struct {
	Namespace string        `json:"namespace"`
	Intents   []IntentInput `json:"intents"`
}

// GetNamespace returns __ReportAppliedKubernetesIntentsInput.Namespace, and is useful for accessing the field via an interface.
func (v *__ReportAppliedKubernetesIntentsInput) GetNamespace() string { return v.Namespace }

// GetIntents returns __ReportAppliedKubernetesIntentsInput.Intents, and is useful for accessing the field via an interface.
func (v *__ReportAppliedKubernetesIntentsInput) GetIntents() []IntentInput { return v.Intents }

// __ReportKafkaServerConfigInput is used internally by genqlient
type __ReportKafkaServerConfigInput struct {
	Namespace string                 `json:"namespace"`
	Source    string                 `json:"source"`
	Server    KafkaServerConfigInput `json:"server"`
}

// GetNamespace returns __ReportKafkaServerConfigInput.Namespace, and is useful for accessing the field via an interface.
func (v *__ReportKafkaServerConfigInput) GetNamespace() string { return v.Namespace }

// GetSource returns __ReportKafkaServerConfigInput.Source, and is useful for accessing the field via an interface.
func (v *__ReportKafkaServerConfigInput) GetSource() string { return v.Source }

// GetServer returns __ReportKafkaServerConfigInput.Server, and is useful for accessing the field via an interface.
func (v *__ReportKafkaServerConfigInput) GetServer() KafkaServerConfigInput { return v.Server }

func ReportAppliedKubernetesIntents(
	ctx context.Context,
	client graphql.Client,
	namespace string,
	intents []IntentInput,
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

func ReportKafkaServerConfig(
	ctx context.Context,
	client graphql.Client,
	namespace string,
	source string,
	server KafkaServerConfigInput,
) (*ReportKafkaServerConfigResponse, error) {
	req := &graphql.Request{
		OpName: "ReportKafkaServerConfig",
		Query: `
mutation ReportKafkaServerConfig ($namespace: String!, $source: String!, $server: KafkaServerConfigInput!) {
	reportKafkaServerConfig(namespace: $namespace, source: $source, serverConfig: $server)
}
`,
		Variables: &__ReportKafkaServerConfigInput{
			Namespace: namespace,
			Source:    source,
			Server:    server,
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
