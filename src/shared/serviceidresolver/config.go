package serviceidresolver

import "github.com/spf13/pflag"

const (
	serviceNameOverrideAnnotationKey        = "service-name-override-annotation"
	serviceNameOverrideAnnotationKeyDefault = "intents.otterize.com/service-name"
)

func init() {
	// To access pflags using viper, call viper.Bind after main() start: viper.BindPFlags(pflag.CommandLine))
	pflag.String(serviceNameOverrideAnnotationKey, serviceNameOverrideAnnotationKeyDefault, "Service name override annotation")
}
