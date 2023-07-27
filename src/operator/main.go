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
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/metadata"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/operator/controllers"
	"github.com/otterize/intents-operator/src/operator/controllers/external_traffic"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/otterizecloud"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	"github.com/otterize/intents-operator/src/operator/webhooks"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetrysender"
	"github.com/otterize/intents-operator/src/shared/version"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	istiosecurityscheme "istio.io/client-go/pkg/apis/security/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
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

	metricsAddr := viper.GetString(operatorconfig.MetricsAddrKey)
	probeAddr := viper.GetString(operatorconfig.ProbeAddrKey)
	enableLeaderElection := viper.GetBool(operatorconfig.EnableLeaderElectionKey)
	selfSignedCert := viper.GetBool(operatorconfig.SelfSignedCertKey)
	autoCreateNetworkPoliciesForExternalTraffic := viper.GetBool(operatorconfig.AutoCreateNetworkPoliciesForExternalTrafficKey)
	autoCreateNetworkPoliciesForExternalTrafficDisableIntentsRequirement := viper.GetBool(operatorconfig.AutoCreateNetworkPoliciesForExternalTrafficNoIntentsRequiredKey)
	watchedNamespaces := viper.GetStringSlice(operatorconfig.WatchedNamespacesKey)
	enforcementConfig := controllers.EnforcementConfig{
		EnforcementEnabledGlobally: viper.GetBool(operatorconfig.EnforcementEnabledGloballyKey),
		EnableNetworkPolicy:        viper.GetBool(operatorconfig.EnableNetworkPolicyKey),
		EnableKafkaACL:             viper.GetBool(operatorconfig.EnableKafkaACLKey),
		EnableIstioPolicy:          viper.GetBool(operatorconfig.EnableIstioPolicyKey),
		EnableProtectedServices:    viper.GetBool(operatorconfig.EnableProtectedServicesKey),
	}
	disableWebhookServer := viper.GetBool(operatorconfig.DisableWebhookServerKey)
	tlsSource := otterizev1alpha2.TLSSource{
		CertFile:   viper.GetString(operatorconfig.KafkaServerTLSCertKey),
		KeyFile:    viper.GetString(operatorconfig.KafkaServerTLSKeyKey),
		RootCAFile: viper.GetString(operatorconfig.KafkaServerTLSCAKey),
	}

	podName := MustGetEnvVar(operatorconfig.IntentsOperatorPodNameKey)
	podNamespace := MustGetEnvVar(operatorconfig.IntentsOperatorPodNamespaceKey)

	ctrl.SetLogger(logrusr.New(logrus.StandardLogger()))

	var err error

	options := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
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
		options.NewCache = cache.MultiNamespacedCacheBuilder(watchedNamespaces)
		logrus.Infof("Will only watch the following namespaces: %v", watchedNamespaces)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		logrus.WithError(err).Fatal(err, "unable to start manager")
	}

	metadataClient, err := metadata.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		logrus.WithError(err).Fatal("unable to create metadata client")
	}
	mapping, err := mgr.GetRESTMapper().RESTMapping(schema.GroupKind{Group: "", Kind: "Namespace"}, "v1")
	if err != nil {
		logrus.WithError(err).Fatal("unable to create Kubernetes API REST mapping")
	}
	kubeSystemUID := ""
	kubeSystemNs, err := metadataClient.Resource(mapping.Resource).Get(context.Background(), "kube-system", metav1.GetOptions{})
	if err != nil || kubeSystemNs == nil {
		logrus.Warningf("failed getting kubesystem UID: %s", err)
		kubeSystemUID = fmt.Sprintf("rand-%s", uuid.New().String())
	} else {
		kubeSystemUID = string(kubeSystemNs.UID)
	}
	telemetrysender.SetGlobalContextId(telemetrysender.Anonymize(kubeSystemUID))
	telemetrysender.SetGlobalVersion(version.Version())

	kafkaServersStore := kafkaacls.NewServersStore(tlsSource, enforcementConfig.EnableKafkaACL, kafkaacls.NewKafkaIntentsAdmin, enforcementConfig.EnforcementEnabledGlobally)

	endpointReconciler := external_traffic.NewEndpointsReconciler(mgr.GetClient(), mgr.GetScheme(), autoCreateNetworkPoliciesForExternalTraffic, autoCreateNetworkPoliciesForExternalTrafficDisableIntentsRequirement, enforcementConfig.EnforcementEnabledGlobally)

	if err = endpointReconciler.InitIngressReferencedServicesIndex(mgr); err != nil {
		logrus.WithError(err).Fatal("unable to init index for ingress")
	}

	if err = endpointReconciler.SetupWithManager(mgr); err != nil {
		logrus.WithError(err).Fatal("unable to create controller", "controller", "Endpoints")
	}

	ingressReconciler := external_traffic.NewIngressReconciler(mgr.GetClient(), mgr.GetScheme(), endpointReconciler)
	if err = ingressReconciler.SetupWithManager(mgr); err != nil {
		logrus.WithError(err).Fatal("unable to create controller", "controller", "Ingress")
	}

	if err = ingressReconciler.InitNetworkPoliciesByIngressNameIndex(mgr); err != nil {
		logrus.WithError(err).Fatal("unable to init index for ingress")
	}

	signalHandlerCtx := ctrl.SetupSignalHandler()
	otterizeCloudClient, connectedToCloud, err := otterizecloud.NewClient(signalHandlerCtx)
	if err != nil {
		logrus.WithError(err).Error("Failed to initialize Otterize Cloud client")
	}
	if connectedToCloud {
		uploadConfiguration(signalHandlerCtx, otterizeCloudClient, enforcementConfig)
		otterizecloud.StartPeriodicallyReportConnectionToCloud(otterizeCloudClient, signalHandlerCtx)
	} else {
		logrus.Info("Not configured for cloud integration")
	}

	if !enforcementConfig.EnforcementEnabledGlobally {
		logrus.Infof("Running with enforcement disabled globally, won't perform any enforcement")
	}

	netpolReconciler := external_traffic.NewNetworkPolicyReconciler(mgr.GetClient(), mgr.GetScheme(), otterizeCloudClient)
	if err = netpolReconciler.SetupWithManager(mgr); err != nil {
		logrus.WithError(err).Fatal("unable to initialize NetworkPolicy reconciler")
	}

	intentsReconciler := controllers.NewIntentsReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		kafkaServersStore,
		endpointReconciler,
		watchedNamespaces,
		enforcementConfig,
		autoCreateNetworkPoliciesForExternalTrafficDisableIntentsRequirement,
		otterizeCloudClient,
		podName,
		podNamespace,
	)

	if err = intentsReconciler.InitIntentsServerIndices(mgr); err != nil {
		logrus.WithError(err).Fatal("unable to init indices")
	}

	if err = intentsReconciler.InitEndpointsPodNamesIndex(mgr); err != nil {
		logrus.WithError(err).Fatal("unable to init indices")
	}

	if err = intentsReconciler.SetupWithManager(mgr); err != nil {
		logrus.WithError(err).Fatal("unable to create controller", "controller", "Intents")
	}

	if selfSignedCert {
		logrus.Infoln("Creating self signing certs")
		certBundle, err :=
			webhooks.GenerateSelfSignedCertificate("intents-operator-webhook-service", podNamespace)
		if err != nil {
			logrus.WithError(err).Fatal("unable to create self signed certs for webhook")
		}
		err = webhooks.WriteCertToFiles(certBundle)
		if err != nil {
			logrus.WithError(err).Fatal("failed writing certs to file system")
		}
		err = webhooks.UpdateWebHookCA(context.Background(),
			"validating-webhook-configuration", certBundle.CertPem)
		if err != nil {
			logrus.WithError(err).Fatal("updating webhook certificate failed")
		}
	}

	if !disableWebhookServer {
		intentsValidator := webhooks.NewIntentsValidator(mgr.GetClient())
		if err = intentsValidator.SetupWebhookWithManager(mgr); err != nil {
			logrus.WithError(err).Fatal("unable to create webhook", "webhook", "Intents")
		}

		protectedServices := webhooks.NewProtectedServicesValidator(mgr.GetClient())
		if err = protectedServices.SetupWebhookWithManager(mgr); err != nil {
			logrus.WithError(err).Fatal("unable to create webhook", "webhook", "ProtectedServices")
		}
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
		logrus.WithError(err).Fatal("unable to create controller", "controller", "KafkaServerConfig")
	}

	if err = (&controllers.ProtectedServicesReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		logrus.WithError(err).Fatal("unable to create controller", "controller", "ProtectedServices")
	}

	//+kubebuilder:scaffold:builder
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		logrus.WithError(err).Fatal("unable to set up health check")
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		logrus.WithError(err).Fatal("unable to set up ready check")
	}

	logrus.Info("starting manager")
	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeStarted, 0)
	if err := mgr.Start(signalHandlerCtx); err != nil {
		logrus.WithError(err).Fatal("problem running manager")
	}
}

func uploadConfiguration(ctx context.Context, otterizeCloudClient otterizecloud.CloudClient, config controllers.EnforcementConfig) {
	timeoutCtx, cancel := context.WithTimeout(ctx, viper.GetDuration(otterizecloudclient.CloudClientTimeoutKey))
	defer cancel()

	err := otterizeCloudClient.ReportIntentsOperatorConfiguration(timeoutCtx, graphqlclient.IntentsOperatorConfigurationInput{
		GlobalEnforcementEnabled:        config.EnforcementEnabledGlobally,
		NetworkPolicyEnforcementEnabled: config.EnforcementEnabledGlobally && config.EnableNetworkPolicy,
		KafkaACLEnforcementEnabled:      config.EnforcementEnabledGlobally && config.EnableKafkaACL,
		IstioPolicyEnforcementEnabled:   config.EnforcementEnabledGlobally && config.EnableIstioPolicy,
		ProtectedServicesEnabled:        config.EnforcementEnabledGlobally && config.EnableProtectedServices,
	})
	if err != nil {
		logrus.WithError(err).Error("Failed to report configuration to the cloud")
	}
}
