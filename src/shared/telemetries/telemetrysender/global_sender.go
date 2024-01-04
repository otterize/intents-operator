package telemetrysender

import (
	"context"
	"flag"
	"github.com/otterize/intents-operator/src/shared/telemetries/componentinfo"
	"github.com/otterize/intents-operator/src/shared/telemetries/errorreporter"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesconfig"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"sync"
	"time"
)

var (
	senderInitOnce = sync.Once{}
	sender         *TelemetrySender
)

func send(componentType telemetriesgql.TelemetryComponentType, eventType telemetriesgql.EventType, count int) {
	senderInitOnce.Do(func() {
		initSender()
	})

	component := currentComponent(componentType)
	err := sender.Send(component, eventType, count)
	if err != nil {
		logrus.Warningf("failed sending telemetry. %s", err)
	}
}

func incrementCounter(componentType telemetriesgql.TelemetryComponentType, eventType telemetriesgql.EventType, key string) {
	senderInitOnce.Do(func() {
		initSender()
	})

	component := currentComponent(componentType)
	err := sender.IncrementCounter(component, eventType, key)
	if err != nil {
		logrus.Warningf("failed sending telemetry. %s", err)
	}
}

func currentComponent(componentType telemetriesgql.TelemetryComponentType) telemetriesgql.Component {
	return telemetriesgql.Component{
		CloudClientId:       componentinfo.GlobalCloudClientId(),
		ComponentType:       componentType,
		ComponentInstanceId: componentinfo.GlobalComponentInstanceId(),
		ContextId:           componentinfo.GlobalContextId(),
		Version:             componentinfo.GlobalVersion(),
	}
}

func initSender() {
	sender = New()
	componentinfo.SetGlobalComponentInstanceId()
	if flag.Lookup("test.v") != nil {
		logrus.Infof("Disabling telemetry sender because this is a test")
		sender.enabled = false
	}
}

func SendIntentOperator(eventType telemetriesgql.EventType, count int) {
	send(telemetriesgql.TelemetryComponentTypeIntentsOperator, eventType, count)
}

func SendNetworkMapper(eventType telemetriesgql.EventType, count int) {
	send(telemetriesgql.TelemetryComponentTypeNetworkMapper, eventType, count)
}

func SendCredentialsOperator(eventType telemetriesgql.EventType, count int) {
	send(telemetriesgql.TelemetryComponentTypeCredentialsOperator, eventType, count)
}

func IncrementUniqueCounterIntentOperator(eventType telemetriesgql.EventType, key string) {
	incrementCounter(telemetriesgql.TelemetryComponentTypeIntentsOperator, eventType, key)
}

func IncrementUniqueCounterNetworkMapper(eventType telemetriesgql.EventType, key string) {
	incrementCounter(telemetriesgql.TelemetryComponentTypeNetworkMapper, eventType, key)
}

func IncrementUniqueCounterCredentialsOperator(eventType telemetriesgql.EventType, key string) {
	incrementCounter(telemetriesgql.TelemetryComponentTypeCredentialsOperator, eventType, key)
}

func IntentsOperatorRunActiveReporter(ctx context.Context) {
	runActiveComponentReporter(ctx, telemetriesgql.TelemetryComponentTypeIntentsOperator)
}

func NetworkMapperRunActiveReporter(ctx context.Context) {
	runActiveComponentReporter(ctx, telemetriesgql.TelemetryComponentTypeNetworkMapper)
}

func CredentialsOperatorRunActiveReporter(ctx context.Context) {
	runActiveComponentReporter(ctx, telemetriesgql.TelemetryComponentTypeCredentialsOperator)
}

func runActiveComponentReporter(ctx context.Context, componentType telemetriesgql.TelemetryComponentType) {
	go func() {
		defer errorreporter.AutoNotify()
		activeInterval := viper.GetDuration(telemetriesconfig.TelemetryActiveIntervalKey)
		reporterTicker := time.NewTicker(activeInterval)
		logrus.Info("Starting active component reporter")
		send(componentType, telemetriesgql.EventTypeActive, 0)

		for {
			select {
			case <-reporterTicker.C:
				send(componentType, telemetriesgql.EventTypeActive, 0)

			case <-ctx.Done():
				logrus.Info("Active component reporter exit")
				return
			}
		}
	}()
}
