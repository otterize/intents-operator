package serviceidresolver

import (
	"github.com/spf13/viper"
	"strings"
)

const (
	serviceNameOverrideAnnotationKey        = "service-name-override-annotation"
	serviceNameOverrideAnnotationKeyDefault = "intents.otterize.com/service-name"
	useImageNameForServiceIDForJobs         = "use-image-name-for-service-id-for-jobs"
	EnvPrefix                               = "OTTERIZE"
)

func init() {
	viper.SetDefault(serviceNameOverrideAnnotationKey, serviceNameOverrideAnnotationKeyDefault)
	viper.SetDefault(useImageNameForServiceIDForJobs, false)
	viper.SetEnvPrefix(EnvPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}
