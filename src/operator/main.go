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
	"context"
	"fmt"
	"github.com/bombsimon/logrusr/v3"
	"github.com/google/uuid"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/operator/controllers"
	"github.com/otterize/intents-operator/src/operator/controllers/external_traffic"
	"github.com/otterize/intents-operator/src/operator/controllers/iam_pod_reconciler"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/networkpolicy"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/networkpolicy/builders"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/port_network_policy"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	"github.com/otterize/intents-operator/src/operator/controllers/pod_reconcilers"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/operator/otterizecrds"
	"github.com/otterize/intents-operator/src/operator/webhooks"
	"github.com/otterize/intents-operator/src/shared/awsagent"
	"github.com/otterize/intents-operator/src/shared/azureagent"
	"github.com/otterize/intents-operator/src/shared/filters"
	"github.com/otterize/intents-operator/src/shared/gcpagent"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/allowexternaltraffic"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/otterize/intents-operator/src/shared/reconcilergroup"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/intents-operator/src/shared/telemetries/componentinfo"
	"github.com/otterize/intents-operator/src/shared/telemetries/errorreporter"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetrysender"
	"github.com/otterize/intents-operator/src/shared/version"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/metadata"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"time"

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
	errorreporter.Init("intents-operator", version.Version(), viper.GetString(operatorconfig.TelemetryErrorsAPIKeyKey))
	defer errorreporter.AutoNotify()

	metricsAddr := viper.GetString(operatorconfig.MetricsAddrKey)
	probeAddr := viper.GetString(operatorconfig.ProbeAddrKey)
	enableLeaderElection := viper.GetBool(operatorconfig.EnableLeaderElectionKey)
	selfSignedCert := viper.GetBool(operatorconfig.SelfSignedCertKey)
	allowExternalTraffic := allowexternaltraffic.Enum(viper.GetString(operatorconfig.AllowExternalTrafficKey))
	watchedNamespaces := viper.GetStringSlice(operatorconfig.WatchedNamespacesKey)
	enforcementConfig := controllers.EnforcementConfig{
		EnforcementDefaultState:              viper.GetBool(operatorconfig.EnforcementDefaultStateKey),
		EnableNetworkPolicy:                  viper.GetBool(operatorconfig.EnableNetworkPolicyKey),
		EnableKafkaACL:                       viper.GetBool(operatorconfig.EnableKafkaACLKey),
		EnableIstioPolicy:                    viper.GetBool(operatorconfig.EnableIstioPolicyKey),
		EnableDatabasePolicy:                 viper.GetBool(operatorconfig.EnableDatabasePolicy),
		EnableEgressNetworkPolicyReconcilers: viper.GetBool(operatorconfig.EnableEgressNetworkPolicyReconcilersKey),
		EnableAWSPolicy:                      viper.GetBool(operatorconfig.EnableAWSPolicyKey),
		EnableGCPPolicy:                      viper.GetBool(operatorconfig.EnableGCPPolicyKey),
		EnableAzurePolicy:                    viper.GetBool(operatorconfig.EnableAzurePolicyKey),
	}
	disableWebhookServer := viper.GetBool(operatorconfig.DisableWebhookServerKey)
	tlsSource := otterizev1alpha3.TLSSource{
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
	signalHandlerCtx := ctrl.SetupSignalHandler()

	metadataClient, err := metadata.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		logrus.WithError(err).Panic("unable to create metadata client")
	}
	mapping, err := mgr.GetRESTMapper().RESTMapping(schema.GroupKind{Group: "", Kind: "Namespace"}, "v1")
	if err != nil {
		logrus.WithError(err).Panic("unable to create Kubernetes API REST mapping")
	}
	kubeSystemUID := ""
	kubeSystemNs, err := metadataClient.Resource(mapping.Resource).Get(signalHandlerCtx, "kube-system", metav1.GetOptions{})
	if err != nil || kubeSystemNs == nil {
		logrus.Warningf("failed getting kubesystem UID: %s", err)
		kubeSystemUID = fmt.Sprintf("rand-%s", uuid.New().String())
	} else {
		kubeSystemUID = string(kubeSystemNs.UID)
	}
	componentinfo.SetGlobalContextId(telemetrysender.Anonymize(kubeSystemUID))
	componentinfo.SetGlobalVersion(version.Version())

	directClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		logrus.WithError(err).Panic("unable to create kubernetes API client")
	}

	kafkaServersStore := kafkaacls.NewServersStore(tlsSource, enforcementConfig.EnableKafkaACL, kafkaacls.NewKafkaIntentsAdmin, enforcementConfig.EnforcementDefaultState)

	extNetpolHandler := external_traffic.NewNetworkPolicyHandler(mgr.GetClient(), mgr.GetScheme(), allowExternalTraffic)
	endpointReconciler := external_traffic.NewEndpointsReconciler(mgr.GetClient(), extNetpolHandler)
	ingressRulesBuilder := builders.NewIngressNetpolBuilder()

	additionalIntentsReconcilers := make([]reconcilergroup.ReconcilerWithEvents, 0)
	svcNetworkPolicyBuilder := builders.NewPortNetworkPolicyReconciler(mgr.GetClient())
	dnsServerNetpolBuilder := builders.NewIngressDNSServerAutoAllowNetpolBuilder()
	epNetpolReconciler := networkpolicy.NewReconciler(mgr.GetClient(), scheme, extNetpolHandler, watchedNamespaces, enforcementConfig.EnableNetworkPolicy, enforcementConfig.EnforcementDefaultState,
		[]networkpolicy.IngressRuleBuilder{ingressRulesBuilder, svcNetworkPolicyBuilder, dnsServerNetpolBuilder}, make([]networkpolicy.EgressRuleBuilder, 0))
	epGroupReconciler := effectivepolicy.NewGroupReconciler(mgr.GetClient(), scheme, epNetpolReconciler)
	if enforcementConfig.EnableEgressNetworkPolicyReconcilers {
		egressNetworkPolicyHandler := builders.NewEgressNetworkPolicyBuilder()
		epNetpolReconciler.AddEgressRuleBuilder(egressNetworkPolicyHandler)
		internetNetpolReconciler := builders.NewInternetEgressRulesBuilder()
		epNetpolReconciler.AddEgressRuleBuilder(internetNetpolReconciler)
		svcEgressNetworkPolicyHandler := builders.NewPortEgressRulesBuilder(mgr.GetClient())
		epNetpolReconciler.AddEgressRuleBuilder(svcEgressNetworkPolicyHandler)

	}
	epIntentsReconciler := intents_reconcilers.NewServiceEffectiveIntentsReconciler(mgr.GetClient(), scheme, epGroupReconciler)
	additionalIntentsReconcilers = append(additionalIntentsReconcilers, epIntentsReconciler)

	var iamAgents []intents_reconcilers.IAMPolicyAgent

	serviceIdResolver := serviceidresolver.NewResolver(mgr.GetClient())

	if enforcementConfig.EnableAWSPolicy {
		awsOptions := make([]awsagent.Option, 0)
		if viper.GetBool(operatorconfig.EnableAWSRolesAnywhereKey) {
			trustAnchorArn := viper.GetString(operatorconfig.AWSRolesAnywhereTrustAnchorARNKey)
			trustDomain := viper.GetString(operatorconfig.AWSRolesAnywhereSPIFFETrustDomainKey)
			clusterName := viper.GetString(operatorconfig.AWSRolesAnywhereClusterName)

			awsOptions = append(awsOptions, awsagent.WithRolesAnywhere(trustAnchorArn, trustDomain, clusterName))
		}
		awsIntentsAgent, err := awsagent.NewAWSAgent(signalHandlerCtx, awsOptions...)
		if err != nil {
			logrus.WithError(err).Panic("could not initialize AWS agent")
		}
		awsIntentsReconciler := intents_reconcilers.NewAWSIntentsReconciler(mgr.GetClient(), scheme, awsIntentsAgent, serviceIdResolver)
		additionalIntentsReconcilers = append(additionalIntentsReconcilers, awsIntentsReconciler)
		iamPodWatcher := iam_pod_reconciler.NewIAMPodReconciler(mgr.GetClient(), mgr.GetEventRecorderFor("intents-operator"), awsIntentsReconciler)
		err = iamPodWatcher.SetupWithManager(mgr)
		if err != nil {
			logrus.WithError(err).Panic("unable to register pod watcher")
		}
	}

	if enforcementConfig.EnableGCPPolicy {
		gcpIntentsAgent, err := gcpagent.NewGCPAgent(signalHandlerCtx, mgr.GetClient())
		if err != nil {
			logrus.WithError(err).Panic("could not initialize GCP agent")
		}
		iamAgents = append(iamAgents, gcpIntentsAgent)
	}

	if enforcementConfig.EnableAzurePolicy {
		config := azureagent.Config{
			SubscriptionID: MustGetEnvVar(operatorconfig.AzureSubscriptionIDKey),
			ResourceGroup:  MustGetEnvVar(operatorconfig.AzureResourceGroupKey),
			AKSClusterName: MustGetEnvVar(operatorconfig.AzureAKSClusterNameKey),
		}

		azureIntentsAgent, err := azureagent.NewAzureAgent(signalHandlerCtx, config)
		if err != nil {
			logrus.WithError(err).Panic("could not initialize Azure agent")
		}
		iamAgents = append(iamAgents, azureIntentsAgent)
	}

	if len(iamAgents) > 0 {
		iamIntentsReconciler := intents_reconcilers.NewIAMIntentsReconciler(mgr.GetClient(), scheme, serviceIdResolver, iamAgents)
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

	logrus.Panic("testing")

	otterizeCloudClient, connectedToCloud, err := operator_cloud_client.NewClient(signalHandlerCtx)
	if err != nil {
		logrus.WithError(err).Error("Failed to initialize Otterize Cloud client")
	}
	if connectedToCloud {
		uploadConfiguration(signalHandlerCtx, otterizeCloudClient, enforcementConfig)
		operator_cloud_client.StartPeriodicallyReportConnectionToCloud(otterizeCloudClient, signalHandlerCtx)

		netpolUploader := external_traffic.NewNetworkPolicyUploaderReconciler(mgr.GetClient(), mgr.GetScheme(), otterizeCloudClient)
		if err = netpolUploader.SetupWithManager(mgr); err != nil {
			logrus.WithError(err).Panic("unable to initialize NetworkPolicy reconciler")
		}
	} else {
		logrus.Info("Not configured for cloud integration")
	}

	externalPolicySvcReconciler := external_traffic.NewServiceReconciler(mgr.GetClient(), extNetpolHandler, otterizeCloudClient)
	ingressReconciler := external_traffic.NewIngressReconciler(mgr.GetClient(), extNetpolHandler, otterizeCloudClient)

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

		err = otterizecrds.Ensure(signalHandlerCtx, directClient, podNamespace)
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

		customResourceDefinitionsReconciler := controllers.NewCustomResourceDefinitionsReconciler(
			mgr.GetClient(),
			mgr.GetScheme(),
			certBundle.CertPem,
			podNamespace,
		)
		err = customResourceDefinitionsReconciler.SetupWithManager(mgr)
		if err != nil {
			logrus.WithError(err).Panic("unable to create controller", "controller", "CustomResourceDefinition")
		}
	}

	if !disableWebhookServer {
		intentsValidator := webhooks.NewIntentsValidatorV1alpha2(mgr.GetClient())
		if err = (&otterizev1alpha2.ClientIntents{}).SetupWebhookWithManager(mgr, intentsValidator); err != nil {
			logrus.WithError(err).Panic(err, "unable to create webhook for v1alpha2", "webhook", "ClientIntents")
		}
		intentsValidatorV1alpha3 := webhooks.NewIntentsValidatorV1alpha3(mgr.GetClient())
		if err = (&otterizev1alpha3.ClientIntents{}).SetupWebhookWithManager(mgr, intentsValidatorV1alpha3); err != nil {
			logrus.WithError(err).Panic(err, "unable to create webhook v1alpha3", "webhook", "ClientIntents")
		}

		protectedServiceValidator := webhooks.NewProtectedServiceValidatorV1alpha2(mgr.GetClient())
		if err = (&otterizev1alpha2.ProtectedService{}).SetupWebhookWithManager(mgr, protectedServiceValidator); err != nil {
			logrus.WithError(err).Panic("unable to create webhook v1alpha2", "webhook", "ProtectedService")
		}

		protectedServiceValidatorV1alpha3 := webhooks.NewProtectedServiceValidatorV1alpha3(mgr.GetClient())
		if err = (&otterizev1alpha3.ProtectedService{}).SetupWebhookWithManager(mgr, protectedServiceValidatorV1alpha3); err != nil {
			logrus.WithError(err).Panic("unable to create webhook v1alpha3", "webhook", "ProtectedService")
		}

		if err = (&otterizev1alpha2.KafkaServerConfig{}).SetupWebhookWithManager(mgr); err != nil {
			logrus.WithError(err).Panic("unable to create webhook v1alpha2", "webhook", "KafkaServerConfig")
		}

		if err = (&otterizev1alpha3.KafkaServerConfig{}).SetupWebhookWithManager(mgr); err != nil {
			logrus.WithError(err).Panic("unable to create webhook v1alpha3", "webhook", "KafkaServerConfig")
		}

	}

	intentsReconciler := controllers.NewIntentsReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		kafkaServersStore,
		watchedNamespaces,
		enforcementConfig,
		otterizeCloudClient,
		podName,
		podNamespace,
		additionalIntentsReconcilers...,
	)

	if err = intentsReconciler.InitIntentsServerIndices(mgr); err != nil {
		logrus.WithError(err).Panic("unable to init indices")
	}

	if err = intentsReconciler.InitEndpointsPodNamesIndex(mgr); err != nil {
		logrus.WithError(err).Panic("unable to init indices")
	}

	if err = intentsReconciler.InitProtectedServiceIndexField(mgr); err != nil {
		logrus.WithError(err).Panic("unable to init protected service index")
	}

	if err = intentsReconciler.SetupWithManager(mgr); err != nil {
		logrus.WithError(err).Panic("unable to create controller", "controller", "Intents")
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
		extNetpolHandler,
		enforcementConfig.EnforcementDefaultState,
		enforcementConfig.EnableNetworkPolicy,
		epGroupReconciler,
	)

	err = protectedServicesReconciler.SetupWithManager(mgr)
	if err != nil {
		logrus.WithError(err).Panic("unable to create controller", "controller", "ProtectedServices")
	}

	podWatcher := pod_reconcilers.NewPodWatcher(mgr.GetClient(), mgr.GetEventRecorderFor("intents-operator"), watchedNamespaces, enforcementConfig.EnforcementDefaultState, enforcementConfig.EnableIstioPolicy)
	nsWatcher := pod_reconcilers.NewNamespaceWatcher(mgr.GetClient())
	svcWatcher := port_network_policy.NewServiceWatcher(mgr.GetClient(), mgr.GetEventRecorderFor("intents-operator"), epGroupReconciler)

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

	//+kubebuilder:scaffold:builder
	if err := mgr.AddHealthzCheck("healthz", healthChecker); err != nil {
		logrus.WithError(err).Panic("unable to set up health check")
	}
	if err := mgr.AddReadyzCheck("readyz", readyChecker); err != nil {
		logrus.WithError(err).Panic("unable to set up ready check")
	}

	logrus.Info("starting manager")
	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeStarted, 0)
	telemetrysender.IntentsOperatorRunActiveReporter(signalHandlerCtx)

	if err := mgr.Start(signalHandlerCtx); err != nil {
		logrus.WithError(err).Panic("problem running manager")
	}
}

func uploadConfiguration(ctx context.Context, otterizeCloudClient operator_cloud_client.CloudClient, config controllers.EnforcementConfig) {
	timeoutCtx, cancel := context.WithTimeout(ctx, viper.GetDuration(otterizecloudclient.CloudClientTimeoutKey))
	defer cancel()

	err := otterizeCloudClient.ReportIntentsOperatorConfiguration(timeoutCtx, graphqlclient.IntentsOperatorConfigurationInput{
		GlobalEnforcementEnabled:              config.EnforcementDefaultState,
		NetworkPolicyEnforcementEnabled:       config.EnableNetworkPolicy,
		EgressNetworkPolicyEnforcementEnabled: config.EnableEgressNetworkPolicyReconcilers,
		KafkaACLEnforcementEnabled:            config.EnableKafkaACL,
		AwsIAMPolicyEnforcementEnabled:        config.EnableAWSPolicy,
		GcpIAMPolicyEnforcementEnabled:        config.EnableGCPPolicy,
		AzureIAMPolicyEnforcementEnabled:      config.EnableAzurePolicy,
		DatabaseEnforcementEnabled:            config.EnableDatabasePolicy,
		IstioPolicyEnforcementEnabled:         config.EnableIstioPolicy,
		ProtectedServicesEnabled:              config.EnableNetworkPolicy, // in this version, protected services are enabled if network policy creation is enabled, regardless of enforcement default state
	})
	if err != nil {
		logrus.WithError(err).Error("Failed to report configuration to the cloud")
	}
}
