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

func Send(eventType telemetriesgql.EventType, data map[string]string) {
	stdOnce.Do(func() {
		std = New()
	})
	if err := std.Send(globalComponent, eventType, data); err != nil {
		logrus.Warningf("failed sending telemetry. %s", err)
	}
}
