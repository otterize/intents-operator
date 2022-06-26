package main

import (
	"context"
	"github.com/caarlos0/env/v6"
	"github.com/otterize/spifferize/src/admission_controller/pkg/admission_controller"
	"github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"syscall"
)

func setLogLevel() error {
	logLevelStr, ok := os.LookupEnv("LOG_LEVEL")
	if !ok || len(logLevelStr) == 0 {
		return nil
	}
	logLevel, err := logrus.ParseLevel(logLevelStr)
	if err != nil {
		return err
	}
	logrus.SetLevel(logLevel)
	return nil
}

func main() {
	err := setLogLevel()
	if err != nil {
		logrus.WithError(err).Fatal("Failed to set log level from env var")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var config admission_controller.AdmissionControllerConfig
	err = env.Parse(&config)
	if err != nil {
		logrus.WithError(err).Fatal("Parsing the env vars failed")
	}

	ac, err := admission_controller.NewAdmissionController(ctx, config)
	if err != nil {
		logrus.WithError(err).Fatal("Creating the admission controller failed")
	}

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-stopChan
		cancel()
	}()

	err = ac.RunForever()
	if err != nil {
		logrus.WithError(err).Fatal("Admission controller run returned an error")
	}
}
