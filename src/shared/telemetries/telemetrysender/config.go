package telemetrysender

import (
	"github.com/spf13/viper"
	"strings"
)

const (
	TelemetryAPIAddressKey       = "telemetry-address"
	TimeoutKey                   = "telemetry-client-timeout"
	CloudClientTimeoutDefault    = "30s"
	TelemetryEnabledKey          = "telemetry-enabled"
	TelemetryEnabledDefault      = false
	TelemetryMaxBatchSizeKey     = "telemetry-max-batch-size"
	TelemetryMaxBatchSizeDefault = 100
	TelemetryIntervalKey         = "telemetry-interval-seconds"
	TelemetryIntervalDefault     = 5
	TelemetryAddressDefault      = "https://app.otterize.com/api/telemetry/query"
	EnvPrefix                    = "OTTERIZE"
)

func init() {
	viper.SetDefault(TelemetryAPIAddressKey, TelemetryAddressDefault)
	viper.SetDefault(TimeoutKey, CloudClientTimeoutDefault)
	viper.SetDefault(TelemetryIntervalKey, TelemetryIntervalDefault)
	viper.SetDefault(TelemetryMaxBatchSizeKey, TelemetryMaxBatchSizeDefault)
	viper.SetDefault(TelemetryEnabledKey, TelemetryEnabledDefault)
	viper.SetEnvPrefix(EnvPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}
