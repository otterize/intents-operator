package errorreporter

import (
	"flag"
	"fmt"
	bugsnagerrors "github.com/bugsnag/bugsnag-go/v2/errors"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/otterize/intents-operator/src/shared/telemetries/componentinfo"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/otterize/intents-operator/src/shared/version"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var (
	sender *ErrorSender
)

func errorFromErrorWithStackFrames(err *bugsnagerrors.Error, metadata map[string]string) telemetriesgql.Error {
	gqlErr := telemetriesgql.Error{
		Message:    lo.ToPtr(err.Error()),
		ErrorClass: lo.ToPtr(fmt.Sprintf("%T", err.Err)),
	}

	if err.Cause != nil {
		gqlErr.Cause = lo.ToPtr(errorFromErrorWithStackFrames(err.Cause, metadata))
	}

	for _, stackFrame := range err.StackFrames() {
		gqlErr.Stack = append(gqlErr.Stack, &telemetriesgql.StackFrame{
			File:       lo.ToPtr(stackFrame.File),
			LineNumber: lo.ToPtr(stackFrame.LineNumber),
			Name:       lo.ToPtr(stackFrame.Name),
			Package:    lo.ToPtr(stackFrame.Package),
		})
	}

	for k, v := range metadata {
		gqlErr.Metadata = append(gqlErr.Metadata, &telemetriesgql.MetadataEntry{
			Key:   lo.ToPtr(k),
			Value: lo.ToPtr(v),
		})
	}

	return gqlErr
}

func sendErrorAsync(err *bugsnagerrors.Error, metadata map[string]string) {
	if sender == nil {
		logrus.Debug("failed to send error because sender is not initialized")
		return
	}

	gqlErr := errorFromErrorWithStackFrames(err, metadata)

	sendErr := sender.SendAsync(gqlErr)
	if sendErr != nil {
		logrus.Debugf("failed reporting error: %s", err.Error())
	}
}

func sendErrorSync(err *bugsnagerrors.Error, metadata map[string]string) error {
	if sender == nil {
		return errors.New("failed to send error because sender is not initialized")
	}

	gqlErr := errorFromErrorWithStackFrames(err, metadata)

	sendErr := sender.SendSync([]*telemetriesgql.Error{&gqlErr})
	if sendErr != nil {
		return errors.Errorf("failed reporting error: %w", sendErr)
	}

	return nil
}

func currentComponent(componentType telemetriesgql.TelemetryComponentType) telemetriesgql.Component {
	return telemetriesgql.Component{
		CloudClientId:       viper.GetString(otterizecloudclient.ApiClientIdKey),
		ComponentType:       componentType,
		ComponentInstanceId: componentinfo.GlobalComponentInstanceId(),
		ContextId:           componentinfo.GlobalContextId(),
		Version:             version.Version(),
	}
}

func initSender(componentType telemetriesgql.TelemetryComponentType) {
	sender = New(componentType)
	if flag.Lookup("test.v") != nil {
		logrus.Infof("Disabling telemetry sender because this is a test")
		sender.enabled = false
	}
}
