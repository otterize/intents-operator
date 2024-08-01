package operatorconfig

import (
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/allowexternaltraffic"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/enforcement"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesconfig"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/util/runtime"
	"os"
	"strings"
	"time"
)

const (
	MetricsAddrKey                         = "metrics-bind-address" // The address the metric endpoint binds to
	MetricsAddrDefault                     = ":2112"
	ProbeAddrKey                           = "health-probe-bind-address" // The address the probe endpoint binds to
	ProbeAddrDefault                       = ":8181"
	PprofBindAddressKey                    = "pprof-bind-address"
	PprofAddrDefault                       = "127.0.0.1:9001"
	EnableLeaderElectionKey                = "leader-elect" // Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager
	EnableLeaderElectionDefault            = false
	WatchedNamespacesKey                   = "watched-namespaces"    // Namespaces that will be watched by the operator. Specify multiple values by specifying multiple times or separate with commas
	KafkaServerTLSCertKey                  = "kafka-server-tls-cert" // name of tls certificate file
	KafkaServerTLSKeyKey                   = "kafka-server-tls-key"  // name of tls private key file
	KafkaServerTLSCAKey                    = "kafka-server-tls-ca"   // name of tls ca file
	SelfSignedCertKey                      = "self-signed-cert"      // Whether to generate and use a self signed cert as the CA for webhooks
	SelfSignedCertDefault                  = true
	DisableWebhookServerKey                = "disable-webhook-server" // Disable webhook validator server
	DisableWebhookServerDefault            = false
	AllowExternalTrafficKey                = "allow-external-traffic" // Whether to automatically create network policies for external traffic
	AllowExternalTrafficDefault            = allowexternaltraffic.IfBlockedByOtterize
	IntentsOperatorPodNameKey              = "pod-name"
	IntentsOperatorPodNamespaceKey         = "pod-namespace"
	EnvPrefix                              = "OTTERIZE"
	RetryDelayTimeKey                      = "retry-delay-time" // Default retry delay time for retrying failed requests
	RetryDelayTimeDefault                  = 5 * time.Second
	DebugLogKey                            = "debug" // Whether to enable debug logging
	DebugLogDefault                        = false
	EnableEgressAutoallowDNSTrafficKey     = "enable-egress-autoallow-dns-traffic" // Whether to automatically allow DNS traffic in egress network policies
	EnableEgressAutoallowDNSTrafficDefault = true
	EnableAWSRolesAnywhereKey              = "enable-aws-iam-rolesanywhere"
	EnableAWSRolesAnywhereDefault          = false
	AzureSubscriptionIDKey                 = "azure-subscription-id"
	AzureResourceGroupKey                  = "azure-resource-group"
	AzureAKSClusterNameKey                 = "azure-aks-cluster-name"
	EKSClusterNameOverrideKey              = "eks-cluster-name-override"
	AWSRolesAnywhereTrustAnchorARNKey      = "rolesanywhere-trust-anchor-arn"
	AWSRolesAnywhereSPIFFETrustDomainKey   = "rolesanywhere-spiffe-trust-domain"
	AWSRolesAnywhereClusterNameKey         = "rolesanywhere-cluster-name"
	AWSRolesAnywhereCertDirKey             = "rolesanywhere-cert-dir"
	AWSRolesAnywhereCertDirDefault         = "/aws-config"
	AWSRolesAnywherePrivKeyFilenameKey     = "rolesanywhere-priv-key-filename"
	AWSRolesAnywhereCertFilenameKey        = "rolesanywhere-cert-filename"
	AWSRolesAnywherePrivKeyFilenameDefault = "tls.key"
	AWSRolesAnywhereCertFilenameDefault    = "tls.crt"
	TelemetryErrorsAPIKeyKey               = "telemetry-errors-api-key"
	TelemetryErrorsAPIKeyDefault           = "60a78208a2b4fe714ef9fb3d3fdc0714"
	AWSAccountsKey                         = "aws"
	IngressControllerConfigKey             = "ingressControllers"
)

func init() {
	viper.SetDefault(MetricsAddrKey, MetricsAddrDefault)
	viper.SetDefault(ProbeAddrKey, ProbeAddrDefault)
	viper.SetDefault(EnableLeaderElectionKey, EnableLeaderElectionDefault)
	viper.SetDefault(SelfSignedCertKey, SelfSignedCertDefault)
	viper.SetDefault(enforcement.EnforcementDefaultStateKey, enforcement.EnforcementDefaultStateDefault)
	viper.SetDefault(AllowExternalTrafficKey, AllowExternalTrafficDefault)
	viper.SetDefault(enforcement.EnableNetworkPolicyKey, enforcement.EnableNetworkPolicyDefault)
	viper.SetDefault(enforcement.EnableKafkaACLKey, enforcement.EnableKafkaACLDefault)
	viper.SetDefault(enforcement.EnableIstioPolicyKey, enforcement.EnableIstioPolicyDefault)
	viper.SetDefault(DisableWebhookServerKey, DisableWebhookServerDefault)
	viper.SetDefault(enforcement.EnableEgressNetworkPolicyReconcilersKey, enforcement.EnableEgressNetworkPolicyReconcilersDefault)
	viper.SetDefault(enforcement.EnableAWSPolicyKey, enforcement.EnableAWSPolicyDefault)
	viper.SetDefault(EnableAWSRolesAnywhereKey, EnableAWSRolesAnywhereDefault)
	viper.SetDefault(enforcement.EnableGCPPolicyKey, enforcement.EnableGCPPolicyDefault)
	viper.SetDefault(enforcement.EnableAzurePolicyKey, enforcement.EnableAzurePolicyDefault)
	viper.SetDefault(TelemetryErrorsAPIKeyKey, TelemetryErrorsAPIKeyDefault)
	viper.SetDefault(AWSRolesAnywhereCertDirKey, AWSRolesAnywhereCertDirDefault)
	viper.SetDefault(AWSRolesAnywherePrivKeyFilenameKey, AWSRolesAnywherePrivKeyFilenameDefault)
	viper.SetDefault(AWSRolesAnywhereCertFilenameKey, AWSRolesAnywhereCertFilenameDefault)
	viper.SetDefault(KafkaServerTLSCertKey, "")
	viper.SetDefault(KafkaServerTLSKeyKey, "")
	viper.SetDefault(KafkaServerTLSCAKey, "")
	viper.SetEnvPrefix(EnvPrefix)
	viper.SetDefault(WatchedNamespacesKey, nil)
	viper.SetDefault(enforcement.ActiveEnforcementNamespacesKey, nil)
	viper.SetDefault(enforcement.EnableDatabasePolicy, enforcement.EnableDatabasePolicyDefault)
	viper.SetDefault(RetryDelayTimeKey, RetryDelayTimeDefault)
	viper.SetDefault(DebugLogKey, DebugLogDefault)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	viper.AddConfigPath("/etc/otterize")
	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			logrus.WithError(err).Panic("Failed to read config file")
		}
	}
}

type AWSAccount struct {
	RoleARN        string
	ProfileARN     string
	TrustAnchorARN string
	TrustDomain    string
}

func GetRolesAnywhereAWSAccounts() []AWSAccount {
	accts := make([]AWSAccount, 0)
	err := viper.UnmarshalKey(AWSAccountsKey, &accts)
	if err != nil {
		logrus.WithError(err).Panic("Failed to unmarshal AWS accounts")
	}
	return accts
}

type IngressControllerConfig struct {
	Name      string
	Namespace string
	Kind      string
}

func GetIngressControllerServiceIdentities() []serviceidentity.ServiceIdentity {
	controllers := make([]IngressControllerConfig, 0)
	err := viper.UnmarshalKey(IngressControllerConfigKey, &controllers)
	if err != nil {
		logrus.WithError(err).Panic("Failed to unmarshal ingress controller config")
	}

	identities := make([]serviceidentity.ServiceIdentity, 0)
	for _, controller := range controllers {
		identities = append(identities, serviceidentity.ServiceIdentity{
			Name:      controller.Name,
			Namespace: controller.Namespace,
			Kind:      controller.Kind,
		})
	}
	return identities
}

func InitCLIFlags() {
	// Backwards compatibility, new flags should be added to as ENV variables using viper
	pflag.String(KafkaServerTLSCertKey, "", "name of tls certificate file")
	pflag.String(KafkaServerTLSKeyKey, "", "name of tls private key file")
	pflag.String(KafkaServerTLSCAKey, "", "name of tls ca file")
	pflag.Bool(SelfSignedCertKey, SelfSignedCertDefault, "Whether to generate and use a self signed cert as the CA for webhooks")
	pflag.Bool(DisableWebhookServerKey, DisableWebhookServerDefault, "Disable webhook validator server")
	pflag.String(MetricsAddrKey, MetricsAddrDefault, "The address the metric endpoint binds to.")
	pflag.String(ProbeAddrKey, ProbeAddrDefault, "The address the probe endpoint binds to.")
	pflag.String(PprofBindAddressKey, PprofAddrDefault, "The address that the Go pprof profiler binds to.")
	pflag.Bool(EnableLeaderElectionKey, EnableLeaderElectionDefault, "Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	pflag.StringSlice(WatchedNamespacesKey, nil, "Namespaces that will be watched by the operator. Specify multiple values by specifying multiple times or separate with commas.")
	pflag.Bool(telemetriesconfig.TelemetryEnabledKey, telemetriesconfig.TelemetryEnabledDefault, "When set to false, all telemetries are disabled")
	pflag.Bool(telemetriesconfig.TelemetryUsageEnabledKey, telemetriesconfig.TelemetryUsageEnabledDefault, "Whether usage telemetry should be enabled")
	pflag.Bool(telemetriesconfig.TelemetryErrorsEnabledKey, telemetriesconfig.TelemetryErrorEnabledDefault, "Whether errors telemetry should be enabled")
	pflag.Bool(EnableEgressAutoallowDNSTrafficKey, EnableEgressAutoallowDNSTrafficDefault, "Whether to automatically allow DNS traffic in egress network policies")
	pflag.Duration(RetryDelayTimeKey, RetryDelayTimeDefault, "Default retry delay time for retrying failed requests")
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
