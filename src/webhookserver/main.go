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
	"github.com/otterize/intents-operator/src/operator/otterizecrds"
	"github.com/otterize/intents-operator/src/operator/webhooks"
	"github.com/otterize/intents-operator/src/shared"
	"github.com/otterize/intents-operator/src/shared/clusterutils"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/filters"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/otterize/intents-operator/src/shared/telemetries/componentinfo"
	"github.com/otterize/intents-operator/src/shared/telemetries/errorreporter"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetrysender"
	"github.com/otterize/intents-operator/src/shared/version"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	selfSignedCert := viper.GetBool(operatorconfig.SelfSignedCertKey)
	watchedNamespaces := viper.GetStringSlice(operatorconfig.WatchedNamespacesKey)

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
		LeaderElection:         false,
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

	mgr, err := ctrl.NewManager(k8sconf.KubernetesConfigOrDie(), options)
	if err != nil {
		logrus.WithError(err).Panic(err, "unable to start manager")
	}

	directClient, err := client.New(k8sconf.KubernetesConfigOrDie(), client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		logrus.WithError(err).Panic("unable to create kubernetes API client")
	}

	if selfSignedCert {
		logrus.Infoln("Creating self signing certs")
		certBundle, ok, err := webhooks.ReadCertBundleFromSecret(signalHandlerCtx, directClient, "intents-operator-webhook-cert", podNamespace)
		if err != nil {
			logrus.WithError(err).Warn("unable to read existing certs from secret, generating new ones")
		}

		if !ok {
			logrus.Info("webhook certs uninitialized, generating new certs")
		}

		if !ok || err != nil {
			certBundleNew, err :=
				webhooks.GenerateSelfSignedCertificate("intents-operator-webhook-service", podNamespace)
			if err != nil {
				logrus.WithError(err).Panic("unable to create self signed certs for webhook")
			}

			err = webhooks.PersistCertBundleToSecret(signalHandlerCtx, directClient, "intents-operator-webhook-cert", podNamespace, certBundleNew)
			if err != nil {
				logrus.WithError(err).Panic("unable to persist certs to secret")
			}
			certBundle = certBundleNew
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

	initWebhookValidators(mgr)

	healthChecker := mgr.GetWebhookServer().StartedChecker()
	readyChecker := mgr.GetWebhookServer().StartedChecker()

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
	if err := mgr.AddReadyzCheck("cache", cacheHealthChecker); err != nil {
		logrus.WithError(err).Panic("unable to set up ready check")
	}

	logrus.Info("starting manager")

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
