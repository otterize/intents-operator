package telemetrysender

import (
	"context"
	"flag"
	"github.com/bugsnag/bugsnag-go/v2"
	"github.com/otterize/intents-operator/src/shared/telemetries/componentinfo"
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

func send(componentType telemetriesgql.ComponentType, eventType telemetriesgql.EventType, count int) {
	senderInitOnce.Do(func() {
		initSender()
	})

	component := currentComponent(componentType)
	err := sender.Send(component, eventType, count)
	if err != nil {
		logrus.Warningf("failed sending telemetry. %s", err)
	}
}

func incrementCounter(componentType telemetriesgql.ComponentType, eventType telemetriesgql.EventType, key string) {
	senderInitOnce.Do(func() {
		initSender()
	})

	component := currentComponent(componentType)
	err := sender.IncrementCounter(component, eventType, key)
	if err != nil {
		logrus.Warningf("failed sending telemetry. %s", err)
	}
}

func currentComponent(componentType telemetriesgql.ComponentType) telemetriesgql.Component {
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
	send(telemetriesgql.ComponentTypeIntentsOperator, eventType, count)
}

func SendNetworkMapper(eventType telemetriesgql.EventType, count int) {
	send(telemetriesgql.ComponentTypeNetworkMapper, eventType, count)
}

func SendCredentialsOperator(eventType telemetriesgql.EventType, count int) {
	send(telemetriesgql.ComponentTypeCredentialsOperator, eventType, count)
}

func IncrementUniqueCounterIntentOperator(eventType telemetriesgql.EventType, key string) {
	incrementCounter(telemetriesgql.ComponentTypeIntentsOperator, eventType, key)
}

func IncrementUniqueCounterNetworkMapper(eventType telemetriesgql.EventType, key string) {
	incrementCounter(telemetriesgql.ComponentTypeNetworkMapper, eventType, key)
}

func IncrementUniqueCounterCredentialsOperator(eventType telemetriesgql.EventType, key string) {
	incrementCounter(telemetriesgql.ComponentTypeCredentialsOperator, eventType, key)
}

func IntentsOperatorRunActiveReporter(ctx context.Context) {
	runActiveComponentReporter(ctx, telemetriesgql.ComponentTypeIntentsOperator)
}

func NetworkMapperRunActiveReporter(ctx context.Context) {
	runActiveComponentReporter(ctx, telemetriesgql.ComponentTypeNetworkMapper)
}

func CredentialsOperatorRunActiveReporter(ctx context.Context) {
	runActiveComponentReporter(ctx, telemetriesgql.ComponentTypeCredentialsOperator)
}

func runActiveComponentReporter(ctx context.Context, componentType telemetriesgql.ComponentType) {
	go func() {
		defer bugsnag.AutoNotify(ctx)
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
