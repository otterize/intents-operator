package mirrorevents

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const approvedClientIntentsSuffix = "-approved"

// mirrorToClientIntentsEventRecorder wraps record.EventRecorder to copy events from ApprovedClientIntents to ClientIntents.
type mirrorToClientIntentsEventRecorder struct {
	recorder record.EventRecorder
	client.Client
}

// GetMirrorToClientIntentsEventRecorderFor creates a new mirrorToClientIntentsEventRecorder
func GetMirrorToClientIntentsEventRecorderFor(mgr manager.Manager, componentName string) record.EventRecorder {
	return &mirrorToClientIntentsEventRecorder{
		recorder: mgr.GetEventRecorderFor(componentName),
		Client:   mgr.GetClient(),
	}
}

// Event records an event and mirrors it if it's for an ApprovedClientIntent
func (r *mirrorToClientIntentsEventRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	r.recorder.Event(object, eventtype, reason, message)
	r.mirrorEvent(object, nil, eventtype, reason, message)
}

// Eventf records an event with formatted message
func (r *mirrorToClientIntentsEventRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	r.recorder.Eventf(object, eventtype, reason, messageFmt, args...)
	r.mirrorEvent(object, nil, eventtype, reason, messageFmt, args...)
}

// AnnotatedEventf records an event with annotations
func (r *mirrorToClientIntentsEventRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	r.recorder.AnnotatedEventf(object, annotations, eventtype, reason, messageFmt, args...)
	r.mirrorEvent(object, annotations, eventtype, reason, messageFmt, args...)
}

// mirrorEvent copies events from ApprovedClientIntents to the corresponding ClientIntent
func (r *mirrorToClientIntentsEventRecorder) mirrorEvent(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	metaObj, ok := object.(client.Object)
	if !ok {
		return // Ignore objects that are not client.Object
	}

	// Only mirror if the event is on an ApprovedClientIntent
	if metaObj.GetObjectKind().GroupVersionKind().Kind != "ApprovedClientIntents" {
		return
	}

	if len(metaObj.GetOwnerReferences()) != 1 {
		return // Ignore objects without exactly one owner
	}

	clientIntents := metaObj.GetOwnerReferences()[0]

	// Find the corresponding ClientIntent (assuming same name)
	clientIntent := &corev1.ObjectReference{
		Kind:       clientIntents.Kind,
		Name:       clientIntents.Name,
		Namespace:  metaObj.GetNamespace(),
		APIVersion: clientIntents.APIVersion,
		UID:        clientIntents.UID,
	}

	// Record event for ClientIntent
	r.recorder.Eventf(clientIntent, eventtype, reason, messageFmt, args...)
}
