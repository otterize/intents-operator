package awsagent

import "github.com/spf13/viper"

const (
	AWSIntentsEnabledKey      = "AWS_INTENTS_ENABLED"
	AWSIntentsEnabledDefault  = false
	ClusterOIDCProviderUrlKey = "OIDC_URL"
)

func init() {
	viper.SetDefault(AWSIntentsEnabledKey, AWSIntentsEnabledDefault)
}
