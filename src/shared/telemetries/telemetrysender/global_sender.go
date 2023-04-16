package telemetrysender

import (
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/sirupsen/logrus"
	"sync"
)

var (
	stdOnce         = sync.Once{}
	std             *TelemetrySender
	globalComponent telemetriesgql.Component
)

func SetGlobalComponent(component telemetriesgql.Component) {
	globalComponent = component
}

func Send(eventType telemetriesgql.EventType, count int) {
	stdOnce.Do(func() {
		std = New()
	})
	if err := std.Send(globalComponent, eventType, count); err != nil {
		logrus.Warningf("failed sending telemetry. %s", err)
	}
}
