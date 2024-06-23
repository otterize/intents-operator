package errorreporter

import (
	"context"
	"github.com/Khan/genqlient/graphql"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/telemetries/basicbatch"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesconfig"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/samber/lo"
	"github.com/spf13/viper"
	"net/http"
	"time"
)

type ErrorSender struct {
	component    telemetriesgql.Component
	errorBatcher *basicbatch.Batcher[*telemetriesgql.Error]
	gqlClient    graphql.Client
	enabled      bool
}

func newGqlClient() graphql.Client {
	apiAddress := viper.GetString(telemetriesconfig.TelemetryAPIAddressKey)
	clientTimeout := viper.GetDuration(telemetriesconfig.TimeoutKey)
	clientWithTimeout := &http.Client{Timeout: clientTimeout}
	return graphql.NewClient(apiAddress, clientWithTimeout)
}

func (s *ErrorSender) SendSync(errs []*telemetriesgql.Error) error {
	if !s.enabled {
		return nil
	}
	ctxTimeout, cancel := context.WithTimeout(context.Background(), viper.GetDuration(telemetriesconfig.TimeoutKey))
	defer cancel()
	_, err := telemetriesgql.ReportErrors(ctxTimeout, s.gqlClient, lo.ToPtr(s.component), errs)
	if err != nil {
		return errors.Errorf("failed batch sending errors: %w", err)
	}
	return nil
}

func New(componentType telemetriesgql.TelemetryComponentType) *ErrorSender {
	enabled := telemetriesconfig.IsErrorTelemetryEnabled()
	maxBatchSize := viper.GetInt(telemetriesconfig.TelemetryMaxBatchSizeKey)
	interval := viper.GetInt(telemetriesconfig.TelemetryIntervalKey)
	gqlClient := newGqlClient()

	sender := &ErrorSender{
		gqlClient: gqlClient,
		enabled:   enabled,
		component: currentComponent(componentType),
	}

	sender.errorBatcher = basicbatch.NewBatcher(sender.SendSync, time.Duration(interval)*time.Second, maxBatchSize, 5000)

	return sender

}

func (s *ErrorSender) SendAsync(err telemetriesgql.Error) error {
	if !s.enabled {
		return nil
	}

	s.errorBatcher.AddNoWait(lo.ToPtr(err))

	return nil
}
