package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	"strings"
)

const OtterizeServerLabelKey = "otterize/server"

// HasMissingOtterizeLabels checks if a pod's labels need updating
func HasMissingOtterizeLabels(pod *v1.Pod, otterizeAccessLabels map[string]string) bool {
	for k, v := range otterizeAccessLabels {
		if currVal, ok := pod.Labels[k]; !ok {
			return true
		} else if v != currVal {
			return true
		}
	}
	return false
}

// UpdateOtterizeAccessLabels updates a pod's labels with Otterize labels representing their intents
// The pod is also labeled with "otterize-client=true" to mark it as having intents
func UpdateOtterizeAccessLabels(pod v1.Pod, otterizeAccessLabels map[string]string) v1.Pod {
	pod = cleanupOtterizeIntentLabels(pod)
	for k, v := range otterizeAccessLabels {
		pod.Labels[k] = v
	}
	pod.Labels[OtterizeMarkerLabelKey] = "true"
	return pod
}

func HasOtterizeServerLabel(pod *v1.Pod) bool {
	_, exists := pod.Labels[OtterizeServerLabelKey]
	return exists
}

// cleanupOtterizeIntentLabels Removes intent related labels from pods
// Returns the pod's label map without Otterize labels
func cleanupOtterizeIntentLabels(pod v1.Pod) v1.Pod {
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
	if strings.HasPrefix(s, "otterize/access") || strings.HasPrefix(s, "otterize-client") {
		return true
	}
	return false
}
