package telemetrysender

import (
	"context"
	"fmt"
	"github.com/Khan/genqlient/graphql"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/otterize/intents-operator/src/shared/telemetries/basicbatch"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesconfig"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/spf13/viper"
	"net/http"
	"time"
)

type TelemetrySender struct {
	eventBatcher          *basicbatch.Batcher[telemetriesgql.TelemetryInput]
	uniqueEventBatcher    *basicbatch.Batcher[UniqueEvent]
	uniqueEventsCounter   *UniqueCounter
	snapshotResetInterval time.Duration
	lastSnapshotResetTime time.Time
	telemetriesClient     graphql.Client
	enabled               bool
}

func newGqlClient() graphql.Client {
	apiAddress := viper.GetString(otterizecloudclient.OtterizeAPIAddressKey)
	graphqlUrl := fmt.Sprintf("%s/telemetry/query", apiAddress)
	clientTimeout := viper.GetDuration(telemetriesconfig.TimeoutKey)
	clientWithTimeout := &http.Client{Timeout: clientTimeout}
	return graphql.NewClient(graphqlUrl, clientWithTimeout)
}

func batchSendTelemetries(ctx context.Context, telemetriesClient graphql.Client, telemetries []telemetriesgql.TelemetryInput) error {
	_, err := telemetriesgql.SendTelemetries(ctx, telemetriesClient, telemetries)
	if err != nil {
		return errors.Errorf("failed batch sending telemetries: %w", err)
	}
	return nil
}

func New() *TelemetrySender {
	enabled := telemetriesconfig.IsUsageTelemetryEnabled()
	maxBatchSize := viper.GetInt(telemetriesconfig.TelemetryMaxBatchSizeKey)
	interval := viper.GetInt(telemetriesconfig.TelemetryIntervalKey)
	telemetriesClient := newGqlClient()
	snapshotResetInterval := viper.GetDuration(telemetriesconfig.TelemetryResetIntervalKey)

	sender := &TelemetrySender{
		snapshotResetInterval: snapshotResetInterval,
		lastSnapshotResetTime: time.Now(),
		telemetriesClient:     telemetriesClient,
		enabled:               enabled,
	}

	batchSendFunc := func(telemetries []telemetriesgql.TelemetryInput) error {
		return batchSendTelemetries(context.Background(), sender.telemetriesClient, telemetries)
	}

	sender.uniqueEventsCounter = NewUniqueCounter()
	sender.uniqueEventBatcher = basicbatch.NewBatcher(sender.HandleCounters, time.Duration(interval)*time.Second, maxBatchSize, 5000)
	sender.eventBatcher = basicbatch.NewBatcher(batchSendFunc, time.Duration(interval)*time.Second, maxBatchSize, 5000)

	return sender

}

func (t *TelemetrySender) Send(component telemetriesgql.Component, eventType telemetriesgql.EventType, count int) error {
	if !t.enabled {
		return nil
	}

	telemetryData := telemetriesgql.TelemetryData{EventType: eventType, Count: count}
	telemetry := telemetriesgql.TelemetryInput{Component: component, Data: telemetryData}
	t.eventBatcher.AddNoWait(telemetry)

	return nil
}

func (t *TelemetrySender) IncrementCounter(component telemetriesgql.Component, eventType telemetriesgql.EventType, key string) error {
	if !t.enabled {
		return nil
	}

	item := UniqueEvent{
		Event: UniqueEventMetadata{
			Component: component,
			EventType: eventType,
		},
		Key: key,
	}
	t.uniqueEventBatcher.AddNoWait(item)
	return nil
}

func (t *TelemetrySender) HandleCounters(batch []UniqueEvent) error {
	if len(batch) == 0 {
		return nil
	}

	for _, item := range batch {
		t.uniqueEventsCounter.IncrementCounter(item.Event.Component, item.Event.EventType, item.Key)
	}

	counts := t.uniqueEventsCounter.Get()

	telemetries := make([]telemetriesgql.TelemetryInput, 0)
	for _, count := range counts {
		telemetry := telemetriesgql.TelemetryInput{
			Component: count.Event.Component,
			Data: telemetriesgql.TelemetryData{
				EventType: count.Event.EventType,
				Count:     count.Count,
			},
		}
		telemetries = append(telemetries, telemetry)
	}

	err := batchSendTelemetries(context.Background(), t.telemetriesClient, telemetries)
	if err != nil {
		return errors.Wrap(err)
	}

	timeUntilReset := t.lastSnapshotResetTime.Add(t.snapshotResetInterval)

	if time.Now().After(timeUntilReset) {
		t.uniqueEventsCounter.Reset()
		t.lastSnapshotResetTime = time.Now()
	}

	return nil
}
