/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"path"
	"time"

	"context"
	"github.com/bombsimon/logrusr/v3"
	linkerdauthscheme "github.com/linkerd/linkerd2/controller/gen/apis/policy/v1alpha1"
	linkerdserverscheme "github.com/linkerd/linkerd2/controller/gen/apis/server/v1beta1"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	otterizev1beta1 "github.com/otterize/intents-operator/src/operator/api/v1beta1"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	otterizev2beta1 "github.com/otterize/intents-operator/src/operator/api/v2beta1"
	"github.com/otterize/intents-operator/src/operator/controllers"
	"github.com/otterize/intents-operator/src/operator/controllers/external_traffic"
	"github.com/otterize/intents-operator/src/operator/controllers/iam_pod_reconciler"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/iam"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/iam/iampolicyagents"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/iam/iampolicyagents/awspolicyagent"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/iam/iampolicyagents/azurepolicyagent"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/iam/iampolicyagents/gcppolicyagent"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/networkpolicy"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/networkpolicy/builders"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/port_network_policy"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	"github.com/otterize/intents-operator/src/operator/controllers/pod_reconcilers"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/operator/health"
	"github.com/otterize/intents-operator/src/operator/otterizecrds"
	"github.com/otterize/intents-operator/src/operator/webhooks"
	"github.com/otterize/intents-operator/src/shared"
	"github.com/otterize/intents-operator/src/shared/awsagent"
	"github.com/otterize/intents-operator/src/shared/azureagent"
	"github.com/otterize/intents-operator/src/shared/clusterutils"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/filters"
	"github.com/otterize/intents-operator/src/shared/gcpagent"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/enforcement"
	"github.com/otterize/intents-operator/src/shared/reconcilergroup"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/intents-operator/src/shared/telemetries/componentinfo"
	"github.com/otterize/intents-operator/src/shared/telemetries/errorreporter"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesconfig"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetrysender"
	"github.com/otterize/intents-operator/src/shared/version"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	istiosecurityscheme "istio.io/client-go/pkg/apis/security/v1beta1"

	gcpiamv1 "github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/iam/v1beta1"
	gcpk8sv1 "github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/k8s/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	//+kubebuilder:scaffold:imports
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(istiosecurityscheme.AddToScheme(scheme))
	utilruntime.Must(otterizev1alpha2.AddToScheme(scheme))
	utilruntime.Must(otterizev1alpha3.AddToScheme(scheme))
	utilruntime.Must(otterizev1beta1.AddToScheme(scheme))
	utilruntime.Must(otterizev2alpha1.AddToScheme(scheme))
	utilruntime.Must(otterizev2beta1.AddToScheme(scheme))
	utilruntime.Must(linkerdauthscheme.AddToScheme(scheme))
	utilruntime.Must(linkerdserverscheme.AddToScheme(scheme))

	// Config Connector CRDs
	utilruntime.Must(gcpiamv1.AddToScheme(scheme))
	utilruntime.Must(gcpk8sv1.AddToScheme(scheme))

	//+kubebuilder:scaffold:scheme
}

func MustGetEnvVar(name string) string {
	value := viper.GetString(name)
	if value == "" {
		logrus.Fatalf("%s environment variable is required", name)
	}

	return value
}

func main() {
	operatorconfig.InitCLIFlags()
	logrus.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339,
	})
	if viper.GetBool(operatorconfig.DebugLogKey) {
		logrus.SetLevel(logrus.DebugLevel)
	}

	signalHandlerCtx := ctrl.SetupSignalHandler()

	clusterUID := clusterutils.GetOrCreateClusterUID(signalHandlerCtx)
	componentinfo.SetGlobalContextId(telemetrysender.Anonymize(clusterUID))

	errorreporter.Init(telemetriesgql.TelemetryComponentTypeIntentsOperator, version.Version())
	defer errorreporter.AutoNotify()
	shared.RegisterPanicHandlers()

	metricsAddr := viper.GetString(operatorconfig.MetricsAddrKey)
	probeAddr := viper.GetString(operatorconfig.ProbeAddrKey)
	enableLeaderElection := viper.GetBool(operatorconfig.EnableLeaderElectionKey)
	selfSignedCert := viper.GetBool(operatorconfig.SelfSignedCertKey)
	watchedNamespaces := viper.GetStringSlice(operatorconfig.WatchedNamespacesKey)
	enforcementConfig := enforcement.GetConfig()
	disableWebhookServer := viper.GetBool(operatorconfig.DisableWebhookServerKey)
	tlsSource := otterizev2alpha1.TLSSource{
		CertFile:   viper.GetString(operatorconfig.KafkaServerTLSCertKey),
		KeyFile:    viper.GetString(operatorconfig.KafkaServerTLSKeyKey),
		RootCAFile: viper.GetString(operatorconfig.KafkaServerTLSCAKey),
	}

	podName := MustGetEnvVar(operatorconfig.IntentsOperatorPodNameKey)
	podNamespace := MustGetEnvVar(operatorconfig.IntentsOperatorPodNamespaceKey)
	ctrl.SetLogger(logrusr.New(logrus.StandardLogger()))

	options := ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port:    9443,
			CertDir: webhooks.CertDirPath,
		}),
		HealthProbeBindAddress: probeAddr,
		PprofBindAddress:       viper.GetString(operatorconfig.PprofBindAddressKey),
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "a3a7d614.otterize.com",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	}

	if len(watchedNamespaces) != 0 {
		for _, namespace := range watchedNamespaces {
			options.Cache.DefaultNamespaces[namespace] = cache.Config{}
		}
		logrus.Infof("Will only watch the following namespaces: %v", watchedNamespaces)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		logrus.WithError(err).Panic(err, "unable to start manager")
	}

	directClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		logrus.WithError(err).Panic("unable to create kubernetes API client")
	}

	kafkaServersStore := kafkaacls.NewServersStore(tlsSource, enforcementConfig.EnableKafkaACL, kafkaacls.NewKafkaIntentsAdmin, enforcementConfig.EnforcementDefaultState)

	extNetpolHandler := external_traffic.NewNetworkPolicyHandler(mgr.GetClient(), mgr.GetScheme(), enforcementConfig.GetActualExternalTrafficPolicy(), operatorconfig.GetIngressControllerServiceIdentities(), viper.GetBool(operatorconfig.IngressControllerALBExemptKey))
	endpointReconciler := external_traffic.NewEndpointsReconciler(mgr.GetClient(), extNetpolHandler)
	ingressRulesBuilder := builders.NewIngressNetpolBuilder()

	serviceIdResolver := serviceidresolver.NewResolver(mgr.GetClient())

	additionalIntentsReconcilers := make([]reconcilergroup.ReconcilerWithEvents, 0)
	svcNetworkPolicyBuilder := builders.NewPortNetworkPolicyReconciler(mgr.GetClient())
	dnsServerNetpolBuilder := builders.NewIngressDNSServerAutoAllowNetpolBuilder()
	epNetpolReconciler :=
		networkpolicy.NewReconciler(
			mgr.GetClient(), scheme, extNetpolHandler, watchedNamespaces,
			enforcementConfig.EnforcedNamespaces, enforcementConfig.EnableNetworkPolicy, enforcementConfig.EnforcementDefaultState,
			viper.GetBool(operatorconfig.SeparateNetpolsForIngressAndEgress),
			[]networkpolicy.IngressRuleBuilder{ingressRulesBuilder, svcNetworkPolicyBuilder, dnsServerNetpolBuilder},
			make([]networkpolicy.EgressRuleBuilder, 0))
	epGroupReconciler := effectivepolicy.NewGroupReconciler(mgr.GetClient(), scheme, serviceIdResolver, epNetpolReconciler)

	if enforcementConfig.EnableLinkerdPolicies {
		epGroupReconciler.AddReconciler(intents_reconcilers.NewLinkerdReconciler(mgr.GetClient(), scheme, watchedNamespaces, enforcementConfig.EnforcementDefaultState))
	}

	if enforcementConfig.EnableEgressNetworkPolicyReconcilers {
		egressNetworkPolicyHandler := builders.NewEgressNetworkPolicyBuilder()
		epNetpolReconciler.AddEgressRuleBuilder(egressNetworkPolicyHandler)
		internetNetpolReconciler := builders.NewInternetEgressRulesBuilder()
		epNetpolReconciler.AddEgressRuleBuilder(internetNetpolReconciler)
		svcEgressNetworkPolicyHandler := builders.NewPortEgressRulesBuilder(mgr.GetClient())
		epNetpolReconciler.AddEgressRuleBuilder(svcEgressNetworkPolicyHandler)
		if viper.GetBool(operatorconfig.EnableEgressAutoallowDNSTrafficKey) {
			dnsEgressNetpolBuilder := builders.NewDNSEgressNetworkPolicyBuilder()
			epNetpolReconciler.AddEgressRuleBuilder(dnsEgressNetpolBuilder)
		}
	}

	epIntentsReconciler := intents_reconcilers.NewServiceEffectiveIntentsReconciler(mgr.GetClient(), scheme, epGroupReconciler)
	additionalIntentsReconcilers = append(additionalIntentsReconcilers, epIntentsReconciler)

	var iamAgents []iampolicyagents.IAMPolicyAgent

	if enforcementConfig.EnableAWSPolicy {
		awsOptions := make([]awsagent.Option, 0)
		if viper.GetBool(operatorconfig.EnableAWSRolesAnywhereKey) {
			keyPath := path.Join(viper.GetString(operatorconfig.AWSRolesAnywhereCertDirKey), viper.GetString(operatorconfig.AWSRolesAnywherePrivKeyFilenameKey))
			certPath := path.Join(viper.GetString(operatorconfig.AWSRolesAnywhereCertDirKey), viper.GetString(operatorconfig.AWSRolesAnywhereCertFilenameKey))
			clusterName := viper.GetString(operatorconfig.AWSRolesAnywhereClusterNameKey)
			accounts := operatorconfig.GetRolesAnywhereAWSAccounts()

			if len(accounts) == 0 {
				logrus.Panic("No AWS accounts configured even though RolesAnywhere is enabled")
			}

			if len(accounts) == 1 {
				awsOptions = append(awsOptions, awsagent.WithRolesAnywhere(accounts[0], clusterName, keyPath, certPath))
				awsAgent, err := awsagent.NewAWSAgent(signalHandlerCtx, awsOptions...)
				if err != nil {
					logrus.WithError(err).Panic("Could not initialize AWS agent")
				}
				awsIntentsAgent := awspolicyagent.NewAWSPolicyAgent(awsAgent)

				iamAgents = append(iamAgents, awsIntentsAgent)
			} else {
				awsIntentsAgent, err := awspolicyagent.NewMultiaccountAWSPolicyAgent(signalHandlerCtx, accounts, clusterName, keyPath, certPath)
				if err != nil {
					logrus.WithError(err).Panic("Could not initialize AWS agent")
				}
				iamAgents = append(iamAgents, awsIntentsAgent)
			}
		} else {
			awsAgent, err := awsagent.NewAWSAgent(signalHandlerCtx, awsOptions...)
			if err != nil {
				logrus.WithError(err).Panic("Could not initialize AWS agent")
			}
			awsIntentsAgent := awspolicyagent.NewAWSPolicyAgent(awsAgent)

			iamAgents = append(iamAgents, awsIntentsAgent)
		}
	}

	if enforcementConfig.EnableGCPPolicy {
		gcpAgent, err := gcpagent.NewGCPAgent(signalHandlerCtx, mgr.GetClient())
		if err != nil {
			logrus.WithError(err).Panic("Could not initialize GCP agent")
		}
		gcpIntentsAgent := gcppolicyagent.NewGCPPolicyAgent(gcpAgent)
		iamAgents = append(iamAgents, gcpIntentsAgent)
	}

	if enforcementConfig.EnableAzurePolicy {
		config := azureagent.Config{
			SubscriptionID: MustGetEnvVar(operatorconfig.AzureSubscriptionIDKey),
			ResourceGroup:  MustGetEnvVar(operatorconfig.AzureResourceGroupKey),
			AKSClusterName: MustGetEnvVar(operatorconfig.AzureAKSClusterNameKey),
		}

		azureAgent, err := azureagent.NewAzureAgent(signalHandlerCtx, config)
		if err != nil {
			logrus.WithError(err).Panic("could not initialize Azure agent")
		}
		azureIntentsAgent := azurepolicyagent.NewAzurePolicyAgent(azureAgent)

		iamAgents = append(iamAgents, azureIntentsAgent)
	}

	for _, iamAgent := range iamAgents {
		iamIntentsReconciler := iam.NewIAMIntentsReconciler(mgr.GetClient(), scheme, serviceIdResolver, iamAgent)
		additionalIntentsReconcilers = append(additionalIntentsReconcilers, iamIntentsReconciler)

		iamPodWatcher := iam_pod_reconciler.NewIAMPodReconciler(mgr.GetClient(), mgr.GetEventRecorderFor("intents-operator"), iamIntentsReconciler)
		err = iamPodWatcher.SetupWithManager(mgr)

		if err != nil {
			logrus.WithError(err).Panic("unable to register pod watcher")
		}
	}

	if err = endpointReconciler.InitIngressReferencedServicesIndex(mgr); err != nil {
		logrus.WithError(err).Panic("unable to init index for ingress")
	}

	otterizeCloudClient, connectedToCloud, err := operator_cloud_client.NewClient(signalHandlerCtx)
	if err != nil {
		logrus.WithError(err).Error("Failed to initialize Otterize Cloud client")
	}
	if connectedToCloud {
		operator_cloud_client.StartPeriodicCloudReports(signalHandlerCtx, otterizeCloudClient, mgr.GetClient())
		intentsEventsSender, err := operator_cloud_client.NewIntentEventsSender(otterizeCloudClient, mgr)
		if err != nil {
			logrus.WithError(err).Panic("unable to create intent events sender")
		}
		err = intentsEventsSender.Start(signalHandlerCtx)
		if err != nil {
			logrus.WithError(err).Panic("unable to start intent events sender")
		}

		serviceUploadReconciler := external_traffic.NewServiceUploadReconciler(mgr.GetClient(), otterizeCloudClient)
		ingressUploadReconciler := external_traffic.NewIngressUploadReconciler(mgr.GetClient(), otterizeCloudClient)

		if err = serviceUploadReconciler.SetupWithManager(mgr); err != nil {
			logrus.WithError(err).Panic("unable to create controller", "controller", "Endpoints")
		}

		if err = ingressUploadReconciler.SetupWithManager(mgr); err != nil {
			logrus.WithError(err).Panic("unable to create controller", "controller", "Ingress")
		}
	} else {
		logrus.Info("Not configured for cloud integration")
	}

	externalPolicySvcReconciler := external_traffic.NewServiceReconciler(mgr.GetClient(), extNetpolHandler)
	ingressReconciler := external_traffic.NewIngressReconciler(mgr.GetClient(), extNetpolHandler)
	netpolReconciler := external_traffic.NewNetworkPolicyReconciler(mgr.GetClient(), extNetpolHandler)

	if !enforcementConfig.EnforcementDefaultState {
		logrus.Infof("Running with enforcement disabled globally, won't perform any enforcement")
	}

	if selfSignedCert {
		logrus.Infoln("Creating self signing certs")
		certBundle, err :=
			webhooks.GenerateSelfSignedCertificate("intents-operator-webhook-service", podNamespace)
		if err != nil {
			logrus.WithError(err).Panic("unable to create self signed certs for webhook")
		}
		err = webhooks.WriteCertToFiles(certBundle)
		if err != nil {
			logrus.WithError(err).Panic("failed writing certs to file system")
		}

		err = otterizecrds.Ensure(signalHandlerCtx, directClient, podNamespace, certBundle.CertPem)
		if err != nil {
			logrus.WithError(err).Panic("unable to ensure otterize CRDs")
		}

		validatingWebhookConfigsReconciler := controllers.NewValidatingWebhookConfigsReconciler(
			mgr.GetClient(),
			mgr.GetScheme(),
			certBundle.CertPem,
			filters.PartOfOtterizeLabelPredicate(),
		)
		err = validatingWebhookConfigsReconciler.SetupWithManager(mgr)
		if err != nil {
			logrus.WithError(err).Panic("unable to create controller", "controller", "ValidatingWebhookConfigs")
		}
		//
		//customResourceDefinitionsReconciler := controllers.NewCustomResourceDefinitionsReconciler(
		//	mgr.GetClient(),
		//	mgr.GetScheme(),
		//	certBundle.CertPem,
		//	podNamespace,
		//)
		//err = customResourceDefinitionsReconciler.SetupWithManager(mgr)
		//if err != nil {
		//	logrus.WithError(err).Panic("unable to create controller", "controller", "CustomResourceDefinition")
		//}
	}

	if !disableWebhookServer {
		initWebhookValidators(mgr)
	}

	approvedIntentsReconciler := controllers.NewApprovedIntentsReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		kafkaServersStore,
		watchedNamespaces,
		enforcementConfig,
		podName,
		podNamespace,
		additionalIntentsReconcilers...,
	)

	if err = approvedIntentsReconciler.InitIntentsServerIndices(mgr); err != nil {
		logrus.WithError(err).Panic("unable to init indices")
	}

	if err = approvedIntentsReconciler.InitEndpointsPodNamesIndex(mgr); err != nil {
		logrus.WithError(err).Panic("unable to init indices")
	}

	if err = approvedIntentsReconciler.InitProtectedServiceIndexField(mgr); err != nil {
		logrus.WithError(err).Panic("unable to init protected service index")
	}

	if err = approvedIntentsReconciler.SetupWithManager(mgr); err != nil {
		logrus.WithError(err).Panic("unable to create controller", "controller", "Approved Intents")
	}

	intentsReconciler := controllers.NewIntentsReconciler(signalHandlerCtx, mgr.GetClient(), otterizeCloudClient)
	if err = intentsReconciler.SetupWithManager(mgr); err != nil {
		logrus.WithError(err).Panic("unable to create controller", "controller", "Client Intents")
	}

	if err = intentsReconciler.InitReviewStatusIndex(mgr); err != nil {
		logrus.WithError(err).Panic("unable to create controller", "controller", "Client Intents")
	}

	if telemetriesconfig.IsUsageTelemetryEnabled() {
		telemetryReconciler := intents_reconcilers.NewTelemetryReconciler(mgr.GetClient(), mgr.GetScheme())
		if err = telemetryReconciler.SetupWithManager(mgr); err != nil {
			logrus.WithError(err).Panic("unable to create controller", "controller", "Telemetry")
		}
	}

	if otterizeCloudClient != nil {
		otterizeCloudReconciler := intents_reconcilers.NewOtterizeCloudReconciler(mgr.GetClient(), mgr.GetScheme(), otterizeCloudClient)
		if err = otterizeCloudReconciler.SetupWithManager(mgr); err != nil {
			logrus.WithError(err).Panic("unable to create controller", "controller", "OtterizeCloud")
		}
	}

	if err = endpointReconciler.SetupWithManager(mgr); err != nil {
		logrus.WithError(err).Panic("unable to create controller", "controller", "Endpoints")
	}

	if err = externalPolicySvcReconciler.SetupWithManager(mgr); err != nil {
		logrus.WithError(err).Panic("unable to create controller", "controller", "Endpoints")
	}

	if err = ingressReconciler.SetupWithManager(mgr); err != nil {
		logrus.WithError(err).Panic("unable to create controller", "controller", "Ingress")
	}

	if err = netpolReconciler.SetupWithManager(mgr); err != nil {
		logrus.WithError(err).Panic("unable to create controller", "controller", "NetworkPolicy")
	}

	kafkaServerConfigReconciler := controllers.NewKafkaServerConfigReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		kafkaServersStore,
		podName,
		podNamespace,
		otterizeCloudClient,
		serviceidresolver.NewResolver(mgr.GetClient()),
	)

	if err = kafkaServerConfigReconciler.SetupWithManager(mgr); err != nil {
		logrus.WithError(err).Panic("unable to create controller", "controller", "KafkaServerConfig")
	}

	if err = kafkaServerConfigReconciler.InitKafkaServerConfigIndices(mgr); err != nil {
		logrus.WithError(err).Panic("unable to init indices for KafkaServerConfig")
	}

	protectedServicesReconciler := controllers.NewProtectedServiceReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		otterizeCloudClient,
		enforcementConfig.EnforcementDefaultState,
		enforcementConfig.EnableNetworkPolicy,
		epGroupReconciler,
	)

	err = protectedServicesReconciler.SetupWithManager(mgr)
	if err != nil {
		logrus.WithError(err).Panic("unable to create controller", "controller", "ProtectedServices")
	}

	podWatcher := pod_reconcilers.NewPodWatcher(mgr.GetClient(), mgr.GetEventRecorderFor("intents-operator"), watchedNamespaces, enforcementConfig.EnforcementDefaultState, enforcementConfig.EnableIstioPolicy, enforcementConfig.EnforcedNamespaces, approvedIntentsReconciler, epGroupReconciler)
	nsWatcher := pod_reconcilers.NewNamespaceWatcher(mgr.GetClient())
	svcWatcher := port_network_policy.NewServiceWatcher(mgr.GetClient(), mgr.GetEventRecorderFor("intents-operator"), epGroupReconciler, enforcementConfig.EnableNetworkPolicy)

	err = svcWatcher.SetupWithManager(mgr)
	if err != nil {
		logrus.WithError(err).Panic()
	}

	err = podWatcher.InitIntentsClientIndices(mgr)
	if err != nil {
		logrus.WithError(err).Panic()
	}

	err = podWatcher.Register(mgr)
	if err != nil {
		logrus.WithError(err).Panic()
	}

	err = nsWatcher.Register(mgr)
	if err != nil {
		logrus.WithError(err).Panic()
	}

	healthChecker := healthz.Ping
	readyChecker := healthz.Ping

	if !disableWebhookServer {
		healthChecker = mgr.GetWebhookServer().StartedChecker()
		readyChecker = mgr.GetWebhookServer().StartedChecker()
	}

	cacheHealthChecker := func(_ *http.Request) error {
		timeoutCtx, cancel := context.WithTimeout(signalHandlerCtx, 1*time.Second)
		defer cancel()
		ok := mgr.GetCache().WaitForCacheSync(timeoutCtx)
		if !ok {
			return errors.New("Failed waiting for caches to sync")
		}
		return nil
	}

	//+kubebuilder:scaffold:builder
	if err := mgr.AddHealthzCheck("healthz", healthChecker); err != nil {
		logrus.WithError(err).Panic("unable to set up health check")
	}
	if err := mgr.AddReadyzCheck("readyz", readyChecker); err != nil {
		logrus.WithError(err).Panic("unable to set up ready check")
	}
	if err := mgr.AddHealthzCheck("cache", cacheHealthChecker); err != nil {
		logrus.WithError(err).Panic("unable to set up cache health check")
	}

	if err := mgr.AddHealthzCheck("intentsReconcile", health.Checker); err != nil {
		logrus.WithError(err).Panic("unable to set up health check")
	}

	logrus.Info("starting manager")
	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeStarted, 0)
	telemetrysender.IntentsOperatorRunActiveReporter(signalHandlerCtx)

	if err := mgr.Start(signalHandlerCtx); err != nil {
		logrus.WithError(err).Panic("problem running manager")
	}
}

func initWebhookValidators(mgr manager.Manager) {
	intentsValidator := webhooks.NewIntentsValidatorV1alpha2(mgr.GetClient())
	if err := (&otterizev1alpha2.ClientIntents{}).SetupWebhookWithManager(mgr, intentsValidator); err != nil {
		logrus.WithError(err).Panic(err, "unable to create webhook for v1alpha2", "webhook", "ClientIntents")
	}
	intentsValidatorV1alpha3 := webhooks.NewIntentsValidatorV1alpha3(mgr.GetClient())
	if err := (&otterizev1alpha3.ClientIntents{}).SetupWebhookWithManager(mgr, intentsValidatorV1alpha3); err != nil {
		logrus.WithError(err).Panic(err, "unable to create webhook v1alpha3", "webhook", "ClientIntents")
	}

	intentsValidatorV1 := webhooks.NewIntentsValidatorV1(mgr.GetClient())
	if err := (&otterizev1beta1.ClientIntents{}).SetupWebhookWithManager(mgr, intentsValidatorV1); err != nil {
		logrus.WithError(err).Panic(err, "unable to create webhook v1", "webhook", "ClientIntents")
	}

	intentsValidatorV2alpha1 := webhooks.NewIntentsValidatorV2alpha1(mgr.GetClient())
	if err := (&otterizev2alpha1.ClientIntents{}).SetupWebhookWithManager(mgr, intentsValidatorV2alpha1); err != nil {
		logrus.WithError(err).Panic(err, "unable to create webhook v2alpha1", "webhook", "ClientIntents")
	}

	intentsValidatorV2beta1 := webhooks.NewIntentsValidatorV2beta1(mgr.GetClient())
	if err := (&otterizev2beta1.ClientIntents{}).SetupWebhookWithManager(mgr, intentsValidatorV2beta1); err != nil {
		logrus.WithError(err).Panic(err, "unable to create webhook v2beta1", "webhook", "ClientIntents")
	}

	protectedServiceValidator := webhooks.NewProtectedServiceValidatorV1alpha2(mgr.GetClient())
	if err := (&otterizev1alpha2.ProtectedService{}).SetupWebhookWithManager(mgr, protectedServiceValidator); err != nil {
		logrus.WithError(err).Panic("unable to create webhook v1alpha2", "webhook", "ProtectedService")
	}

	protectedServiceValidatorV1alpha3 := webhooks.NewProtectedServiceValidatorV1alpha3(mgr.GetClient())
	if err := (&otterizev1alpha3.ProtectedService{}).SetupWebhookWithManager(mgr, protectedServiceValidatorV1alpha3); err != nil {
		logrus.WithError(err).Panic("unable to create webhook v1alpha3", "webhook", "ProtectedService")
	}

	protectedServiceValidatorV1 := webhooks.NewProtectedServiceValidatorV1(mgr.GetClient())
	if err := (&otterizev1beta1.ProtectedService{}).SetupWebhookWithManager(mgr, protectedServiceValidatorV1); err != nil {
		logrus.WithError(err).Panic("unable to create webhook v1", "webhook", "ProtectedService")
	}

	protectedServiceValidatorV2alpha1 := webhooks.NewProtectedServiceValidatorV2alpha1(mgr.GetClient())
	if err := (&otterizev2alpha1.ProtectedService{}).SetupWebhookWithManager(mgr, protectedServiceValidatorV2alpha1); err != nil {
		logrus.WithError(err).Panic("unable to create webhook v2alpha1", "webhook", "ProtectedService")
	}

	protectedServiceValidatorV2beta1 := webhooks.NewProtectedServiceValidatorV2beta1(mgr.GetClient())
	if err := (&otterizev2beta1.ProtectedService{}).SetupWebhookWithManager(mgr, protectedServiceValidatorV2beta1); err != nil {
		logrus.WithError(err).Panic("unable to create webhook v2beta1", "webhook", "ProtectedService")
	}

	if err := (&otterizev1alpha2.KafkaServerConfig{}).SetupWebhookWithManager(mgr); err != nil {
		logrus.WithError(err).Panic("unable to create webhook v1alpha2", "webhook", "KafkaServerConfig")
	}

	if err := (&otterizev1alpha3.KafkaServerConfig{}).SetupWebhookWithManager(mgr); err != nil {
		logrus.WithError(err).Panic("unable to create webhook v1alpha3", "webhook", "KafkaServerConfig")
	}

	if err := (&otterizev1beta1.KafkaServerConfig{}).SetupWebhookWithManager(mgr); err != nil {
		logrus.WithError(err).Panic("unable to create webhook v1", "webhook", "KafkaServerConfig")
	}

	if err := (&otterizev2alpha1.KafkaServerConfig{}).SetupWebhookWithManager(mgr); err != nil {
		logrus.WithError(err).Panic("unable to create webhook v2alpha1", "webhook", "KafkaServerConfig")
	}

	if err := (&otterizev2beta1.KafkaServerConfig{}).SetupWebhookWithManager(mgr); err != nil {
		logrus.WithError(err).Panic("unable to create webhook v2beta1", "webhook", "KafkaServerConfig")
	}

	pgServerConfValidator := webhooks.NewPostgresConfValidator(mgr.GetClient())
	if err := (&otterizev1alpha3.PostgreSQLServerConfig{}).SetupWebhookWithManager(mgr, pgServerConfValidator); err != nil {
		logrus.WithError(err).Panic("unable to create webhook v1alpha3", "webhook", "PostgreSQLServerConfig")
	}

	pgServerConfValidatorV1 := webhooks.NewPostgresConfValidatorV1(mgr.GetClient())
	if err := (&otterizev1beta1.PostgreSQLServerConfig{}).SetupWebhookWithManager(mgr, pgServerConfValidatorV1); err != nil {
		logrus.WithError(err).Panic("unable to create webhook v1", "webhook", "PostgreSQLServerConfig")
	}

	pgServerConfValidatorV2alpha1 := webhooks.NewPostgresConfValidatorV2alpha1(mgr.GetClient())
	if err := (&otterizev2alpha1.PostgreSQLServerConfig{}).SetupWebhookWithManager(mgr, pgServerConfValidatorV2alpha1); err != nil {
		logrus.WithError(err).Panic("unable to create webhook v2alpha1", "webhook", "PostgreSQLServerConfig")
	}

	pgServerConfValidatorV2beta1 := webhooks.NewPostgresConfValidatorV2beta1(mgr.GetClient())
	if err := (&otterizev2beta1.PostgreSQLServerConfig{}).SetupWebhookWithManager(mgr, pgServerConfValidatorV2beta1); err != nil {
		logrus.WithError(err).Panic("unable to create webhook v2beta1", "webhook", "PostgreSQLServerConfig")
	}

	mysqlServerConfValidator := webhooks.NewMySQLConfValidator(mgr.GetClient())
	if err := (&otterizev1alpha3.MySQLServerConfig{}).SetupWebhookWithManager(mgr, mysqlServerConfValidator); err != nil {
		logrus.WithError(err).Panic("unable to create webhook v1alpha3", "webhook", "MySQLServerConfig")
	}

	mysqlServerConfValidatorV1 := webhooks.NewMySQLConfValidatorV1(mgr.GetClient())
	if err := (&otterizev1beta1.MySQLServerConfig{}).SetupWebhookWithManager(mgr, mysqlServerConfValidatorV1); err != nil {
		logrus.WithError(err).Panic("unable to create webhook v1", "webhook", "MySQLServerConfig")
	}

	mysqlServerConfValidatorV2alpha1 := webhooks.NewMySQLConfValidatorV2alpha1(mgr.GetClient())
	if err := (&otterizev2alpha1.MySQLServerConfig{}).SetupWebhookWithManager(mgr, mysqlServerConfValidatorV2alpha1); err != nil {
		logrus.WithError(err).Panic("unable to create webhook v2alpha1", "webhook", "MySQLServerConfig")
	}

	mysqlServerConfValidatorV2beta1 := webhooks.NewMySQLConfValidatorV2beta1(mgr.GetClient())
	if err := (&otterizev2beta1.MySQLServerConfig{}).SetupWebhookWithManager(mgr, mysqlServerConfValidatorV2beta1); err != nil {
		logrus.WithError(err).Panic("unable to create webhook v2beta1", "webhook", "MySQLServerConfig")
	}
}
