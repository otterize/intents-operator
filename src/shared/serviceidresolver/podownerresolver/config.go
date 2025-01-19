package podownerresolver

import (
	"github.com/spf13/viper"
	"strings"
)

const (
	WorkloadNameOverrideAnnotationKey        = "workload-name-override-annotation"
	WorkloadNameOverrideAnnotationKeyDefault = "intents.otterize.com/workload-name"
	ServiceNameOverrideAnnotationDeprecated  = "intents.otterize.com/service-name"
	UseImageNameForServiceIDForJobs          = "use-image-name-for-service-id-for-jobs"
	EnvPrefix                                = "OTTERIZE"
)

func init() {
	viper.SetDefault(WorkloadNameOverrideAnnotationKey, WorkloadNameOverrideAnnotationKeyDefault)
	viper.SetDefault(UseImageNameForServiceIDForJobs, false)
	viper.SetEnvPrefix(EnvPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}
