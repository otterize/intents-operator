package intents_client

import (
	"context"
	"encoding/json"

	"github.com/Khan/genqlient/graphql"
)

// ApplyIntentsApplySourcedIntentsIntent includes the requested fields of the GraphQL type Intent.
type ApplyIntentsApplySourcedIntentsIntent struct {
	IntentsFields `json:"-"`
}

// GetId returns ApplyIntentsApplySourcedIntentsIntent.Id, and is useful for accessing the field via an interface.
func (v *ApplyIntentsApplySourcedIntentsIntent) GetId() string { return v.IntentsFields.Id }

// GetType returns ApplyIntentsApplySourcedIntentsIntent.Type, and is useful for accessing the field via an interface.
func (v *ApplyIntentsApplySourcedIntentsIntent) GetType() IntentType { return v.IntentsFields.Type }

// GetClient returns ApplyIntentsApplySourcedIntentsIntent.Client, and is useful for accessing the field via an interface.
func (v *ApplyIntentsApplySourcedIntentsIntent) GetClient() string { return v.IntentsFields.Client }

// GetServer returns ApplyIntentsApplySourcedIntentsIntent.Server, and is useful for accessing the field via an interface.
func (v *ApplyIntentsApplySourcedIntentsIntent) GetServer() string { return v.IntentsFields.Server }

// GetKafkaResources returns ApplyIntentsApplySourcedIntentsIntent.KafkaResources, and is useful for accessing the field via an interface.
func (v *ApplyIntentsApplySourcedIntentsIntent) GetKafkaResources() []IntentsFieldsKafkaResourcesKafkaConfig {
	return v.IntentsFields.KafkaResources
}

// GetHttpResources returns ApplyIntentsApplySourcedIntentsIntent.HttpResources, and is useful for accessing the field via an interface.
func (v *ApplyIntentsApplySourcedIntentsIntent) GetHttpResources() []IntentsFieldsHttpResourcesHTTPConfig {
	return v.IntentsFields.HttpResources
}

// GetFullyQualifiedServiceName returns ApplyIntentsApplySourcedIntentsIntent.FullyQualifiedServiceName, and is useful for accessing the field via an interface.
func (v *ApplyIntentsApplySourcedIntentsIntent) GetFullyQualifiedServiceName() string {
	return v.IntentsFields.FullyQualifiedServiceName
}

func (v *ApplyIntentsApplySourcedIntentsIntent) UnmarshalJSON(b []byte) error {

	if string(b) == "null" {
		return nil
	}

	var firstPass struct {
		*ApplyIntentsApplySourcedIntentsIntent
		graphql.NoUnmarshalJSON
	}
	firstPass.ApplyIntentsApplySourcedIntentsIntent = v

	err := json.Unmarshal(b, &firstPass)
	if err != nil {
		return err
	}

	err = json.Unmarshal(
		b, &v.IntentsFields)
	if err != nil {
		return err
	}
	return nil
}

type __premarshalApplyIntentsApplySourcedIntentsIntent struct {
	Id string `json:"id"`

	Type IntentType `json:"type"`

	Client string `json:"client"`

	Server string `json:"server"`

	KafkaResources []IntentsFieldsKafkaResourcesKafkaConfig `json:"kafkaResources"`

	HttpResources []IntentsFieldsHttpResourcesHTTPConfig `json:"httpResources"`

	FullyQualifiedServiceName string `json:"fullyQualifiedServiceName"`
}

func (v *ApplyIntentsApplySourcedIntentsIntent) MarshalJSON() ([]byte, error) {
	premarshaled, err := v.__premarshalJSON()
	if err != nil {
		return nil, err
	}
	return json.Marshal(premarshaled)
}

func (v *ApplyIntentsApplySourcedIntentsIntent) __premarshalJSON() (*__premarshalApplyIntentsApplySourcedIntentsIntent, error) {
	var retval __premarshalApplyIntentsApplySourcedIntentsIntent

	retval.Id = v.IntentsFields.Id
	retval.Type = v.IntentsFields.Type
	retval.Client = v.IntentsFields.Client
	retval.Server = v.IntentsFields.Server
	retval.KafkaResources = v.IntentsFields.KafkaResources
	retval.HttpResources = v.IntentsFields.HttpResources
	retval.FullyQualifiedServiceName = v.IntentsFields.FullyQualifiedServiceName
	return &retval, nil
}

// ApplyIntentsResponse is returned by ApplyIntents on success.
type ApplyIntentsResponse struct {
	ApplySourcedIntents []*ApplyIntentsApplySourcedIntentsIntent `json:"applySourcedIntents"`
}

// GetApplySourcedIntents returns ApplyIntentsResponse.ApplySourcedIntents, and is useful for accessing the field via an interface.
func (v *ApplyIntentsResponse) GetApplySourcedIntents() []*ApplyIntentsApplySourcedIntentsIntent {
	return v.ApplySourcedIntents
}

type EnvSelector struct {
	Id   *string `json:"id"`
	Name *string `json:"name"`
}

// GetId returns EnvSelector.Id, and is useful for accessing the field via an interface.
func (v *EnvSelector) GetId() *string { return v.Id }

// GetName returns EnvSelector.Name, and is useful for accessing the field via an interface.
func (v *EnvSelector) GetName() *string { return v.Name }

type HTTPConfigInput struct {
	Path   *string     `json:"path"`
	Method *HTTPMethod `json:"method"`
}

// GetPath returns HTTPConfigInput.Path, and is useful for accessing the field via an interface.
func (v *HTTPConfigInput) GetPath() *string { return v.Path }

// GetMethod returns HTTPConfigInput.Method, and is useful for accessing the field via an interface.
func (v *HTTPConfigInput) GetMethod() *HTTPMethod { return v.Method }

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
	Type      *IntentType         `json:"type"`
	Topics    []*KafkaConfigInput `json:"topics"`
	Resources []*HTTPConfigInput  `json:"resources"`
}

// GetType returns IntentBody.Type, and is useful for accessing the field via an interface.
func (v *IntentBody) GetType() *IntentType { return v.Type }

// GetTopics returns IntentBody.Topics, and is useful for accessing the field via an interface.
func (v *IntentBody) GetTopics() []*KafkaConfigInput { return v.Topics }

// GetResources returns IntentBody.Resources, and is useful for accessing the field via an interface.
func (v *IntentBody) GetResources() []*HTTPConfigInput { return v.Resources }

type IntentInput struct {
	Client *string     `json:"client"`
	Server *string     `json:"server"`
	Body   *IntentBody `json:"body"`
}

// GetClient returns IntentInput.Client, and is useful for accessing the field via an interface.
func (v *IntentInput) GetClient() *string { return v.Client }

// GetServer returns IntentInput.Server, and is useful for accessing the field via an interface.
func (v *IntentInput) GetServer() *string { return v.Server }

// GetBody returns IntentInput.Body, and is useful for accessing the field via an interface.
func (v *IntentInput) GetBody() *IntentBody { return v.Body }

type IntentType string

const (
	IntentTypeHttp  IntentType = "HTTP"
	IntentTypeKafka IntentType = "Kafka"
	IntentTypeGrpc  IntentType = "gRPC"
	IntentTypeRedis IntentType = "Redis"
)

// IntentsFields includes the GraphQL fields of Intent requested by the fragment IntentsFields.
type IntentsFields struct {
	Id                        string                                   `json:"id"`
	Type                      IntentType                               `json:"type"`
	Client                    string                                   `json:"client"`
	Server                    string                                   `json:"server"`
	KafkaResources            []IntentsFieldsKafkaResourcesKafkaConfig `json:"kafkaResources"`
	HttpResources             []IntentsFieldsHttpResourcesHTTPConfig   `json:"httpResources"`
	FullyQualifiedServiceName string                                   `json:"fullyQualifiedServiceName"`
}

// GetId returns IntentsFields.Id, and is useful for accessing the field via an interface.
func (v *IntentsFields) GetId() string { return v.Id }

// GetType returns IntentsFields.Type, and is useful for accessing the field via an interface.
func (v *IntentsFields) GetType() IntentType { return v.Type }

// GetClient returns IntentsFields.Client, and is useful for accessing the field via an interface.
func (v *IntentsFields) GetClient() string { return v.Client }

// GetServer returns IntentsFields.Server, and is useful for accessing the field via an interface.
func (v *IntentsFields) GetServer() string { return v.Server }

// GetKafkaResources returns IntentsFields.KafkaResources, and is useful for accessing the field via an interface.
func (v *IntentsFields) GetKafkaResources() []IntentsFieldsKafkaResourcesKafkaConfig {
	return v.KafkaResources
}

// GetHttpResources returns IntentsFields.HttpResources, and is useful for accessing the field via an interface.
func (v *IntentsFields) GetHttpResources() []IntentsFieldsHttpResourcesHTTPConfig {
	return v.HttpResources
}

// GetFullyQualifiedServiceName returns IntentsFields.FullyQualifiedServiceName, and is useful for accessing the field via an interface.
func (v *IntentsFields) GetFullyQualifiedServiceName() string { return v.FullyQualifiedServiceName }

// IntentsFieldsHttpResourcesHTTPConfig includes the requested fields of the GraphQL type HTTPConfig.
type IntentsFieldsHttpResourcesHTTPConfig struct {
	Method HTTPMethod `json:"method"`
	Path   string     `json:"path"`
}

// GetMethod returns IntentsFieldsHttpResourcesHTTPConfig.Method, and is useful for accessing the field via an interface.
func (v *IntentsFieldsHttpResourcesHTTPConfig) GetMethod() HTTPMethod { return v.Method }

// GetPath returns IntentsFieldsHttpResourcesHTTPConfig.Path, and is useful for accessing the field via an interface.
func (v *IntentsFieldsHttpResourcesHTTPConfig) GetPath() string { return v.Path }

// IntentsFieldsKafkaResourcesKafkaConfig includes the requested fields of the GraphQL type KafkaConfig.
type IntentsFieldsKafkaResourcesKafkaConfig struct {
	Operation KafkaOperation `json:"operation"`
	Topic     string         `json:"topic"`
}

// GetOperation returns IntentsFieldsKafkaResourcesKafkaConfig.Operation, and is useful for accessing the field via an interface.
func (v *IntentsFieldsKafkaResourcesKafkaConfig) GetOperation() KafkaOperation { return v.Operation }

// GetTopic returns IntentsFieldsKafkaResourcesKafkaConfig.Topic, and is useful for accessing the field via an interface.
func (v *IntentsFieldsKafkaResourcesKafkaConfig) GetTopic() string { return v.Topic }

type KafkaConfigInput struct {
	Topic     *string         `json:"topic"`
	Operation *KafkaOperation `json:"operation"`
}

// GetTopic returns KafkaConfigInput.Topic, and is useful for accessing the field via an interface.
func (v *KafkaConfigInput) GetTopic() *string { return v.Topic }

// GetOperation returns KafkaConfigInput.Operation, and is useful for accessing the field via an interface.
func (v *KafkaConfigInput) GetOperation() *KafkaOperation { return v.Operation }

type KafkaOperation string

const (
	KafkaOperationConsume         KafkaOperation = "consume"
	KafkaOperationProduce         KafkaOperation = "produce"
	KafkaOperationCreate          KafkaOperation = "create"
	KafkaOperationAlter           KafkaOperation = "alter"
	KafkaOperationDelete          KafkaOperation = "delete"
	KafkaOperationDescribe        KafkaOperation = "describe"
	KafkaOperationClusteraction   KafkaOperation = "ClusterAction"
	KafkaOperationDescribeconfigs KafkaOperation = "DescribeConfigs"
	KafkaOperationAlterconfigs    KafkaOperation = "AlterConfigs"
	KafkaOperationIdempotentwrite KafkaOperation = "IdempotentWrite"
)

// __ApplyIntentsInput is used internally by genqlient
type __ApplyIntentsInput struct {
	Env     *EnvSelector   `json:"env"`
	Source  *string        `json:"source"`
	Intents []*IntentInput `json:"intents"`
}

// GetEnv returns __ApplyIntentsInput.Env, and is useful for accessing the field via an interface.
func (v *__ApplyIntentsInput) GetEnv() *EnvSelector { return v.Env }

// GetSource returns __ApplyIntentsInput.Source, and is useful for accessing the field via an interface.
func (v *__ApplyIntentsInput) GetSource() *string { return v.Source }

// GetIntents returns __ApplyIntentsInput.Intents, and is useful for accessing the field via an interface.
func (v *__ApplyIntentsInput) GetIntents() []*IntentInput { return v.Intents }

func ApplyIntents(
	ctx context.Context,
	client graphql.Client,
	env *EnvSelector,
	source *string,
	intents []*IntentInput,
) (*ApplyIntentsResponse, error) {
	__input := __ApplyIntentsInput{
		Env:     env,
		Source:  source,
		Intents: intents,
	}
	var err error

	var retval ApplyIntentsResponse
	err = client.MakeRequest(
		ctx,
		"ApplyIntents",
		`
mutation ApplyIntents ($env: EnvSelector!, $source: String!, $intents: [IntentInput!]!) {
	applySourcedIntents(env: $env, intents: $intents, source: $source) {
		... IntentsFields
	}
}
fragment IntentsFields on Intent {
	id
	type
	client
	server
	kafkaResources {
		operation
		topic
	}
	httpResources {
		method
		path
	}
	fullyQualifiedServiceName
}
`,
		&retval,
		&__input,
	)
	return &retval, err
}
