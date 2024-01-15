package injectablerecorder

import "k8s.io/apimachinery/pkg/runtime"

type ObjectEventRecorder struct {
	recorder *InjectableRecorder
	object   runtime.Object
}

func NewObjectEventRecorder(recorder *InjectableRecorder, object runtime.Object) *ObjectEventRecorder {
	return &ObjectEventRecorder{recorder: recorder, object: object}
}

// RecordWarningEvent - reason: This should be a short, machine understandable string that gives the reason
// for the transition into the object's current status.
func (r *ObjectEventRecorder) RecordWarningEvent(reason string, message string) {
	r.recorder.RecordWarningEvent(r.object, reason, message)
}

// RecordWarningEventf - reason: This should be a short, machine understandable string that gives the reason
// for the transition into the object's current status.
func (r *ObjectEventRecorder) RecordWarningEventf(reason string, message string, args ...interface{}) {
	r.recorder.RecordWarningEventf(r.object, reason, message, args...)
}

// RecordNormalEvent - reason: This should be a short, machine understandable string that gives the reason
// for the transition into the object's current status.
func (r *ObjectEventRecorder) RecordNormalEvent(reason string, message string) {
	r.recorder.RecordNormalEvent(r.object, reason, message)
}

// RecordNormalEventf - reason: This should be a short, machine understandable string that gives the reason
// for the transition into the object's current status.
func (r *ObjectEventRecorder) RecordNormalEventf(reason string, message string, args ...interface{}) {
	r.recorder.RecordNormalEventf(r.object, reason, message, args...)
}
