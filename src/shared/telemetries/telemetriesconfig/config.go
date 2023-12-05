package telemetriesconfig

import (
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"time"
)

const (
	TelemetryAPIAddressKey         = "telemetry-address"
	TimeoutKey                     = "telemetry-client-timeout"
	CloudClientTimeoutDefault      = 30 * time.Second
	TelemetryEnabledKey            = "telemetry-enabled"
	TelemetryEnabledDefault        = false
	TelemetryMaxBatchSizeKey       = "telemetry-max-batch-size"
	TelemetryMaxBatchSizeDefault   = 100
	TelemetryIntervalKey           = "telemetry-interval-seconds"
	TelemetryIntervalDefault       = 5
	TelemetryAddressDefault        = "https://app.otterize.com/api/telemetry/query"
	TelemetryResetIntervalKey      = "telemetry-reset-interval-duration"
	TelemetryResetIntervalDefault  = 24 * time.Hour
	TelemetryActiveIntervalKey     = "telemetry-active-interval-duration"
	TelemetryActiveIntervalDefault = 2 * time.Minute
)

func init() {
	// To access pflags using viper, call viper.Bind after main() start: viper.BindPFlags(pflag.CommandLine))
	pflag.String(TelemetryAPIAddressKey, TelemetryAddressDefault, "Telemetry API address")
	pflag.Duration(TimeoutKey, CloudClientTimeoutDefault, "Telemetry client timeout")
	pflag.Int(TelemetryIntervalKey, TelemetryIntervalDefault, "Telemetry interval seconds")
	pflag.Int(TelemetryMaxBatchSizeKey, TelemetryMaxBatchSizeDefault, "Telemetry max batch size")
	pflag.Bool(TelemetryEnabledKey, TelemetryEnabledDefault, "Telemetry enabled")
	pflag.Duration(TelemetryResetIntervalKey, TelemetryResetIntervalDefault, "Telemetry reset interval duration")
	pflag.Duration(TelemetryActiveIntervalKey, TelemetryActiveIntervalDefault, "Telemetry active interval duration")
}

func IsTelemetryEnabled() bool {
	return viper.GetBool(TelemetryEnabledKey)
}
