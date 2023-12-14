package operatorconfig

import (
	"github.com/spf13/viper"
	"strings"
)

const (
	TelemetryErrorsAPIKeyKey     = "telemetry-errors-api-key"
	TelemetryErrorsAPIKeyDefault = "20b1b74678347375fedfdba65171acb2"

	EnvPrefix = "OTTERIZE"
)

func init() {
	viper.SetDefault(TelemetryErrorsAPIKeyKey, TelemetryErrorsAPIKeyDefault)
	viper.SetEnvPrefix(EnvPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}
