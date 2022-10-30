// Code generated by github.com/Khan/genqlient, DO NOT EDIT.

package intents

import (
	"context"

	"github.com/Khan/genqlient/graphql"
)

// ApplyIntentsResponse is returned by ApplyIntents on success.
type ApplyIntentsResponse struct {
	ApplySourcedIntents bool `json:"applySourcedIntents"`
}

// GetApplySourcedIntents returns ApplyIntentsResponse.ApplySourcedIntents, and is useful for accessing the field via an interface.
func (v *ApplyIntentsResponse) GetApplySourcedIntents() bool { return v.ApplySourcedIntents }

type EnvSelector struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

// GetId returns EnvSelector.Id, and is useful for accessing the field via an interface.
func (v *EnvSelector) GetId() string { return v.Id }

// GetName returns EnvSelector.Name, and is useful for accessing the field via an interface.
func (v *EnvSelector) GetName() string { return v.Name }

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
	Client string     `json:"client"`
	Server string     `json:"server"`
	Body   IntentBody `json:"body"`
}

// GetClient returns IntentInput.Client, and is useful for accessing the field via an interface.
func (v *IntentInput) GetClient() string { return v.Client }

// GetServer returns IntentInput.Server, and is useful for accessing the field via an interface.
func (v *IntentInput) GetServer() string { return v.Server }

// GetBody returns IntentInput.Body, and is useful for accessing the field via an interface.
func (v *IntentInput) GetBody() IntentBody { return v.Body }

type IntentType string

const (
	IntentTypeHttp  IntentType = "HTTP"
	IntentTypeKafka IntentType = "Kafka"
	IntentTypeGrpc  IntentType = "gRPC"
	IntentTypeRedis IntentType = "Redis"
)

type KafkaConfigInput struct {
	Topic     string         `json:"topic"`
	Operation KafkaOperation `json:"operation"`
}

// GetTopic returns KafkaConfigInput.Topic, and is useful for accessing the field via an interface.
func (v *KafkaConfigInput) GetTopic() string { return v.Topic }

// GetOperation returns KafkaConfigInput.Operation, and is useful for accessing the field via an interface.
func (v *KafkaConfigInput) GetOperation() KafkaOperation { return v.Operation }

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
	Env     EnvSelector   `json:"env"`
	Source  string        `json:"source"`
	Intents []IntentInput `json:"intents"`
}

// GetEnv returns __ApplyIntentsInput.Env, and is useful for accessing the field via an interface.
func (v *__ApplyIntentsInput) GetEnv() EnvSelector { return v.Env }

// GetSource returns __ApplyIntentsInput.Source, and is useful for accessing the field via an interface.
func (v *__ApplyIntentsInput) GetSource() string { return v.Source }

// GetIntents returns __ApplyIntentsInput.Intents, and is useful for accessing the field via an interface.
func (v *__ApplyIntentsInput) GetIntents() []IntentInput { return v.Intents }

func ApplyIntents(
	ctx context.Context,
	client graphql.Client,
	env EnvSelector,
	source string,
	intents []IntentInput,
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
	applySourcedIntents(env: $env, intents: $intents, source: $source)
}
`,
		&retval,
		&__input,
	)
	return &retval, err
}
