package errorreporter

import (
	bugsnagerrors "github.com/bugsnag/bugsnag-go/v2/errors"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesconfig"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/sirupsen/logrus"
)

func Init(componentType telemetriesgql.TelemetryComponentType, version string) {
	logrus.Infof("starting error telemetry for component '%s' with version '%s'", componentType, version)

	if !telemetriesconfig.IsErrorTelemetryEnabled() {
		logrus.Info("error reporting disabled")
	}

	initSender(componentType, version)

	hook, err := NewErrorReportingHook()
	if err != nil {
		logrus.WithError(err).Panic("failed to initialize bugsnag")
	}
	logrus.AddHook(hook)
}

func AutoNotify() {
	if err := recover(); err != nil {
		if logrusEntry, ok := err.(*logrus.Entry); ok {
			_ = sendToErrorTelemetry(logrusEntry, true)
			return
		}
		_ = sendErrorSync(errors.ErrorfWithSkip(2, "panic caught: %s", err).(*bugsnagerrors.Error), nil)
		panic(err)
	}
}
