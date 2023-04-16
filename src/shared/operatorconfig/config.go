package operatorconfig

import (
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/util/runtime"
	"os"
	"strings"
)

const (
	MetricsAddrKey                                     = "metrics-bind-address" // The address the metric endpoint binds to
	MetricsAddrDefault                                 = ":8080"
	ProbeAddrKey                                       = "health-probe-bind-address" // The address the probe endpoint binds to
	ProbeAddrDefault                                   = ":8081"
	EnableLeaderElectionKey                            = "leader-elect" // Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager
	EnableLeaderElectionDefault                        = false
	WatchedNamespacesKey                               = "watched-namespaces"    // Namespaces that will be watched by the operator. Specify multiple values by specifying multiple times or separate with commas
	KafkaServerTLSCertKey                              = "kafka-server-tls-cert" // name of tls certificate file
	KafkaServerTLSKeyKey                               = "kafka-server-tls-key"  // name of tls private key file
	KafkaServerTLSCAKey                                = "kafka-server-tls-ca"   // name of tls ca file
	SelfSignedCertKey                                  = "self-signed-cert"      // Whether to generate and use a self signed cert as the CA for webhooks
	SelfSignedCertDefault                              = true
	DisableWebhookServerKey                            = "disable-webhook-server" // Disable webhook validator server
	DisableWebhookServerDefault                        = false
	EnforcementEnabledGloballyKey                      = "enable-enforcement" // If set to false disables the enforcement globally, superior to the other flags
	EnforcementEnabledGloballyDefault                  = true
	AutoCreateNetworkPoliciesForExternalTrafficKey     = "auto-create-network-policies-for-external-traffic" // Whether to automatically create network policies for external traffic
	AutoCreateNetworkPoliciesForExternalTrafficDefault = true
	EnableNetworkPolicyKey                             = "enable-network-policy-creation" // Whether to enable Intents network policy creation
	EnableNetworkPolicyDefault                         = true
	EnableIstioPolicyKey                               = "experimental-enable-istio-policy-creation" // Whether to enable istio authorization policy creation
	EnableIstioPolicyDefault                           = false
	EnableKafkaACLKey                                  = "enable-kafka-acl-creation" // Whether to disable Intents Kafka ACL creation
	EnableKafkaACLDefault                              = true
	IntentsOperatorPodNameKey                          = "pod-name"
	IntentsOperatorPodNamespaceKey                     = "pod-namespace"
	EnvPrefix                                          = "OTTERIZE"
)

func init() {
	viper.SetDefault(MetricsAddrKey, MetricsAddrDefault)
	viper.SetDefault(ProbeAddrKey, ProbeAddrDefault)
	viper.SetDefault(EnableLeaderElectionKey, EnableLeaderElectionDefault)
	viper.SetDefault(SelfSignedCertKey, SelfSignedCertDefault)
	viper.SetDefault(EnforcementEnabledGloballyKey, EnforcementEnabledGloballyDefault)
	viper.SetDefault(AutoCreateNetworkPoliciesForExternalTrafficKey, AutoCreateNetworkPoliciesForExternalTrafficDefault)
	viper.SetDefault(EnableNetworkPolicyKey, EnableNetworkPolicyDefault)
	viper.SetDefault(EnableKafkaACLKey, EnableKafkaACLDefault)
	viper.SetDefault(EnableIstioPolicyKey, EnableIstioPolicyDefault)
	viper.SetDefault(DisableWebhookServerKey, DisableWebhookServerDefault)
	viper.SetEnvPrefix(EnvPrefix)
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
	pflag.Bool(EnforcementEnabledGloballyKey, EnforcementEnabledGloballyDefault, "If set to false disables the enforcement globally, superior to the other flags")
	pflag.Bool(AutoCreateNetworkPoliciesForExternalTrafficKey, AutoCreateNetworkPoliciesForExternalTrafficDefault, "Whether to automatically create network policies for external traffic")
	pflag.Bool(EnableNetworkPolicyKey, EnableNetworkPolicyDefault, "Whether to enable Intents network policy creation")
	pflag.Bool(EnableKafkaACLKey, EnableKafkaACLDefault, "Whether to disable Intents Kafka ACL creation")
	pflag.String(MetricsAddrKey, MetricsAddrDefault, "The address the metric endpoint binds to.")
	pflag.String(ProbeAddrKey, ProbeAddrDefault, "The address the probe endpoint binds to.")
	pflag.Bool(EnableLeaderElectionKey, EnableLeaderElectionDefault, "Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	pflag.StringSlice(WatchedNamespacesKey, nil, "Namespaces that will be watched by the operator. Specify multiple values by specifying multiple times or separate with commas.")
	pflag.Bool(EnableIstioPolicyKey, EnableIstioPolicyDefault, "Whether to enable Istio authorization policy creation")

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
