package errorreporter

import (
	"context"
	"github.com/bugsnag/bugsnag-go/v2"
	"github.com/otterize/intents-operator/src/shared/logrus_bugsnag"
	"github.com/otterize/intents-operator/src/shared/telemetries/componentinfo"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesconfig"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type GoRoutineFunc func(ctx context.Context)

func RunWithErrorReport(ctx context.Context, callback GoRoutineFunc) {
	go func(c context.Context) {
		defer bugsnag.AutoNotify(c)
		callback(c)
	}(ctx)
}

func RunWithErrorReportAndRecover(ctx context.Context, name string, callback GoRoutineFunc) {
	go func(c context.Context) {
		defer bugsnag.Recover(c)
		defer func() {
			r := recover()
			if r != nil {
				logrus.Errorf("recovered from panic in %s", name)
			}
		}()
		callback(c)
	}(ctx)
}

func addComponentInfoToBugsnagEvent(componentType string, event *bugsnag.Event) {
	event.MetaData.Add("component", "componentType", componentType)
	event.MetaData.Add("component", "componentInstanceId", componentinfo.GlobalComponentInstanceId())
	event.MetaData.Add("component", "contextId", componentinfo.GlobalContextId())
	event.MetaData.Add("component", "cloudClientId", componentinfo.GlobalCloudClientId())
}

func Init(componentName string, version string) {
	if !viper.GetBool(telemetriesconfig.TelemetryErrorsEnabledKey) {
		logrus.Info("error reporting disabled")
		return
	}

	bugsnag.OnBeforeNotify(func(event *bugsnag.Event, _ *bugsnag.Configuration) error {
		addComponentInfoToBugsnagEvent(componentName, event)
		return nil
	})

	errorsServerAddress := viper.GetString(telemetriesconfig.TelemetryErrorsAddressKey)
	releaseStage := viper.GetString(telemetriesconfig.TelemetryErrorsStageKey)
	apiKey := viper.GetString(telemetriesconfig.TelemetryErrorsAPIKeyKey)

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
		Logger:          logrus.StandardLogger(),
	}
	bugsnag.Configure(conf)

	hook, err := logrus_bugsnag.NewBugsnagHook()
	if err != nil {
		logrus.WithError(err).Panic("failed to initialize bugsnag")
	}
	logrus.AddHook(hook)
}
