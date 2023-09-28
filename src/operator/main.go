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
	"fmt"
	"github.com/bombsimon/logrusr/v3"
	certmanager "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/otterize/credentials-operator/src/controllers"
	"github.com/otterize/credentials-operator/src/controllers/certificates/otterizecertgen"
	"github.com/otterize/credentials-operator/src/controllers/certificates/spirecertgen"
	"github.com/otterize/credentials-operator/src/controllers/certmanageradapter"
	"github.com/otterize/credentials-operator/src/controllers/otterizeclient"
	"github.com/otterize/credentials-operator/src/controllers/secrets"
	"github.com/otterize/credentials-operator/src/controllers/serviceaccount"
	"github.com/otterize/credentials-operator/src/controllers/spireclient"
	"github.com/otterize/credentials-operator/src/controllers/spireclient/bundles"
	"github.com/otterize/credentials-operator/src/controllers/spireclient/entries"
	"github.com/otterize/credentials-operator/src/controllers/spireclient/svids"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"golang.org/x/exp/slices"
	"os"
	"strings"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	// +kubebuilder:scaffold:imports
)

var (
	scheme = runtime.NewScheme()
)

const (
	socketPath = "unix:////run/spire/sockets/agent.sock"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(certmanager.AddToScheme(scheme))

	// +kubebuilder:scaffold:scheme
}

func initSpireClient(ctx context.Context, spireServerAddr string) (spireclient.ServerClient, error) {
	// fetch Certificate & bundle through spire-agent API
	source, err := workloadapi.NewX509Source(ctx, workloadapi.WithClientOptions(workloadapi.WithAddr(socketPath), workloadapi.WithLogger(logrus.StandardLogger())))
	if err != nil {
		return nil, err
	}

	serverClient, err := spireclient.NewServerClient(ctx, spireServerAddr, source)
	if err != nil {
		return nil, err
	}
	logrus.WithField("server_address", spireServerAddr).Infof("Successfully connected to SPIRE server")
	return serverClient, nil
}

const (
	ProviderSpire       = "spire"
	ProviderCloud       = "otterize-cloud"
	ProviderCertManager = "cert-manager"
)

type CertProvider struct {
	Provider string
}

func (cpf *CertProvider) getOptionalValues() []string {
	return []string{ProviderSpire, ProviderCloud, ProviderCertManager}
}

func (cpf *CertProvider) GetPrintableOptionalValues() string {
	return strings.Join(cpf.getOptionalValues(), ", ")
}

func (cpf *CertProvider) Set(v string) error {
	if v == "" {
		v = ProviderSpire
	}
	if slices.Contains(cpf.getOptionalValues(), v) {
		cpf.Provider = v
		return nil
	}
	return fmt.Errorf("certificate-provider should be one of: %s", cpf.GetPrintableOptionalValues())
}

func (cpf *CertProvider) String() string {
	return cpf.Provider
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var spireServerAddr string
	var certProvider CertProvider
	var certManagerIssuer string
	var certManagerUseClusterIssuer bool
	var certManagerApprover bool
	var secretsManager controllers.SecretsManager
	var workloadRegistry controllers.WorkloadRegistry
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":7071", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":7072", "The address the probe endpoint binds to.")
	flag.StringVar(&spireServerAddr, "spire-server-address", "spire-server.spire:8081", "SPIRE server API address.")
	flag.Var(&certProvider, "certificate-provider", fmt.Sprintf("Certificate generation provider (%s)", certProvider.GetPrintableOptionalValues()))
	flag.StringVar(&certManagerIssuer, "cert-manager-issuer", "ca-issuer", "Name of the Issuer to be used by cert-manager to sign certificates")
	flag.BoolVar(&certManagerUseClusterIssuer, "cert-manager-use-cluster-issuer", false, "Use ClusterIssuer instead of a (namespace bound) Issuer")
	flag.BoolVar(&certManagerApprover, "cert-manager-approve-requests", false, "Make credentials-operator approve its own CertificateRequests")

	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()
	provider := certProvider.String()

	ctrl.SetLogger(logrusr.New(logrus.StandardLogger()))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "credentials-operator.otterize.com",
	})
	if err != nil {
		logrus.WithError(err).Error("unable to start manager")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()

	serviceIdResolver := serviceidresolver.NewResolver(mgr.GetClient())
	eventRecorder := mgr.GetEventRecorderFor("credentials-operator")

	if provider == ProviderCloud {
		otterizeCloudClient, err := otterizeclient.NewCloudClient(ctx)
		if err != nil {
			logrus.WithError(err).Error("failed to connect to otterize cloud")
			os.Exit(1)
		}
		workloadRegistry = otterizeCloudClient
		otterizeclient.PeriodicallyReportConnectionToCloud(otterizeCloudClient)
		otterizeCertManager := otterizecertgen.NewOtterizeCertificateGenerator(otterizeCloudClient)
		secretsManager = secrets.NewDirectSecretsManager(mgr.GetClient(), serviceIdResolver, eventRecorder, otterizeCertManager)
	} else if provider == ProviderSpire {
		spireClient, err := initSpireClient(ctx, spireServerAddr)
		if err != nil {
			logrus.WithError(err).Error("failed to connect to spire server")
			os.Exit(1)
		}
		defer spireClient.Close()

		bundlesStore := bundles.NewBundlesStore(spireClient)
		svidsStore := svids.NewSVIDsStore(spireClient)
		certGenerator := spirecertgen.NewSpireCertificateDataGenerator(bundlesStore, svidsStore)
		secretsManager = secrets.NewDirectSecretsManager(mgr.GetClient(), serviceIdResolver, eventRecorder, certGenerator)
		workloadRegistry = entries.NewSpireRegistry(spireClient)
	} else { // ProviderCertManager
		secretsManager, workloadRegistry = certmanageradapter.NewCertManagerSecretsManager(mgr.GetClient(), serviceIdResolver,
			eventRecorder, certManagerIssuer, certManagerUseClusterIssuer)
	}

	client := mgr.GetClient()
	podReconciler := controllers.NewPodReconciler(client, mgr.GetScheme(), workloadRegistry, secretsManager,
		serviceIdResolver, eventRecorder, serviceaccount.NewServiceAccountEnsurer(client, eventRecorder), provider == ProviderCloud)

	if err = podReconciler.SetupWithManager(mgr); err != nil {
		logrus.WithField("controller", "Pod").WithError(err).Error("unable to create controller")
		os.Exit(1)
	}

	if provider == ProviderCertManager && certManagerApprover {
		if err = secretsManager.(*certmanageradapter.CertManagerSecretsManager).RegisterCertificateApprover(ctx, mgr); err != nil {
			logrus.WithField("controller", "CertificateRequest").WithError(err).Error("unable to create controller")
			os.Exit(1)
		}
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
		logrus.WithError(err).Error("unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("check", healthz.Ping); err != nil {
		logrus.WithError(err).Error("unable to set up ready check")
		os.Exit(1)
	}

	logrus.Info("starting manager")

	go podReconciler.MaintenanceLoop(ctx)
	if err := mgr.Start(ctx); err != nil {
		logrus.WithError(err).Error("problem running manager")
		os.Exit(1)
	}
}
