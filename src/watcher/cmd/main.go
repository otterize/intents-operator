package main

import (
	"github.com/bombsimon/logrusr/v3"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/otterize/intents-operator/src/watcher/reconcilers"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	istiosecurityscheme "istio.io/client-go/pkg/apis/security/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(istiosecurityscheme.AddToScheme(scheme))
	utilruntime.Must(otterizev1alpha2.AddToScheme(scheme))
}

func main() {
	operatorconfig.InitCLIFlags()

	metricsAddr := viper.GetString(operatorconfig.MetricsAddrKey)
	enableLeaderElection := viper.GetBool(operatorconfig.EnableLeaderElectionKey)
	probeAddr := viper.GetString(operatorconfig.ProbeAddrKey)
	watchedNamespaces := viper.GetStringSlice(operatorconfig.WatchedNamespacesKey)

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
		logrus.WithError(err).Fatal("unable to start manager")
	}

	recorder := mgr.GetEventRecorderFor("intents-operator")
	podWatcher := reconcilers.NewPodWatcher(mgr.GetClient(), recorder, watchedNamespaces)

	nsWatcher := reconcilers.NewNamespaceWatcher(mgr.GetClient())

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

	logrus.Infoln("## Starting Otterize Watcher ##")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logrus.WithError(err).Panic("problem running manager")
	}
}
