package main

import (
	"context"
	"flag"
	"github.com/otterize/spifferize/src/operator/controllers"
	spire_client "github.com/otterize/spifferize/src/spire-client"
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

func initSpireClient(ctx context.Context, spireServerAddr string) (spire_client.ServerClient, error) {
	// fetch SVID & bundle through spire-agent API
	source, err := workloadapi.New(ctx, workloadapi.WithAddr(socketPath))
	if err != nil {
		return nil, err
	}
	defer source.Close()

	svid, err := source.FetchX509SVID(ctx)
	if err != nil {
		return nil, err
	}
	bundle, err := source.FetchX509Bundles(ctx)
	if err != nil {
		return nil, err
	}

	// use SVID & bundle to connect to spire-server API
	conf := spire_client.ServerClientConfig{
		ServerAddress: spireServerAddr,
		ClientSVID:    svid,
		ClientBundle:  bundle,
	}

	serverClient, err := spire_client.NewServerClient(conf)
	if err != nil {
		return nil, err
	}
	setupLog.Info("Successfully connected to SPIRE server",
		"server", spireServerAddr)
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
		LeaderElectionID:       "otterize/spifferize-operator",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	spireClient, err := initSpireClient(context.TODO(), spireServerAddr)
	if err != nil {
		setupLog.Error(err, "failed to connect to spire server")
		os.Exit(1)
	}
	defer spireClient.Close()

	if err = (&controllers.PodReconciler{
		Client:      mgr.GetClient(),
		Log:         ctrl.Log.WithName("controllers").WithName("Pod"),
		Scheme:      mgr.GetScheme(),
		SpireClient: spireClient,
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
