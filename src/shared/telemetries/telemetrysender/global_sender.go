package telemetrysender

import (
	"flag"
	"github.com/google/uuid"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/sirupsen/logrus"
	"sync"
)

var (
	senderInitOnce           = sync.Once{}
	sender                   *TelemetrySender
	globalPlatformIdentifier string
	globalRuntimeIdentifier  string
)

func SetGlobalPlatformIdentifier(platformIdentifier string) {
	globalPlatformIdentifier = platformIdentifier
}

func send(componentType telemetriesgql.ComponentType, eventType telemetriesgql.EventType, count int) {
	senderInitOnce.Do(func() {
		sender = New()
		globalRuntimeIdentifier = uuid.NewString()
		if flag.Lookup("test.v") != nil {
			logrus.Infof("Disabling telemetry sender because this is a test")
			sender.enabled = false
		}
	})
	if err := sender.Send(
		telemetriesgql.Component{
			ComponentType:      componentType,
			Identifier:         globalRuntimeIdentifier,
			PlatformIdentifier: globalPlatformIdentifier,
		}, eventType, count); err != nil {
		logrus.Warningf("failed sending telemetry. %s", err)
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
