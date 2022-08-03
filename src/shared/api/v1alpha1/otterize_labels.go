package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	"strings"
)

const OtterizeDestServerLabelKey = "otterize/server"

// HasMissingOtterizeLabels checks if a pod's labels need updating
func HasMissingOtterizeLabels(pod *v1.Pod, otterizeAccessLabels map[string]string) bool {
	for k, _ := range otterizeAccessLabels {
		if _, ok := pod.Labels[k]; !ok {
			return true
		}
	}
	return false
}

// UpdateOtterizeAccessLabels updates a pod's labels with Otterize labels representing their intents
// The pod is also labeled with "otterize-client=true" to mark it as having intents
func UpdateOtterizeAccessLabels(pod *v1.Pod, otterizeAccessLabels map[string]string) *v1.Pod {
	for k, v := range otterizeAccessLabels {
		pod.Labels[k] = v
	}
	pod.Labels[OtterizeMarkerLabelKey] = "true"
	return pod
}

func HasOtterizeServerLabel(pod *v1.Pod) bool {
	_, exists := pod.Labels[OtterizeDestServerLabelKey]
	return exists
}

// CleanupOtterizeIntentLabels Removes intent related labels from pods
// Returns the pod's label map without Otterize labels
func CleanupOtterizeIntentLabels(pod v1.Pod) v1.Pod {
	postCleanupLabels := map[string]string{}

	for k, v := range pod.Labels {
		if !isOtterizeLabelKey(k) {
			postCleanupLabels[k] = v
		}
	}

	pod.Labels = postCleanupLabels
	return pod
}

func isOtterizeLabelKey(s string) bool {
	if strings.Contains(s, OtterizeAccessLabelKey) || strings.Contains(s, OtterizeMarkerLabelKey) {
		return true
	}

	return false
}
