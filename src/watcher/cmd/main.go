package main

import (
	otterizev1alpha1 "github.com/otterize/intents-operator/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/watcher/reconcilers"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(otterizev1alpha1.AddToScheme(scheme))
}

func main() {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		MetricsBindAddress: "0",
		Scheme:             scheme,
	})
	if err != nil {
		logrus.WithError(err).Panic("unable to start manager")
	}
	podWatcher := reconcilers.NewPodWatcher(mgr.GetClient())
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
