package enforcement

import (
	"github.com/amit7itz/goset"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/allow_metrics_collection_traffic"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/allowexternaltraffic"
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
	AllowExternalTraffic                 allowexternaltraffic.Enum
	AllowMetricsCollectionTraffic        allow_metrics_collection_traffic.Enum
}

func (c Config) GetActualExternalTrafficPolicy() allowexternaltraffic.Enum {
	// rewrite the above code to use a switch statement
	switch c.AllowExternalTraffic {
	case allowexternaltraffic.Off:
		return allowexternaltraffic.Off
	case allowexternaltraffic.Always:
		if !c.EnforcementDefaultState {
			// We don't want to create network policies for external traffic when enforcement is disabled.
			// However, if one uses shadow mode we can still block external traffic to his protected services
			// therefore we should return allowexternaltraffic.IfBlockedByOtterize
			return allowexternaltraffic.IfBlockedByOtterize
		}
		return allowexternaltraffic.Always
	default:
		return allowexternaltraffic.IfBlockedByOtterize
	}
}

func (c Config) GetAllowMetricsCollectionTrafficPolicy() allow_metrics_collection_traffic.Enum {
	// rewrite the above code to use a switch statement
	switch c.AllowMetricsCollectionTraffic {
	case allow_metrics_collection_traffic.Off:
		return allow_metrics_collection_traffic.Off
	case allow_metrics_collection_traffic.Always:
		if !c.EnforcementDefaultState {
			// We don't want to create network policies for metrics collection traffic when enforcement is disabled.
			// However, if one uses shadow mode we can still block metrics collection traffic to his protected services
			// therefore we should return allow_metrics_collection_traffic.IfBlockedByOtterize
			return allow_metrics_collection_traffic.IfBlockedByOtterize
		}
		return allow_metrics_collection_traffic.Always
	default:
		return allow_metrics_collection_traffic.IfBlockedByOtterize
	}
}

const (
	ActiveEnforcementNamespacesKey              = "active-enforcement-namespaces" // When using the "shadow enforcement" mode, namespaces in this list will be treated as if the enforcement were active
	AllowExternalTrafficKey                     = "allow-external-traffic"        // Whether to automatically create network policies for external traffic
	AllowExternalTrafficDefault                 = string(allowexternaltraffic.IfBlockedByOtterize)
	AllowMetricsCollectionTrafficKey            = "allow-metrics-collection-traffic" // Whether to automatically create network policies for metrics collection traffic
	AllowMetricsCollectionTrafficDefault        = string(allow_metrics_collection_traffic.IfBlockedByOtterize)
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
	viper.SetDefault(AllowExternalTrafficKey, AllowExternalTrafficDefault)
	viper.SetDefault(AllowMetricsCollectionTrafficKey, AllowMetricsCollectionTrafficDefault)
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
	allowExternalTrafficDefault := AllowExternalTrafficDefault
	pflag.String(allowExternalTrafficDefault, AllowExternalTrafficKey, "Whether to automatically create network policies for external traffic")
	allowMetricsCollectionTrafficDefault := AllowMetricsCollectionTrafficDefault
	pflag.String(AllowMetricsCollectionTrafficKey, allowMetricsCollectionTrafficDefault, "Whether to automatically create network policies for metrics collection traffic")
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
		AllowExternalTraffic:                 allowexternaltraffic.Enum(viper.GetString(AllowExternalTrafficKey)),
		AllowMetricsCollectionTraffic:        allow_metrics_collection_traffic.Enum(viper.GetString(AllowMetricsCollectionTrafficKey)),
	}
}
