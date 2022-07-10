package main

import (
	"context"
	"flag"
	"github.com/bombsimon/logrusr/v3"
	"github.com/otterize/spifferize/src/operator/controllers"
	spire_client "github.com/otterize/spifferize/src/spire-client"
	"github.com/otterize/spifferize/src/spire-client/bundles"
	"github.com/otterize/spifferize/src/spire-client/entries"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"os"

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

	// +kubebuilder:scaffold:scheme
}

func initSpireClient(ctx context.Context, spireServerAddr string) (spire_client.ServerClient, error) {
	// fetch SVID & bundle through spire-agent API
	source, err := workloadapi.NewX509Source(ctx, workloadapi.WithClientOptions(workloadapi.WithAddr(socketPath)))
	if err != nil {
		return nil, err
	}

	serverClient, err := spire_client.NewServerClient(ctx, spireServerAddr, source)
	if err != nil {
		return nil, err
	}
	logrus.WithField("server_address", spireServerAddr).Infof("Successfully connected to SPIRE server")
	return serverClient, nil
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var spireServerAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&spireServerAddr, "spire-server-address", "spire-server.spire:8081", "SPIRE server API address.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	ctrl.SetLogger(logrusr.New(logrus.StandardLogger()))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "spifferize-operator.otterize.com",
	})
	if err != nil {
		logrus.Error(err, "unable to start manager")
		os.Exit(1)
	}

	spireClient, err := initSpireClient(context.TODO(), spireServerAddr)
	if err != nil {
		logrus.Error(err, "failed to connect to spire server")
		os.Exit(1)
	}
	defer spireClient.Close()

	bundlesManager := bundles.NewBundlesManager(spireClient)
	entriesManager := entries.NewEntriesManager(spireClient)

	podReconciler := &controllers.PodReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		SpireClient:    spireClient,
		BundlesManager: bundlesManager,
		EntriesManager: entriesManager,
	}

	if err = podReconciler.SetupWithManager(mgr); err != nil {
		logrus.WithField("controller", "Pod").Error(err, "unable to create controller")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
		logrus.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("check", healthz.Ping); err != nil {
		logrus.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	logrus.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logrus.Error(err, "problem running manager")
		os.Exit(1)
	}
}
