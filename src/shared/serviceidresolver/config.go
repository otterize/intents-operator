package serviceidresolver

import (
	"github.com/spf13/viper"
	"strings"
)

const (
	serviceNameOverrideAnnotationKey        = "service-name-override-annotation"
	serviceNameOverrideAnnotationKeyDefault = "intents.otterize.com/service-name"
	EnvPrefix                               = "OTTERIZE"
)

func init() {
	viper.SetDefault(serviceNameOverrideAnnotationKey, serviceNameOverrideAnnotationKeyDefault)
	viper.SetEnvPrefix(EnvPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}
