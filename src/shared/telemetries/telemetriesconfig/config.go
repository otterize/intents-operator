package telemetriesconfig

import (
	"github.com/spf13/viper"
	"strings"
)

const (
	TimeoutKey                     = "telemetry-client-timeout"
	CloudClientTimeoutDefault      = "30s"
	TelemetryEnabledKey            = "telemetry-enabled"
	TelemetryEnabledDefault        = true
	TelemetryUsageEnabledKey       = "telemetry-usage-enabled"
	TelemetryUsageEnabledDefault   = true
	TelemetryMaxBatchSizeKey       = "telemetry-max-batch-size"
	TelemetryMaxBatchSizeDefault   = 100
	TelemetryIntervalKey           = "telemetry-interval-seconds"
	TelemetryIntervalDefault       = 5
	TelemetryResetIntervalKey      = "telemetry-reset-interval-duration"
	TelemetryResetIntervalDefault  = "24h"
	TelemetryActiveIntervalKey     = "telemetry-active-interval-duration"
	TelemetryActiveIntervalDefault = "2m"
	TelemetryErrorsEnabledKey      = "telemetry-errors-enabled"
	TelemetryErrorEnabledDefault   = true
	TelemetryErrorsStageKey        = "telemetry-errors-stage"
	TelemetryErrorsStageDefault    = "production"
	EnvPrefix                      = "OTTERIZE"
)

func init() {
	viper.SetDefault(TimeoutKey, CloudClientTimeoutDefault)
	viper.SetDefault(TelemetryIntervalKey, TelemetryIntervalDefault)
	viper.SetDefault(TelemetryMaxBatchSizeKey, TelemetryMaxBatchSizeDefault)
	viper.SetDefault(TelemetryEnabledKey, TelemetryEnabledDefault)
	viper.SetDefault(TelemetryUsageEnabledKey, TelemetryUsageEnabledDefault)
	viper.SetDefault(TelemetryResetIntervalKey, TelemetryResetIntervalDefault)
	viper.SetDefault(TelemetryActiveIntervalKey, TelemetryActiveIntervalDefault)
	viper.SetDefault(TelemetryErrorsEnabledKey, TelemetryErrorEnabledDefault)
	viper.SetDefault(TelemetryErrorsStageKey, TelemetryErrorsStageDefault)
	viper.SetEnvPrefix(EnvPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}

func IsUsageTelemetryEnabled() bool {
	return viper.GetBool(TelemetryEnabledKey) && viper.GetBool(TelemetryUsageEnabledKey)
}

func IsErrorTelemetryEnabled() bool {
	return viper.GetBool(TelemetryEnabledKey) && viper.GetBool(TelemetryErrorsEnabledKey)
}
