package otterizecloudclient

import (
	"github.com/spf13/pflag"
	"time"
)

const (
	ApiClientIdKey                 = "client-id"
	ApiClientSecretKey             = "client-secret"
	OtterizeAPIAddressKey          = "api-address"
	CloudClientTimeoutKey          = "cloud-client-timeout"
	OtterizeAPIExtraCAPEMPathsKey  = "api-extra-ca-pem"
	CloudClientTimeoutDefault      = 30 * time.Second
	OtterizeAPIAddressDefault      = "https://app.otterize.com/api"
	ComponentReportIntervalKey     = "component-report-interval"
	ComponentReportIntervalDefault = 60
)

func init() {
	// To access pflags using viper, call viper.Bind after main() start: viper.BindPFlags(pflag.CommandLine))
	pflag.String(ApiClientIdKey, "", "Otterize API client ID")
	pflag.String(ApiClientSecretKey, "", "Otterize API client secret")
	pflag.String(OtterizeAPIAddressKey, OtterizeAPIAddressDefault, "Otterize API address")
	pflag.Duration(CloudClientTimeoutKey, CloudClientTimeoutDefault, "Cloud client timeout")
	pflag.StringSlice(OtterizeAPIExtraCAPEMPathsKey, []string{}, "Otterize API extra CA PEM paths")
	pflag.Int(ComponentReportIntervalKey, ComponentReportIntervalDefault, "Component report interval")
}
