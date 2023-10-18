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
	globalVersion             string
	globalCloudClientId       string
)

func SetGlobalContextId(contextId string) {
	globalContextId = contextId
}

func SetGlobalVersion(version string) {
	globalVersion = version
}

func SetGlobalCloudClientId(clientId string) {
	globalCloudClientId = clientId
}

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
		CloudClientId:       globalCloudClientId,
		ComponentType:       componentType,
		ComponentInstanceId: globalComponentInstanceId,
		ContextId:           globalContextId,
		Version:             globalVersion,
	}
}

func initSender() {
	sender = New()
	globalComponentInstanceId = uuid.NewString()
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
