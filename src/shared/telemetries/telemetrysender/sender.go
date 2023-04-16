package telemetrysender

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Khan/genqlient/graphql"
	"github.com/otterize/intents-operator/src/shared/telemetries/basicbatch"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"net/http"
	"time"
)

type TelemetrySender struct {
	batcher           *basicbatch.Batcher[telemetriesgql.TelemetryInput]
	telemetriesClient graphql.Client
	enabled           bool
}

func newGqlClient() graphql.Client {
	apiAddress := viper.GetString(TelemetriesAPIAddressKey)
	clientTimeout := viper.GetDuration(TimeoutKey)
	transport := &http.Transport{}
	clientWithTimeout := &http.Client{Timeout: clientTimeout, Transport: transport}
	return graphql.NewClient(apiAddress, clientWithTimeout)
}

func batchSendTelemetries(ctx context.Context, telemetriesClient graphql.Client, telemetries []telemetriesgql.TelemetryInput) error {
	_, err := telemetriesgql.SendTelemetries(ctx, telemetriesClient, telemetries)
	if err != nil {
		logrus.Errorf("failed batch sending telemetries: %s", err)
	}
	return nil
}

func New() *TelemetrySender {
	enabled := viper.GetBool(TelemetriesEnabledKey)
	maxBatchSize := viper.GetInt(TelemetriesMaxBatchSizeKey)
	interval := viper.GetInt(TelemetriesIntervalKey)
	telemetriesClient := newGqlClient()
	sender := &TelemetrySender{telemetriesClient: telemetriesClient, enabled: enabled}
	batchSendFunc := func(telemetries []telemetriesgql.TelemetryInput) error {
		return batchSendTelemetries(context.Background(), sender.telemetriesClient, telemetries)

	}
	sender.batcher = basicbatch.NewBatcher(batchSendFunc, time.Duration(interval)*time.Second, maxBatchSize, 5000)
	return sender

}

func (t *TelemetrySender) Send(component telemetriesgql.Component, eventType telemetriesgql.EventType, data map[string]string) error {
	if !t.enabled {
		return nil
	}
	dataJsonStr := ""
	if len(data) != 0 {
		dataJson, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("failed serializing data %s", data)
		}
		dataJsonStr = string(dataJson)
	}

	telemetryData := telemetriesgql.TelemetryData{EventType: eventType, Data: dataJsonStr}
	telemetry := telemetriesgql.TelemetryInput{Component: component, Data: telemetryData}
	t.batcher.AddNoWait(telemetry)
	return nil
}
