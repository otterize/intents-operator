package telemetrysender

import (
	"flag"
	"github.com/google/uuid"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/sirupsen/logrus"
	"sync"
)

var (
	senderInitOnce            = sync.Once{}
	sender                    *TelemetrySender
	globalContextId           string
	globalComponentInstanceId string
)

func SetGlobalContextId(contextId string) {
	globalContextId = contextId
}

func send(componentType telemetriesgql.ComponentType, eventType telemetriesgql.EventType, count int) {
	senderInitOnce.Do(func() {
		sender = New()
		globalComponentInstanceId = uuid.NewString()
		if flag.Lookup("test.v") != nil {
			logrus.Infof("Disabling telemetry sender because this is a test")
			sender.enabled = false
		}
	})
	if err := sender.Send(
		telemetriesgql.Component{
			ComponentType:       componentType,
			ComponentInstanceId: globalComponentInstanceId,
			ContextId:           globalContextId,
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
