package enforcement

import (
	"github.com/amit7itz/goset"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/automate_third_party_network_policy"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type Config struct {
	EnforcementDefaultState              bool
	EnableNetworkPolicy                  bool
	EnableKafkaACL                       bool
	EnableIstioPolicy                    bool
	EnableDatabasePolicy                 bool
	EnableEgressNetworkPolicyReconcilers bool
	EnableAWSPolicy                      bool
	EnableGCPPolicy                      bool
	EnableAzurePolicy                    bool
	EnableLinkerdPolicies                bool
	EnforcedNamespaces                   *goset.Set[string]
	AutomateThirdPartyNetworkPolicies    automate_third_party_network_policy.Enum
}

func (c Config) GetAutomateThirdPartyNetworkPolicy() automate_third_party_network_policy.Enum {
	// rewrite the above code to use a switch statement
	switch c.AutomateThirdPartyNetworkPolicies {
	case automate_third_party_network_policy.Off:
		return automate_third_party_network_policy.Off
	case automate_third_party_network_policy.Always:
		if !c.EnforcementDefaultState {
			// We don't want to create network policies for third parties when enforcement is disabled.
			// However, if one uses shadow mode we can still block third party traffic to his protected services
			// therefore we should return automate_third_party_network_policy.IfBlockedByOtterize
			return automate_third_party_network_policy.IfBlockedByOtterize
		}
		return automate_third_party_network_policy.Always
	default:
		return automate_third_party_network_policy.IfBlockedByOtterize
	}
}

const (
	ActiveEnforcementNamespacesKey              = "active-enforcement-namespaces"         // When using the "shadow enforcement" mode, namespaces in this list will be treated as if the enforcement were active
	AutomateThirdPartyNetworkPoliciesKey        = "automate-third-party-network-policies" // Whether to automatically create network policies for external traffic & metrics collection traffic
	AutomateThirdPartyNetworkPoliciesDefault    = string(automate_third_party_network_policy.IfBlockedByOtterize)
	EnforcementDefaultStateKey                  = "enforcement-default-state" // Sets the default state of the  If true, always enforces. If false, can be overridden using ProtectedService.
	EnforcementDefaultStateDefault              = true
	EnableNetworkPolicyKey                      = "enable-network-policy-creation" // Whether to enable Intents network policy creation
	EnableNetworkPolicyDefault                  = true
	EnableIstioPolicyKey                        = "enable-istio-policy-creation" // Whether to enable Istio authorization policy creation
	EnableIstioPolicyDefault                    = true
	EnableLinkerdPolicyKey                      = "enable-linkerd-policy"
	EnableLinkerdPolicyDefault                  = true
	EnableKafkaACLKey                           = "enable-kafka-acl-creation" // Whether to disable Intents Kafka ACL creation
	EnableKafkaACLDefault                       = true
	EnableDatabasePolicy                        = "enable-database-policy-creation" // Whether to enable the new database reconciler
	EnableDatabasePolicyDefault                 = true
	EnableEgressNetworkPolicyReconcilersKey     = "enable-egress-network-policies" // Enable the generation of egress network policies alongside ingress network policies
	EnableEgressNetworkPolicyReconcilersDefault = false
	EnableAWSPolicyKey                          = "enable-aws-iam-policy"
	EnableAWSPolicyDefault                      = false
	EnableGCPPolicyKey                          = "enable-gcp-iam-policy"
	EnableGCPPolicyDefault                      = false
	EnableAzurePolicyKey                        = "enable-azure-iam-policy"
	EnableAzurePolicyDefault                    = false
)

func init() {
	viper.SetDefault(EnforcementDefaultStateKey, EnforcementDefaultStateDefault)
	viper.SetDefault(EnableNetworkPolicyKey, EnableNetworkPolicyDefault)
	viper.SetDefault(EnableKafkaACLKey, EnableKafkaACLDefault)
	viper.SetDefault(ActiveEnforcementNamespacesKey, nil)
	viper.SetDefault(EnableIstioPolicyKey, EnableIstioPolicyDefault)
	viper.SetDefault(EnableDatabasePolicy, EnableDatabasePolicyDefault)
	viper.SetDefault(EnableEgressNetworkPolicyReconcilersKey, EnableEgressNetworkPolicyReconcilersDefault)
	viper.SetDefault(EnableAWSPolicyKey, EnableAWSPolicyDefault)
	viper.SetDefault(EnableGCPPolicyKey, EnableGCPPolicyDefault)
	viper.SetDefault(EnableAzurePolicyKey, EnableAzurePolicyDefault)
	viper.SetDefault(AutomateThirdPartyNetworkPoliciesKey, AutomateThirdPartyNetworkPoliciesDefault)
}

func InitCLIFlags() {
	pflag.Bool(EnforcementDefaultStateKey, EnforcementDefaultStateDefault, "Sets the default state of the enforcement. If true, always enforces. If false, can be overridden using ProtectedService.")
	pflag.Bool(EnableNetworkPolicyKey, EnableNetworkPolicyDefault, "Whether to enable Intents network policy creation")
	pflag.Bool(EnableKafkaACLKey, EnableKafkaACLDefault, "Whether to disable Intents Kafka ACL creation")
	pflag.StringSlice(ActiveEnforcementNamespacesKey, nil, "While using the shadow enforcement mode, namespaces in this list will be treated as if the enforcement were active.")
	pflag.Bool(EnableIstioPolicyKey, EnableIstioPolicyDefault, "Whether to enable Istio authorization policy creation")
	pflag.Bool(EnableLinkerdPolicyKey, EnableLinkerdPolicyDefault, "Experimental - enable Linkerd policy creation")
	pflag.Bool(EnableDatabasePolicy, EnableDatabasePolicyDefault, "Enable the database reconciler")
	pflag.Bool(EnableEgressNetworkPolicyReconcilersKey, EnableEgressNetworkPolicyReconcilersDefault, "Experimental - enable the generation of egress network policies alongside ingress network policies")
	pflag.Bool(EnableAWSPolicyKey, EnableAWSPolicyDefault, "Enable the AWS IAM reconciler")
	automateThirdPartyNetworkPoliciesKeyDefault := AutomateThirdPartyNetworkPoliciesDefault
	pflag.String(automateThirdPartyNetworkPoliciesKeyDefault, AutomateThirdPartyNetworkPoliciesKey, "Whether to automatically create network policies for third parties traffic, like external traffic and metrics collection traffic")
}

func GetConfig() Config {
	return Config{
		EnforcementDefaultState:              viper.GetBool(EnforcementDefaultStateKey),
		EnableNetworkPolicy:                  viper.GetBool(EnableNetworkPolicyKey),
		EnableKafkaACL:                       viper.GetBool(EnableKafkaACLKey),
		EnableIstioPolicy:                    viper.GetBool(EnableIstioPolicyKey),
		EnableLinkerdPolicies:                viper.GetBool(EnableLinkerdPolicyKey),
		EnableDatabasePolicy:                 viper.GetBool(EnableDatabasePolicy),
		EnableEgressNetworkPolicyReconcilers: viper.GetBool(EnableEgressNetworkPolicyReconcilersKey),
		EnableAWSPolicy:                      viper.GetBool(EnableAWSPolicyKey),
		EnableGCPPolicy:                      viper.GetBool(EnableGCPPolicyKey),
		EnableAzurePolicy:                    viper.GetBool(EnableAzurePolicyKey),
		EnforcedNamespaces:                   goset.FromSlice(viper.GetStringSlice(ActiveEnforcementNamespacesKey)),
		AutomateThirdPartyNetworkPolicies:    automate_third_party_network_policy.Enum(viper.GetString(AutomateThirdPartyNetworkPoliciesKey)),
	}
}
