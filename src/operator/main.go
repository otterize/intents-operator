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
	"github.com/bombsimon/logrusr/v3"
	"github.com/otterize/intents-operator/src/operator/controllers"
	"github.com/otterize/intents-operator/src/operator/controllers/external_traffic"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/otterizecloud"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	"github.com/otterize/intents-operator/src/operator/webhooks"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"time"

	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

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

const enableEnforcementKey = "enable-enforcement"

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(otterizev1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func MustGetEnvVar(name string) string {
	value := os.Getenv(name)
	if value == "" {
		logrus.Fatalf("%s environment variable is required", name)
	}

	return value
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var selfSignedCert bool
	var enforcementEnabledGlobally bool
	var autoCreateNetworkPoliciesForExternalTraffic bool
	var watchedNamespaces []string
	var enableNetworkPolicyCreation bool
	var enableKafkaACLCreation bool
	var disableWebhookServer bool
	var tlsSource otterizev1alpha1.TLSSource
	var otterizeCloudClient otterizecloud.CloudClient

	pflag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	pflag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	pflag.StringVar(&tlsSource.CertFile, "kafka-server-tls-cert", "", "name of tls certificate file")
	pflag.StringVar(&tlsSource.KeyFile, "kafka-server-tls-key", "", "name of tls private key file")
	pflag.StringVar(&tlsSource.RootCAFile, "kafka-server-tls-ca", "", "name of tls ca file")
	pflag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	pflag.BoolVar(&selfSignedCert, "self-signed-cert", true,
		"Whether to generate and use a self signed cert as the CA for webhooks")
	pflag.BoolVar(&disableWebhookServer, "disable-webhook-server", false,
		"Disable webhook validator server")
	pflag.BoolVar(&enforcementEnabledGlobally, enableEnforcementKey, true,
		"If set to false disables the enforcement globally, superior to the other flags")
	pflag.BoolVar(&autoCreateNetworkPoliciesForExternalTraffic, "auto-create-network-policies-for-external-traffic", true,
		"Whether to automatically create network policies for external traffic")
	pflag.StringSliceVar(&watchedNamespaces, "watched-namespaces", nil,
		"Namespaces that will be watched by the operator. Specify multiple values by specifying multiple times or separate with commas.")
	pflag.BoolVar(&enableNetworkPolicyCreation, "enable-network-policy-creation", true,
		"Whether to disable Intents network policy creation")
	pflag.BoolVar(&enableKafkaACLCreation, "enable-kafka-acl-creation", true,
		"Whether to disable Intents Kafka ACL creation")

	pflag.Parse()

	podName := MustGetEnvVar("POD_NAME")
	podNamespace := MustGetEnvVar("POD_NAMESPACE")

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

	kafkaServersStore := kafkaacls.NewServersStore(tlsSource, enableKafkaACLCreation, kafkaacls.NewKafkaIntentsAdmin, enforcementEnabledGlobally)

	endpointReconciler := external_traffic.NewEndpointsReconciler(mgr.GetClient(), mgr.GetScheme(), autoCreateNetworkPoliciesForExternalTraffic, enforcementEnabledGlobally)

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
	otterizeCloudClient, ok, err := otterizecloud.NewClient(context.Background())
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create otterize cloud client")
	}
	if !ok {
		logrus.Info("missing configuration for cloud integration, disabling cloud communication")
	} else {
		otterizecloud.StartPeriodicallyReportConnectionToCloud(otterizeCloudClient, signalHandlerCtx)
	}

	intentsReconciler := controllers.NewIntentsReconciler(
		mgr.GetClient(), mgr.GetScheme(), kafkaServersStore, endpointReconciler,
		watchedNamespaces, enforcementEnabledGlobally, enableNetworkPolicyCreation, enableKafkaACLCreation,
		otterizeCloudClient)

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
	}

	kafkaServerConfigReconciler := controllers.NewKafkaServerConfigReconciler(mgr.GetClient(), mgr.GetScheme(), kafkaServersStore, podName, podNamespace, otterizeCloudClient)

	if err = kafkaServerConfigReconciler.SetupWithManager(mgr); err != nil {
		logrus.WithError(err).Fatal("unable to create controller", "controller", "KafkaServerConfig")
	}

	//+kubebuilder:scaffold:builder
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		logrus.WithError(err).Fatal("unable to set up health check")
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		logrus.WithError(err).Fatal("unable to set up ready check")
	}

	ctx, cancelFunc := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelFunc()
	cloudClient, connectedToCloud, err := otterizecloud.NewClient(ctx)
	if err != nil {
		logrus.WithError(err).Warning("Failed to connect to Otterize cloud")
	}
	if connectedToCloud {
		err := cloudClient.ReportIntentsOperatorConfiguration(ctx, enforcementEnabledGlobally)
		if err != nil {
			logrus.WithError(err).Warning("Failed to report configuration to the cloud")
		}
	}
	if !enforcementEnabledGlobally {
		logrus.Infof("Running with %s=false, won't perform any enforcement", enableEnforcementKey)
	}

	logrus.Info("starting manager")
	if err := mgr.Start(signalHandlerCtx); err != nil {
		logrus.WithError(err).Fatal("problem running manager")
	}
}
