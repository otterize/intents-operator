package errorreporter

import (
	"context"
	"github.com/bugsnag/bugsnag-go/v2"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/logrus_bugsnag"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/otterize/intents-operator/src/shared/telemetries/componentinfo"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesconfig"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type GoRoutineFunc func(ctx context.Context)

func addComponentInfoToBugsnagEvent(componentType string, event *bugsnag.Event) {
	event.MetaData.Add("component", "componentType", componentType)
	event.MetaData.Add("component", "componentInstanceId", componentinfo.GlobalComponentInstanceId())
	event.MetaData.Add("component", "contextId", componentinfo.GlobalContextId())
	event.MetaData.Add("component", "cloudClientId", componentinfo.GlobalCloudClientId())
}

type noopLogger struct{}

func (noopLogger) Printf(format string, v ...interface{}) {
	// Do nothing intentionally
}

func Init(componentName string, version string, apiKey string) {
	logrus.Infof("starting error telemetry for component '%s' with version '%s'", componentName, version)

	if !telemetriesconfig.IsErrorTelemetryEnabled() {
		logrus.Info("error reporting disabled")
		return
	}

	bugsnag.OnBeforeNotify(func(event *bugsnag.Event, _ *bugsnag.Configuration) error {
		addComponentInfoToBugsnagEvent(componentName, event)
		return nil
	})

	errorsServerAddress := viper.GetString(telemetriesconfig.TelemetryErrorsAddressKey)
	releaseStage := viper.GetString(telemetriesconfig.TelemetryErrorsStageKey)
	// send to staging if Otterize Cloud API is not the default
	if viper.GetString(otterizecloudclient.OtterizeAPIAddressKey) != otterizecloudclient.OtterizeAPIAddressDefault || isStagingVersion(version) {
		releaseStage = "staging"
	}

	conf := bugsnag.Configuration{
		Endpoints: bugsnag.Endpoints{
			Sessions: errorsServerAddress + "/sessions",
			Notify:   errorsServerAddress + "/notify",
		},
		ReleaseStage:    releaseStage,
		APIKey:          apiKey,
		AppVersion:      version,
		AppType:         componentName,
		ProjectPackages: []string{"main*", "github.com/otterize/**"},
		Logger:          noopLogger{},
		PanicHandler:    func() {},
	}
	bugsnag.Configure(conf)

	hook, err := logrus_bugsnag.NewBugsnagHook()
	if err != nil {
		logrus.WithError(err).Panic("failed to initialize bugsnag")
	}
	logrus.AddHook(hook)
}

func isStagingVersion(version string) bool {
	return strings.HasPrefix(version, "0.0.") || version == "0-local" || version == ""
}

func AutoNotify() {
	// Check if bugsnag is initialized, or notify will crash.
	if bugsnag.Config.APIKey == "" {
		return
	}

	if err := recover(); err != nil {
		const shouldNotifySync = true
		rawData := []any{bugsnag.SeverityError, bugsnag.SeverityReasonHandledPanic, shouldNotifySync}
		if logrusEntry, ok := err.(*logrus.Entry); ok {
			_ = logrus_bugsnag.SendToBugsnag(logrusEntry, rawData...)
			return
		}
		_ = bugsnag.Notify(errors.ErrorfWithSkip(2, "panic caught: %s", err), rawData...)
		panic(err)
	}
}
