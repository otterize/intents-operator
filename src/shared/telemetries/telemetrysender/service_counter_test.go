package telemetrysender

import (
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/stretchr/testify/suite"
	"testing"
)

const (
	eventTypeServiceDiscovered                = "ServiceDiscovered"
	eventTypeServiceDiscoveredFromServiceMesh = "ServiceDiscoveredFromServiceMesh"
)

type ServiceCounterSuite struct {
	suite.Suite
}

func (s *ServiceCounterSuite) TestAddService() {
	counter := NewUniqueCounter()
	component := telemetriesgql.Component{
		ComponentType:       "test-component",
		ComponentInstanceId: "test-component-instance-id",
		ContextId:           "test-context-id",
		Version:             "test-version",
		CloudClientId:       "test-cloud-client-id",
	}

	counter.IncrementCounter(component, eventTypeServiceDiscovered, Anonymize("service1"))
	counter.IncrementCounter(component, eventTypeServiceDiscovered, Anonymize("service2"))
	counter.IncrementCounter(component, eventTypeServiceDiscovered, Anonymize("service1"))
	counter.IncrementCounter(component, eventTypeServiceDiscovered, Anonymize("service3"))
	counter.IncrementCounter(component, eventTypeServiceDiscoveredFromServiceMesh, Anonymize("service1"))
	counter.IncrementCounter(component, eventTypeServiceDiscoveredFromServiceMesh, Anonymize("service2"))
	counter.IncrementCounter(component, eventTypeServiceDiscoveredFromServiceMesh, Anonymize("service1"))

	results := counter.Get()
	s.Require().Equal(2, len(results))
	s.Require().Contains(results, UniqueEventCount{Event: UniqueEventMetadata{Component: component, EventType: eventTypeServiceDiscovered}, Count: 3})
	s.Require().Contains(results, UniqueEventCount{Event: UniqueEventMetadata{Component: component, EventType: eventTypeServiceDiscoveredFromServiceMesh}, Count: 2})

	counter.IncrementCounter(component, eventTypeServiceDiscoveredFromServiceMesh, Anonymize("service3"))

	results = counter.Get()
	s.Require().Equal(2, len(results))
	s.Require().Contains(results, UniqueEventCount{Event: UniqueEventMetadata{Component: component, EventType: eventTypeServiceDiscovered}, Count: 3})
	s.Require().Contains(results, UniqueEventCount{Event: UniqueEventMetadata{Component: component, EventType: eventTypeServiceDiscoveredFromServiceMesh}, Count: 3})

	counter.Reset()

	results = counter.Get()
	s.Require().Equal(0, len(results))
}

func TestServiceCounterSuite(t *testing.T) {
	suite.Run(t, &ServiceCounterSuite{})
}
