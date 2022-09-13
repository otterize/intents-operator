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
	"flag"
	"github.com/bombsimon/logrusr/v3"
	"github.com/otterize/intents-operator/src/operator/controllers"
	"github.com/otterize/intents-operator/src/operator/controllers/external_traffic"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	"github.com/otterize/intents-operator/src/operator/webhooks"
	"github.com/sirupsen/logrus"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/cache"

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
	var configFile string
	var selfSignedCert bool
	var autoCreateNetworkPoliciesForExternalTraffic bool

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&configFile, "config", "",
		"The controller will load its initial configuration from this file. "+
			"Omit this flag to use the default configuration values. "+
			"Command-line flags override configuration from this file.")
	flag.BoolVar(&selfSignedCert, "self-signed-cert", true,
		"Whether to generate and use a self signed cert as the CA for webhooks")
	flag.BoolVar(&autoCreateNetworkPoliciesForExternalTraffic, "auto-create-network-policies-for-external-traffic", true,
		"Whether to automatically create network policies for external traffic")

	flag.Parse()

	podName := MustGetEnvVar("POD_NAME")
	podNamespace := MustGetEnvVar("POD_NAMESPACE")

	ctrl.SetLogger(logrusr.New(logrus.StandardLogger()))

	var err error
	var certBundle webhooks.CertificateBundle
	ctrlConfig := otterizev1alpha1.ProjectConfig{}

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
	if configFile != "" {
		options, err = options.AndFrom(ctrl.ConfigFile().AtPath(configFile).OfKind(&ctrlConfig))
		if err != nil {
			logrus.WithError(err).Fatal("unable to load the config file")
		}

		if len(ctrlConfig.WatchNamespaces) != 0 {
			options.NewCache = cache.MultiNamespacedCacheBuilder(ctrlConfig.WatchNamespaces)
			logrus.Infof("Will only watch the following namespaces: %v", ctrlConfig.WatchNamespaces)
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		logrus.WithError(err).Fatal(err, "unable to start manager")
	}

	kafkaServersStore := kafkaacls.NewServersStore()

	endpointReconciler := external_traffic.NewEndpointReconciler(mgr.GetClient(), mgr.GetScheme(), autoCreateNetworkPoliciesForExternalTraffic)

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

	intentsReconciler := controllers.NewIntentsReconciler(mgr.GetClient(), mgr.GetScheme(), kafkaServersStore, endpointReconciler, ctrlConfig.WatchNamespaces)

	if err = intentsReconciler.InitIntentsServerIndices(mgr); err != nil {
		logrus.WithError(err).Fatal("unable to init indices")
	}

	if err = intentsReconciler.InitEndpointsPodNamesIndex(mgr); err != nil {
		logrus.WithError(err).Fatal("unable to init indices")
	}

	if err = intentsReconciler.SetupWithManager(mgr); err != nil {
		logrus.WithError(err).Fatal("unable to create controller", "controller", "Intents")

	}

	if selfSignedCert == true {
		logrus.Infoln("Creating self signing certs")
		certBundle, err =
			webhooks.GenerateSelfSignedCertificate("intents-operator-webhook-service", "intents-operator-system")
		if err != nil {
			logrus.WithError(err).Fatal("unable to create self signed certs for webhook")
		}
		err = webhooks.WriteCertToFiles(certBundle)
		if err != nil {
			logrus.WithError(err).Fatal("failed writing certs to file system")
		}
		err = webhooks.UpdateWebHookCA(context.Background(),
			"intents-operator-validating-webhook-configuration", certBundle.CertPem)
		if err != nil {
			logrus.WithError(err).Fatal("updating webhook certificate failed")
		}
	}

	intentsValidator := webhooks.NewIntentsValidator(mgr.GetClient())

	if err = intentsValidator.SetupWebhookWithManager(mgr); err != nil {
		logrus.WithError(err).Fatal("unable to create webhook", "webhook", "Intents")
	}

	kafkaServerConfigReconciler := controllers.NewKafkaServerConfigReconciler(mgr.GetClient(), mgr.GetScheme(), kafkaServersStore, podName, podNamespace)

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

	logrus.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logrus.WithError(err).Fatal("problem running manager")
	}
}
