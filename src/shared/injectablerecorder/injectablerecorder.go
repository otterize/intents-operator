package injectablerecorder

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

type InjectableRecorder struct {
	Recorder record.EventRecorder
}

func (r *InjectableRecorder) InjectRecorder(recorder record.EventRecorder) {
	r.Recorder = recorder
}

// RecordWarningEvent - reason: This should be a short, machine understandable string that gives the reason
// for the transition into the object's current status.
func (r *InjectableRecorder) RecordWarningEvent(object runtime.Object, reason string, message string) {
	r.Recorder.Event(object, v1.EventTypeWarning, reason, message)
}

// RecordWarningEventf - reason: This should be a short, machine understandable string that gives the reason
// for the transition into the object's current status.
func (r *InjectableRecorder) RecordWarningEventf(object runtime.Object, reason string, message string, args ...interface{}) {
	r.Recorder.Eventf(object, v1.EventTypeWarning, reason, message, args...)
}

// RecordNormalEvent - reason: This should be a short, machine understandable string that gives the reason
// for the transition into the object's current status.
func (r *InjectableRecorder) RecordNormalEvent(object runtime.Object, reason string, message string) {
	r.Recorder.Event(object, v1.EventTypeNormal, reason, message)
}

// RecordNormalEventf - reason: This should be a short, machine understandable string that gives the reason
// for the transition into the object's current status.
func (r *InjectableRecorder) RecordNormalEventf(object runtime.Object, reason string, message string, args ...interface{}) {
	r.Recorder.Eventf(object, v1.EventTypeNormal, reason, message, args...)
}
