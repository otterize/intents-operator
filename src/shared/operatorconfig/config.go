package operatorconfig

import (
	"github.com/otterize/intents-operator/src/shared/operatorconfig/allowexternaltraffic"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesconfig"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/util/runtime"
	"os"
	"strings"
	"time"
)

const (
	MetricsAddrKey                              = "metrics-bind-address" // The address the metric endpoint binds to
	MetricsAddrDefault                          = ":2112"
	ProbeAddrKey                                = "health-probe-bind-address" // The address the probe endpoint binds to
	ProbeAddrDefault                            = ":8181"
	EnableLeaderElectionKey                     = "leader-elect" // Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager
	EnableLeaderElectionDefault                 = false
	WatchedNamespacesKey                        = "watched-namespaces"    // Namespaces that will be watched by the operator. Specify multiple values by specifying multiple times or separate with commas
	KafkaServerTLSCertKey                       = "kafka-server-tls-cert" // name of tls certificate file
	KafkaServerTLSKeyKey                        = "kafka-server-tls-key"  // name of tls private key file
	KafkaServerTLSCAKey                         = "kafka-server-tls-ca"   // name of tls ca file
	SelfSignedCertKey                           = "self-signed-cert"      // Whether to generate and use a self signed cert as the CA for webhooks
	SelfSignedCertDefault                       = true
	DisableWebhookServerKey                     = "disable-webhook-server" // Disable webhook validator server
	DisableWebhookServerDefault                 = false
	EnforcementDefaultStateKey                  = "enforcement-default-state" // Sets the default state of the enforcement. If true, always enforces. If false, can be overridden using ProtectedService.
	EnforcementDefaultStateDefault              = true
	AllowExternalTrafficKey                     = "allow-external-traffic" // Whether to automatically create network policies for external traffic
	AllowExternalTrafficDefault                 = allowexternaltraffic.IfBlockedByOtterize
	EnableNetworkPolicyKey                      = "enable-network-policy-creation" // Whether to enable Intents network policy creation
	EnableNetworkPolicyDefault                  = true
	EnableIstioPolicyKey                        = "enable-istio-policy-creation" // Whether to enable Istio authorization policy creation
	EnableIstioPolicyDefault                    = true
	EnableKafkaACLKey                           = "enable-kafka-acl-creation" // Whether to disable Intents Kafka ACL creation
	EnableKafkaACLDefault                       = true
	IntentsOperatorPodNameKey                   = "pod-name"
	IntentsOperatorPodNamespaceKey              = "pod-namespace"
	EnvPrefix                                   = "OTTERIZE"
	EnableDatabasePolicy                        = "enable-database-policy-creation" // Whether to enable the new database reconciler
	EnableDatabasePolicyDefault                 = true
	RetryDelayTimeKey                           = "retry-delay-time" // Default retry delay time for retrying failed requests
	RetryDelayTimeDefault                       = 5 * time.Second
	DebugLogKey                                 = "debug" // Whether to enable debug logging
	DebugLogDefault                             = false
	EnableEgressNetworkPolicyReconcilersKey     = "exp-enable-egress-network-policies" // Experimental - enable the generation of egress network policies alongside ingress network policies
	EnableEgressNetworkPolicyReconcilersDefault = false
	EnableAWSPolicyKey                          = "enable-aws-iam-policy"
	EnableAWSPolicyDefault                      = false
	EnableGCPPolicyKey                          = "enable-gcp-iam-policy"
	EnableGCPPolicyDefault                      = false
	EnableAzurePolicyKey                        = "enable-azure-iam-policy"
	EnableAzurePolicyDefault                    = false
	EKSClusterNameOverrideKey                   = "eks-cluster-name-override"
	TelemetryErrorsAPIKeyKey                    = "telemetry-errors-api-key"
	TelemetryErrorsAPIKeyDefault                = "60a78208a2b4fe714ef9fb3d3fdc0714"
)

func init() {
	viper.SetDefault(MetricsAddrKey, MetricsAddrDefault)
	viper.SetDefault(ProbeAddrKey, ProbeAddrDefault)
	viper.SetDefault(EnableLeaderElectionKey, EnableLeaderElectionDefault)
	viper.SetDefault(SelfSignedCertKey, SelfSignedCertDefault)
	viper.SetDefault(EnforcementDefaultStateKey, EnforcementDefaultStateDefault)
	viper.SetDefault(AllowExternalTrafficKey, AllowExternalTrafficDefault)
	viper.SetDefault(EnableNetworkPolicyKey, EnableNetworkPolicyDefault)
	viper.SetDefault(EnableKafkaACLKey, EnableKafkaACLDefault)
	viper.SetDefault(EnableIstioPolicyKey, EnableIstioPolicyDefault)
	viper.SetDefault(DisableWebhookServerKey, DisableWebhookServerDefault)
	viper.SetDefault(EnableEgressNetworkPolicyReconcilersKey, EnableEgressNetworkPolicyReconcilersDefault)
	viper.SetDefault(EnableAWSPolicyKey, EnableAWSPolicyDefault)
	viper.SetDefault(TelemetryErrorsAPIKeyKey, TelemetryErrorsAPIKeyDefault)
	viper.SetDefault(KafkaServerTLSCertKey, "")
	viper.SetDefault(KafkaServerTLSKeyKey, "")
	viper.SetDefault(KafkaServerTLSCAKey, "")
	viper.SetEnvPrefix(EnvPrefix)
	viper.SetDefault(WatchedNamespacesKey, nil)
	viper.SetDefault(EnableDatabasePolicy, EnableDatabasePolicyDefault)
	viper.SetDefault(RetryDelayTimeKey, RetryDelayTimeDefault)
	viper.SetDefault(DebugLogKey, DebugLogDefault)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}

func InitCLIFlags() {
	// Backwards compatibility, new flags should be added to as ENV variables using viper
	pflag.String(KafkaServerTLSCertKey, "", "name of tls certificate file")
	pflag.String(KafkaServerTLSKeyKey, "", "name of tls private key file")
	pflag.String(KafkaServerTLSCAKey, "", "name of tls ca file")
	pflag.Bool(SelfSignedCertKey, SelfSignedCertDefault, "Whether to generate and use a self signed cert as the CA for webhooks")
	pflag.Bool(DisableWebhookServerKey, DisableWebhookServerDefault, "Disable webhook validator server")
	pflag.Bool(EnforcementDefaultStateKey, EnforcementDefaultStateDefault, "Sets the default state of the enforcement. If true, always enforces. If false, can be overridden using ProtectedService.")
	pflag.Bool(EnableNetworkPolicyKey, EnableNetworkPolicyDefault, "Whether to enable Intents network policy creation")
	pflag.Bool(EnableKafkaACLKey, EnableKafkaACLDefault, "Whether to disable Intents Kafka ACL creation")
	pflag.String(MetricsAddrKey, MetricsAddrDefault, "The address the metric endpoint binds to.")
	pflag.String(ProbeAddrKey, ProbeAddrDefault, "The address the probe endpoint binds to.")
	pflag.Bool(EnableLeaderElectionKey, EnableLeaderElectionDefault, "Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	pflag.StringSlice(WatchedNamespacesKey, nil, "Namespaces that will be watched by the operator. Specify multiple values by specifying multiple times or separate with commas.")
	pflag.Bool(EnableIstioPolicyKey, EnableIstioPolicyDefault, "Whether to enable Istio authorization policy creation")
	pflag.Bool(telemetriesconfig.TelemetryEnabledKey, telemetriesconfig.TelemetryEnabledDefault, "When set to false, all telemetries are disabled")
	pflag.Bool(telemetriesconfig.TelemetryUsageEnabledKey, telemetriesconfig.TelemetryUsageEnabledDefault, "Whether usage telemetry should be enabled")
	pflag.Bool(telemetriesconfig.TelemetryErrorsEnabledKey, telemetriesconfig.TelemetryErrorEnabledDefault, "Whether errors telemetry should be enabled")
	pflag.Bool(EnableDatabasePolicy, EnableDatabasePolicyDefault, "Enable the database reconciler")
	pflag.Bool(EnableEgressNetworkPolicyReconcilersKey, EnableEgressNetworkPolicyReconcilersDefault, "Experimental - enable the generation of egress network policies alongside ingress network policies")
	pflag.Duration(RetryDelayTimeKey, RetryDelayTimeDefault, "Default retry delay time for retrying failed requests")
	pflag.Bool(EnableAWSPolicyKey, EnableAWSPolicyDefault, "Enable the AWS IAM reconciler")
	pflag.Bool(DebugLogKey, DebugLogDefault, "Enable debug logging")

	allowExternalTrafficDefault := AllowExternalTrafficDefault
	pflag.Var(&allowExternalTrafficDefault, AllowExternalTrafficKey, "Whether to automatically create network policies for external traffic")
	runtime.Must(viper.BindPFlags(pflag.CommandLine))

	pflag.Parse()

	// Backwards compatibility for ENV variables without prefix
	if !viper.IsSet(IntentsOperatorPodNameKey) {
		podName := os.Getenv("POD_NAME")
		viper.Set(IntentsOperatorPodNameKey, podName)
	}

	if !viper.IsSet(IntentsOperatorPodNamespaceKey) {
		podNamespace := os.Getenv("POD_NAMESPACE")
		viper.Set(IntentsOperatorPodNamespaceKey, podNamespace)
	}
}
