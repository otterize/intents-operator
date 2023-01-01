package otterizecloudclient

import (
	"github.com/spf13/viper"
	"strings"
)

const (
	ApiClientIdKey            = "client-id"
	ApiClientSecretKey        = "client-secret"
	OtterizeAPIAddressKey     = "api-address"
	OtterizeAPIAddressDefault = "https://app.otterize.com/api"
	EnvPrefix                 = "OTTERIZE"
)

func init() {
	viper.SetDefault(OtterizeAPIAddressKey, OtterizeAPIAddressDefault)
	viper.SetEnvPrefix(EnvPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}
