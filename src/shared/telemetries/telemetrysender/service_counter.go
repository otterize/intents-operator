package telemetrysender

import (
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"k8s.io/apimachinery/pkg/util/sets"
)

type UniqueEventMetadata struct {
	Component telemetriesgql.Component
	EventType telemetriesgql.EventType
}

type UniqueEvent struct {
	Event UniqueEventMetadata
	Key   string
}

type UniqueEventCount struct {
	Event UniqueEventMetadata
	Count int
}

type CountersByEvent map[UniqueEventMetadata]sets.Set[string]

type UniqueCounter struct {
	countersByType CountersByEvent
}

func NewUniqueCounter() *UniqueCounter {
	return &UniqueCounter{
		countersByType: make(CountersByEvent),
	}
}

func (s *UniqueCounter) IncrementCounter(component telemetriesgql.Component, eventType telemetriesgql.EventType, eventKey string) {
	event := UniqueEventMetadata{
		Component: component,
		EventType: eventType,
	}

	if _, ok := s.countersByType[event]; !ok {
		s.countersByType[event] = make(sets.Set[string])
	}

	s.countersByType[event].Insert(eventKey)
}

func (s *UniqueCounter) Get() []UniqueEventCount {
	counts := make([]UniqueEventCount, 0)
	for event := range s.countersByType {
		count := s.getCount(event)
		counts = append(counts, UniqueEventCount{Event: event, Count: count})
	}

	return counts
}

func (s *UniqueCounter) getCount(event UniqueEventMetadata) int {
	if _, ok := s.countersByType[event]; !ok {
		return 0
	}

	return s.countersByType[event].Len()
}

func (s *UniqueCounter) Reset() {
	s.countersByType = make(CountersByEvent)
}
