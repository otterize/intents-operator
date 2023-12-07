package telemetriesconfig

import (
	"github.com/spf13/viper"
	"strings"
)

const (
	TelemetryAPIAddressKey         = "telemetry-address"
	TimeoutKey                     = "telemetry-client-timeout"
	CloudClientTimeoutDefault      = "30s"
	TelemetryEnabledKey            = "telemetry-enabled"
	TelemetryEnabledDefault        = false
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
	TelemetryErrorsStageKey        = "telemetry-errors-stage"
	TelemetryErrorsStageDefault    = "production"
	TelemetryErrorsAddressKey      = "telemetry-errors-address"
	TelemetryErrorsAddressDefault  = "https://app.otterize.com/api/errors"
	TelemetryErrorsAPIKeyKey       = "telemetry-errors-api-key"
	TelemetryErrorsAPIKeyDefault   = ""
	EnvPrefix                      = "OTTERIZE"
)

func init() {
	viper.SetDefault(TelemetryAPIAddressKey, TelemetryAddressDefault)
	viper.SetDefault(TimeoutKey, CloudClientTimeoutDefault)
	viper.SetDefault(TelemetryIntervalKey, TelemetryIntervalDefault)
	viper.SetDefault(TelemetryMaxBatchSizeKey, TelemetryMaxBatchSizeDefault)
	viper.SetDefault(TelemetryEnabledKey, TelemetryEnabledDefault)
	viper.SetDefault(TelemetryUsageEnabledKey, TelemetryUsageEnabledDefault)
	viper.SetDefault(TelemetryResetIntervalKey, TelemetryResetIntervalDefault)
	viper.SetDefault(TelemetryActiveIntervalKey, TelemetryActiveIntervalDefault)
	viper.SetDefault(TelemetryErrorsEnabledKey, TelemetryErrorEnabledDefault)
	viper.SetDefault(TelemetryErrorsStageKey, TelemetryErrorsStageDefault)
	viper.SetDefault(TelemetryErrorsAddressKey, TelemetryErrorsAddressDefault)
	viper.SetDefault(TelemetryErrorsAPIKeyKey, TelemetryErrorsAPIKeyDefault)
	viper.SetEnvPrefix(EnvPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}

func IsUsageTelemetryEnabled() bool {
	return viper.GetBool(TelemetryEnabledKey) && viper.GetBool(TelemetryUsageEnabledKey)
}

func IsErrorsTelemetryEnabled() bool {
	return viper.GetBool(TelemetryEnabledKey) && viper.GetBool(TelemetryErrorsEnabledKey)
}
