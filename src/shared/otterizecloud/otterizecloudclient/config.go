package otterizecloudclient

import (
	"github.com/spf13/viper"
	"strings"
)

const (
	ApiClientIdKey                      = "client-id"
	ApiClientSecretKey                  = "client-secret"
	OtterizeAPIAddressKey               = "api-address"
	CloudClientTimeoutKey               = "cloud-client-timeout"
	OtterizeAPIExtraCAPEMPathsKey       = "api-extra-ca-pem"
	CloudClientTimeoutDefault           = "30s"
	OtterizeAPIAddressDefault           = "https://app.otterize.com/api"
	ComponentReportIntervalKey          = "component-report-interval"
	ComponentReportIntervalDefault      = 60
	OperatorConfigReportIntervalKey     = "operator-config-report-interval"
	OperatorConfigReportIntervalDefault = 60
	IntentEventsReportIntervalKey       = "intent-events-report-interval"
	IntentEventsReportIntervalDefault   = 60
	IntentStatusReportIntervalKey       = "intent-status-report-interval"
	IntentStatusReportIntervalDefault   = 60
	IntentsApprovalQueryIntervalKey     = "intents-approval-query-interval"
	IntentsApprovalQueryIntervalDefault = 5
	EnvPrefix                           = "OTTERIZE"
)

func init() {
	viper.SetDefault(OtterizeAPIAddressKey, OtterizeAPIAddressDefault)
	viper.SetDefault(ComponentReportIntervalKey, ComponentReportIntervalDefault)
	viper.SetDefault(OperatorConfigReportIntervalKey, OperatorConfigReportIntervalDefault)
	viper.SetDefault(CloudClientTimeoutKey, CloudClientTimeoutDefault)
	viper.SetDefault(OtterizeAPIExtraCAPEMPathsKey, []string{})
	viper.SetDefault(IntentEventsReportIntervalKey, IntentEventsReportIntervalDefault)
	viper.SetDefault(IntentStatusReportIntervalKey, IntentStatusReportIntervalDefault)
	viper.SetDefault(IntentsApprovalQueryIntervalKey, IntentsApprovalQueryIntervalDefault)
	viper.SetEnvPrefix(EnvPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}
