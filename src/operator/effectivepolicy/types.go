package effectivepolicy

import (
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
)

type ClientCall struct {
	Service             serviceidentity.ServiceIdentity
	IntendedCall        v2alpha1.Target
	ObjectEventRecorder *injectablerecorder.ObjectEventRecorder
}

type Call struct {
	v2alpha1.Target
	ReferencingKubernetesServices []v1.Service
	EventRecorder                 *injectablerecorder.ObjectEventRecorder
}

type ServiceEffectivePolicy struct {
	Service                    serviceidentity.ServiceIdentity
	CalledBy                   []ClientCall
	Calls                      []Call
	ClientIntentsStatus        v2alpha1.IntentsStatus
	ClientIntentsEventRecorder *injectablerecorder.ObjectEventRecorder
}

func (s *ServiceEffectivePolicy) RecordOnClientsNormalEvent(eventType string, message string) {
	lo.ForEach(s.CalledBy, func(clientCall ClientCall, _ int) {
		clientCall.ObjectEventRecorder.RecordNormalEvent(eventType, message)
	})
}

func (s *ServiceEffectivePolicy) RecordOnClientsNormalEventf(eventType string, message string, args ...interface{}) {
	lo.ForEach(s.CalledBy, func(clientCall ClientCall, _ int) {
		clientCall.ObjectEventRecorder.RecordNormalEventf(eventType, message, args...)
	})
}

func (s *ServiceEffectivePolicy) RecordOnClientsWarningEvent(eventType string, message string) {
	lo.ForEach(s.CalledBy, func(clientCall ClientCall, _ int) {
		clientCall.ObjectEventRecorder.RecordWarningEvent(eventType, message)
	})
}

func (s *ServiceEffectivePolicy) RecordOnClientsWarningEventf(eventType string, message string, args ...interface{}) {
	lo.ForEach(s.CalledBy, func(clientCall ClientCall, _ int) {
		clientCall.ObjectEventRecorder.RecordWarningEventf(eventType, message, args...)
	})
}
