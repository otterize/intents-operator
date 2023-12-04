package telemetrysender

import (
	"github.com/spf13/viper"
	"strings"
)

const (
	TelemetryAPIAddressKey         = "telemetry-address"
	TimeoutKey                     = "telemetry-client-timeout"
	CloudClientTimeoutDefault      = "30s"
	TelemetryUsageEnabledKey       = "telemetry-usage-enabled"
	TelemetryUsageEnabledDefault   = false
	TelemetryMaxBatchSizeKey       = "telemetry-max-batch-size"
	TelemetryMaxBatchSizeDefault   = 100
	TelemetryIntervalKey           = "telemetry-interval-seconds"
	TelemetryIntervalDefault       = 5
	TelemetryAddressDefault        = "https://app.otterize.com/api/telemetry/query"
	TelemetryResetIntervalKey      = "telemetry-reset-interval-duration"
	TelemetryResetIntervalDefault  = "24h"
	TelemetryActiveIntervalKey     = "telemetry-active-interval-duration"
	TelemetryActiveIntervalDefault = "2m"
	TelemetryErrorsEnabledKey      = "telemetry-errors-enabled"
	TelemetryErrorEnabledDefault   = false
	EnvPrefix                      = "OTTERIZE"
)

func init() {
	viper.SetDefault(TelemetryAPIAddressKey, TelemetryAddressDefault)
	viper.SetDefault(TimeoutKey, CloudClientTimeoutDefault)
	viper.SetDefault(TelemetryIntervalKey, TelemetryIntervalDefault)
	viper.SetDefault(TelemetryMaxBatchSizeKey, TelemetryMaxBatchSizeDefault)
	viper.SetDefault(TelemetryUsageEnabledKey, TelemetryUsageEnabledDefault)
	viper.SetDefault(TelemetryResetIntervalKey, TelemetryResetIntervalDefault)
	viper.SetDefault(TelemetryActiveIntervalKey, TelemetryActiveIntervalDefault)
	viper.SetDefault(TelemetryErrorsEnabledKey, TelemetryErrorEnabledDefault)
	viper.SetEnvPrefix(EnvPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}

func IsTelemetryEnabled() bool {
	return viper.GetBool(TelemetryUsageEnabledKey)
}
