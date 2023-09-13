// Code generated by github.com/Khan/genqlient, DO NOT EDIT.

package telemetriesgql

import (
	"context"

	"github.com/Khan/genqlient/graphql"
)

type Component struct {
	ComponentType       ComponentType `json:"componentType"`
	ComponentInstanceId string        `json:"componentInstanceId"`
	ContextId           string        `json:"contextId"`
	Version             string        `json:"version"`
	CloudClientId       string        `json:"cloudClientId"`
}

// GetComponentType returns Component.ComponentType, and is useful for accessing the field via an interface.
func (v *Component) GetComponentType() ComponentType { return v.ComponentType }

// GetComponentInstanceId returns Component.ComponentInstanceId, and is useful for accessing the field via an interface.
func (v *Component) GetComponentInstanceId() string { return v.ComponentInstanceId }

// GetContextId returns Component.ContextId, and is useful for accessing the field via an interface.
func (v *Component) GetContextId() string { return v.ContextId }

// GetVersion returns Component.Version, and is useful for accessing the field via an interface.
func (v *Component) GetVersion() string { return v.Version }

// GetCloudClientId returns Component.CloudClientId, and is useful for accessing the field via an interface.
func (v *Component) GetCloudClientId() string { return v.CloudClientId }

type ComponentType string

const (
	ComponentTypeIntentsOperator     ComponentType = "INTENTS_OPERATOR"
	ComponentTypeCredentialsOperator ComponentType = "CREDENTIALS_OPERATOR"
	ComponentTypeNetworkMapper       ComponentType = "NETWORK_MAPPER"
	ComponentTypeCli                 ComponentType = "CLI"
)

type EventType string

const (
	EventTypeIntentsDeleted              EventType = "INTENTS_DELETED"
	EventTypeIntentsApplied              EventType = "INTENTS_APPLIED"
	EventTypeIntentsAppliedKafka         EventType = "INTENTS_APPLIED_KAFKA"
	EventTypeIntentsAppliedHttp          EventType = "INTENTS_APPLIED_HTTP"
	EventTypeIntentsAppliedDatabase      EventType = "INTENTS_APPLIED_DATABASE"
	EventTypeIntentsDiscovered           EventType = "INTENTS_DISCOVERED"
	EventTypeIntentsDiscoveredSocketScan EventType = "INTENTS_DISCOVERED_SOCKET_SCAN"
	EventTypeIntentsDiscoveredCapture    EventType = "INTENTS_DISCOVERED_CAPTURE"
	EventTypeIntentsDiscoveredKafka      EventType = "INTENTS_DISCOVERED_KAFKA"
	EventTypeIntentsDiscoveredIstio      EventType = "INTENTS_DISCOVERED_ISTIO"
	EventTypeMapperExport                EventType = "MAPPER_EXPORT"
	EventTypeMapperVisualize             EventType = "MAPPER_VISUALIZE"
	EventTypeKafkaServerConfigApplied    EventType = "KAFKA_SERVER_CONFIG_APPLIED"
	EventTypeKafkaServerConfigDeleted    EventType = "KAFKA_SERVER_CONFIG_DELETED"
	EventTypeNetworkPoliciesCreated      EventType = "NETWORK_POLICIES_CREATED"
	EventTypeNetworkPoliciesDeleted      EventType = "NETWORK_POLICIES_DELETED"
	EventTypeKafkaAclsCreated            EventType = "KAFKA_ACLS_CREATED"
	EventTypeKafkaAclsDeleted            EventType = "KAFKA_ACLS_DELETED"
	EventTypeIstioPoliciesCreated        EventType = "ISTIO_POLICIES_CREATED"
	EventTypeIstioPoliciesDeleted        EventType = "ISTIO_POLICIES_DELETED"
	EventTypeStarted                     EventType = "STARTED"
	EventTypeServiceDiscovered           EventType = "SERVICE_DISCOVERED"
)

// SendTelemetriesResponse is returned by SendTelemetries on success.
type SendTelemetriesResponse struct {
	SendTelemetries bool `json:"sendTelemetries"`
}

// GetSendTelemetries returns SendTelemetriesResponse.SendTelemetries, and is useful for accessing the field via an interface.
func (v *SendTelemetriesResponse) GetSendTelemetries() bool { return v.SendTelemetries }

type TelemetryData struct {
	EventType EventType `json:"eventType"`
	Count     int       `json:"count"`
}

// GetEventType returns TelemetryData.EventType, and is useful for accessing the field via an interface.
func (v *TelemetryData) GetEventType() EventType { return v.EventType }

// GetCount returns TelemetryData.Count, and is useful for accessing the field via an interface.
func (v *TelemetryData) GetCount() int { return v.Count }

type TelemetryInput struct {
	Component Component     `json:"component"`
	Data      TelemetryData `json:"data"`
}

// GetComponent returns TelemetryInput.Component, and is useful for accessing the field via an interface.
func (v *TelemetryInput) GetComponent() Component { return v.Component }

// GetData returns TelemetryInput.Data, and is useful for accessing the field via an interface.
func (v *TelemetryInput) GetData() TelemetryData { return v.Data }

// __SendTelemetriesInput is used internally by genqlient
type __SendTelemetriesInput struct {
	Telemetries []TelemetryInput `json:"telemetries"`
}

// GetTelemetries returns __SendTelemetriesInput.Telemetries, and is useful for accessing the field via an interface.
func (v *__SendTelemetriesInput) GetTelemetries() []TelemetryInput { return v.Telemetries }

func SendTelemetries(
	ctx context.Context,
	client graphql.Client,
	telemetries []TelemetryInput,
) (*SendTelemetriesResponse, error) {
	req := &graphql.Request{
		OpName: "SendTelemetries",
		Query: `
mutation SendTelemetries ($telemetries: [TelemetryInput!]!) {
	sendTelemetries(telemetries: $telemetries)
}
`,
		Variables: &__SendTelemetriesInput{
			Telemetries: telemetries,
		},
	}
	var err error

	var data SendTelemetriesResponse
	resp := &graphql.Response{Data: &data}

	err = client.MakeRequest(
		ctx,
		req,
		resp,
	)

	return &data, err
}
