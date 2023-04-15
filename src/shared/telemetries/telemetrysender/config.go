package telemetrysender

import (
	"github.com/spf13/viper"
	"strings"
)

const (
	TelemetriesAPIAddressKey       = "telemetries-address"
	TimeoutKey                     = "telemetry-client-timeout"
	CloudClientTimeoutDefault      = "30s"
	TelemetriesEnabledKey          = "telemetries-enabled"
	TelemetriesEnabledDefault      = true
	TelemetriesMaxBatchSizeKey     = "telemetries-max-batch-size"
	TelemetriesMaxBatchSizeDefault = 100
	TelemetriesIntervalKey         = "telemetries-interval-seconds"
	TelemetriesIntervalDefault     = 5
	//TelemetriesAddressDefault = "https://app.otterize.com/api/telemetry/query"
	TelemetriesAddressDefault = "http://app-omris94-cloud-pr903.staging.otterize.com/api/telemetry/query"
	EnvPrefix                 = "OTTERIZE"
)

func init() {
	viper.SetDefault(TelemetriesAPIAddressKey, TelemetriesAddressDefault)
	viper.SetDefault(TimeoutKey, CloudClientTimeoutDefault)
	viper.SetDefault(TelemetriesIntervalKey, TelemetriesIntervalDefault)
	viper.SetDefault(TelemetriesMaxBatchSizeKey, TelemetriesMaxBatchSizeDefault)
	viper.SetDefault(TelemetriesEnabledKey, TelemetriesEnabledDefault)
	viper.SetEnvPrefix(EnvPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}
