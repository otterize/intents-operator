package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/otterize/spifferize/src/operator/controllers"
	spire_client "github.com/otterize/spifferize/src/spire-client"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	entryv1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/entry/v1"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

const (
	socketPath = "unix:////run/spire/sockets/agent.sock"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "spifferize",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	source, err := workloadapi.New(context.Background(), workloadapi.WithAddr(socketPath))
	if err != nil {
		setupLog.Error(err, "unable to start source")
		os.Exit(1)
	}
	defer source.Close()
	setupLog.Info("API Initialized")
	svid, err := source.FetchX509SVID(context.Background())
	if err != nil {
		setupLog.Error(err, "failed to get svid")
		os.Exit(1)
	}
	bundle, err := source.FetchX509Bundles(context.Background())
	if err != nil {
		logrus.Error(err, "failed to get bundle")
		os.Exit(1)
	}

	setupLog.Info(fmt.Sprintf("svid: %s", svid.ID))

	entryClient, err := spire_client.NewEntryClientFromTCP("spire-server.spire:8081", svid, bundle)
	if err != nil {
		setupLog.Error(err, "failed to connect to server")
		os.Exit(1)
	}

	entries, err := entryClient.ListEntries(context.Background(), &entryv1.ListEntriesRequest{})
	if err != nil {
		setupLog.Error(err, "failed to list entries")
		os.Exit(1)
	}
	setupLog.Info(fmt.Sprintf("%v", entries))

	if err = (&controllers.PodReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Pod"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Pod")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("check", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
