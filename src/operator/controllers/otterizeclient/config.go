package otterizeclient

import (
	"github.com/spf13/viper"
	"strings"
)

const (
	ApiClientIdKey                 = "client-id"
	ApiClientSecretKey             = "client-secret"
	OtterizeAPIAddressKey          = "api-address"
	OtterizeAPIAddressDefault      = "https://app.otterize.com"
	EnvPrefix                      = "OTTERIZE"
	ComponentReportIntervalKey     = "component-report-interval"
	ComponentReportIntervalDefault = 60
)

func init() {
	viper.SetDefault(OtterizeAPIAddressKey, OtterizeAPIAddressDefault)
	viper.SetDefault(ComponentReportIntervalKey, ComponentReportIntervalDefault)
	viper.SetEnvPrefix(EnvPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}
