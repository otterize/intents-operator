package otterizecloudclient

import (
	"github.com/spf13/viper"
	"strings"
)

const (
	ApiClientIdKey                 = "client-id"
	ApiClientSecretKey             = "client-secret"
	OtterizeAPIAddressKey          = "api-address"
	OtterizeAPIAddressDefault      = "https://app.otterize.com/api"
	ComponentReportIntervalKey     = "component-report-interval"
	ComponentReportIntervalDefault = 60
	EnvPrefix                      = "OTTERIZE"
)

func init() {
	viper.SetDefault(OtterizeAPIAddressKey, OtterizeAPIAddressDefault)
	viper.SetDefault(ComponentReportIntervalKey, ComponentReportIntervalDefault)
	viper.SetEnvPrefix(EnvPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}
