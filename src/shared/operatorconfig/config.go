package operatorconfig

import (
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetrysender"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/util/runtime"
	"os"
	"strings"
	"time"
)

const (
	MetricsAddrKey                                                      = "metrics-bind-address" // The address the metric endpoint binds to
	MetricsAddrDefault                                                  = ":8180"
	ProbeAddrKey                                                        = "health-probe-bind-address" // The address the probe endpoint binds to
	ProbeAddrDefault                                                    = ":8181"
	EnableLeaderElectionKey                                             = "leader-elect" // Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager
	EnableLeaderElectionDefault                                         = false
	WatchedNamespacesKey                                                = "watched-namespaces"    // Namespaces that will be watched by the operator. Specify multiple values by specifying multiple times or separate with commas
	KafkaServerTLSCertKey                                               = "kafka-server-tls-cert" // name of tls certificate file
	KafkaServerTLSKeyKey                                                = "kafka-server-tls-key"  // name of tls private key file
	KafkaServerTLSCAKey                                                 = "kafka-server-tls-ca"   // name of tls ca file
	SelfSignedCertKey                                                   = "self-signed-cert"      // Whether to generate and use a self signed cert as the CA for webhooks
	SelfSignedCertDefault                                               = true
	DisableWebhookServerKey                                             = "disable-webhook-server" // Disable webhook validator server
	DisableWebhookServerDefault                                         = false
	EnforcementDefaultStateKey                                          = "enforcement-default-state" // Sets the default state of the enforcement. If true, always enforces. If false, can be overridden using ProtectedService.
	EnforcementDefaultStateDefault                                      = true
	AutoCreateNetworkPoliciesForExternalTrafficKey                      = "auto-create-network-policies-for-external-traffic" // Whether to automatically create network policies for external traffic
	AutoCreateNetworkPoliciesForExternalTrafficDefault                  = true
	AutoCreateNetworkPoliciesForExternalTrafficNoIntentsRequiredKey     = "exp-auto-create-network-policies-for-external-traffic-disable-intents-requirement" // Whether to automatically create network policies for external traffic, even if no intents point to the relevant service
	AutoCreateNetworkPoliciesForExternalTrafficNoIntentsRequiredDefault = false
	EnableNetworkPolicyKey                                              = "enable-network-policy-creation" // Whether to enable Intents network policy creation
	EnableNetworkPolicyDefault                                          = true
	EnableIstioPolicyKey                                                = "enable-istio-policy-creation" // Whether to enable Istio authorization policy creation
	EnableIstioPolicyDefault                                            = true
	EnableKafkaACLKey                                                   = "enable-kafka-acl-creation" // Whether to disable Intents Kafka ACL creation
	EnableKafkaACLDefault                                               = true
	IntentsOperatorPodNameKey                                           = "pod-name"
	IntentsOperatorPodNamespaceKey                                      = "pod-namespace"
	EnvPrefix                                                           = "OTTERIZE"
	EnableDatabaseReconciler                                            = "enable-database-reconciler" // Whether to enable the new database reconciler
	EnableDatabaseReconcilerDefault                                     = false
	RetryDelayTimeKey                                                   = "retry-delay-time" // Default retry delay time for retrying failed requests
	RetryDelayTimeDefault                                               = 5 * time.Second
	DebugLogKey                                                         = "debug" // Whether to enable debug logging
	DebugLogDefault                                                     = false
	EnableKubernetesServiceIntentsKey                                   = "exp-enable-kubernetes-service-intents"
	EnableKubernetesServiceIntentsDefault                               = false
)

func init() {
	viper.SetDefault(MetricsAddrKey, MetricsAddrDefault)
	viper.SetDefault(ProbeAddrKey, ProbeAddrDefault)
	viper.SetDefault(EnableLeaderElectionKey, EnableLeaderElectionDefault)
	viper.SetDefault(SelfSignedCertKey, SelfSignedCertDefault)
	viper.SetDefault(EnforcementDefaultStateKey, EnforcementDefaultStateDefault)
	viper.SetDefault(AutoCreateNetworkPoliciesForExternalTrafficKey, AutoCreateNetworkPoliciesForExternalTrafficDefault)
	viper.SetDefault(EnableNetworkPolicyKey, EnableNetworkPolicyDefault)
	viper.SetDefault(EnableKafkaACLKey, EnableKafkaACLDefault)
	viper.SetDefault(EnableIstioPolicyKey, EnableIstioPolicyDefault)
	viper.SetDefault(DisableWebhookServerKey, DisableWebhookServerDefault)
	viper.SetDefault(EnableKubernetesServiceIntentsKey, EnableKubernetesServiceIntentsDefault)
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
	pflag.Bool(EnforcementDefaultStateKey, EnforcementDefaultStateDefault, "Sets the default state of the enforcement. If true, always enforces. If false, can be overridden using ProtectedService.")
	pflag.Bool(AutoCreateNetworkPoliciesForExternalTrafficKey, AutoCreateNetworkPoliciesForExternalTrafficDefault, "Whether to automatically create network policies for external traffic")
	pflag.Bool(AutoCreateNetworkPoliciesForExternalTrafficNoIntentsRequiredKey, AutoCreateNetworkPoliciesForExternalTrafficNoIntentsRequiredDefault, "Whether to create network policies for external traffic, even if no intents point to the relevant service")
	pflag.Bool(EnableNetworkPolicyKey, EnableNetworkPolicyDefault, "Whether to enable Intents network policy creation")
	pflag.Bool(EnableKafkaACLKey, EnableKafkaACLDefault, "Whether to disable Intents Kafka ACL creation")
	pflag.String(MetricsAddrKey, MetricsAddrDefault, "The address the metric endpoint binds to.")
	pflag.String(ProbeAddrKey, ProbeAddrDefault, "The address the probe endpoint binds to.")
	pflag.Bool(EnableLeaderElectionKey, EnableLeaderElectionDefault, "Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	pflag.StringSlice(WatchedNamespacesKey, nil, "Namespaces that will be watched by the operator. Specify multiple values by specifying multiple times or separate with commas.")
	pflag.Bool(EnableIstioPolicyKey, EnableIstioPolicyDefault, "Whether to enable Istio authorization policy creation")
	pflag.Bool(telemetrysender.TelemetryEnabledKey, telemetrysender.TelemetryEnabledDefault, "Whether telemetry should be enabled")
	pflag.Bool(EnableDatabaseReconciler, EnableDatabaseReconcilerDefault, "Enable the database reconciler")
	pflag.Bool(EnableKubernetesServiceIntentsKey, EnableKubernetesServiceIntentsDefault, "Experimental - enable Kubernetes service intents, which allow you to refer to services using their Kubernetes service name")
	pflag.Duration(RetryDelayTimeKey, RetryDelayTimeDefault, "Default retry delay time for retrying failed requests")
	pflag.Bool(DebugLogKey, DebugLogDefault, "Enable debug logging")

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
