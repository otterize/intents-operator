package main

import (
	"github.com/otterize/intents-operator/watcher/reconcilers"
	"github.com/sirupsen/logrus"
	ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{MetricsBindAddress: "0"})
	if err != nil {
		logrus.WithError(err).Panic("unable to start manager")
	}
	podWatcher := reconcilers.NewPodWatcher(mgr.GetClient())

	err = podWatcher.Register(mgr)
	if err != nil {
		logrus.WithError(err).Panic()
	}

	logrus.Infoln("## Starting Otterize Pod Watcher ##")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logrus.WithError(err).Panic("problem running manager")
	}
}
