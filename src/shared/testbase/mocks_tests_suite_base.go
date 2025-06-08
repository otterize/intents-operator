package testbase

import (
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"go.uber.org/mock/gomock"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"strings"
	"time"
)

const (
	FakeRecorderBufferSize = 100
)

type MocksSuiteBase struct {
	TestSuiteBase
	Controller   *gomock.Controller
	Recorder     *record.FakeRecorder
	Client       *mocks.MockClient
	StatusWriter *mocks.MockSubResourceWriter
	Scheme       *runtime.Scheme
}

func (s *MocksSuiteBase) SetupTest() {
	s.Controller = gomock.NewController(s.T())
	s.Client = mocks.NewMockClient(s.Controller)
	s.Recorder = record.NewFakeRecorder(FakeRecorderBufferSize)
	s.StatusWriter = mocks.NewMockSubResourceWriter(s.Controller)
	s.Scheme = runtime.NewScheme()
	s.Require().NoError(otterizev2alpha1.AddToScheme(s.Scheme))
}

func (s *MocksSuiteBase) TearDownTest() {
	s.ExpectNoEvent(s.Recorder)
	s.Recorder = nil
	s.Client = nil
	s.Controller.Finish()
}

func (s *MocksSuiteBase) ExpectEvent(expectedEventReason string) {
	s.ExpectEventsForRecorder(s.Recorder, expectedEventReason)
}

func (s *MocksSuiteBase) ExpectEventsForRecorder(recorder *record.FakeRecorder, expectedEventReason string) {
	select {
	case event := <-recorder.Events:
		s.Require().Contains(event, expectedEventReason)
	default:
		s.Failf("Expected event not found", "Event '%v'", expectedEventReason)
	}
}

func (s *MocksSuiteBase) ExpectEventsOrderAndCountDontMatter(expectedEventReasons ...string) {
	for {
		select {
		case event := <-s.Recorder.Events:
			found := false
			for _, reason := range expectedEventReasons {
				if strings.Contains(event, reason) {
					found = true
					break
				}
			}
			s.Require().True(found, "Expected event that contains one of the following reasons: %v.\nFound: %s", expectedEventReasons, event)
		case <-time.After(100 * time.Millisecond):
			return
		}
	}
}
