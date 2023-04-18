package telemetrysender

import (
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/sirupsen/logrus"
	"sync"
)

var (
	senderInitOnce  = sync.Once{}
	sender          *TelemetrySender
	globalComponent telemetriesgql.Component
)

func SetGlobalComponent(component telemetriesgql.Component) {
	globalComponent = component
}

func Send(eventType telemetriesgql.EventType, count int) {
	senderInitOnce.Do(func() {
		sender = New()
	})
	if err := sender.Send(globalComponent, eventType, count); err != nil {
		logrus.Warningf("failed sending telemetry. %s", err)
	}
}
