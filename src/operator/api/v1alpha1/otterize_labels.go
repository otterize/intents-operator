package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	"strings"
)

// IsMissingOtterizeAccessLabels checks if a pod's labels need updating
func IsMissingOtterizeAccessLabels(pod *v1.Pod, otterizeAccessLabels map[string]string) bool {
	podOtterizeAccessLabels := GetOtterizeLabelsFromPod(pod)
	if len(podOtterizeAccessLabels) != len(otterizeAccessLabels) {
		return true
	}

	// Length is equal, check for diff in keys
	for k := range podOtterizeAccessLabels {
		if _, ok := otterizeAccessLabels[k]; !ok {
			return true
		}
	}
	return false
}

// UpdateOtterizeAccessLabels updates a pod's labels with Otterize labels representing their intents
// The pod is also labeled with "otterize-client=true" to mark it as having intents
func UpdateOtterizeAccessLabels(pod *v1.Pod, otterizeAccessLabels map[string]string) *v1.Pod {
	pod = cleanupOtterizeLabelsAndAnnotations(pod)
	for k, v := range otterizeAccessLabels {
		pod.Labels[k] = v
	}
	pod.Labels[OtterizeClientLabelKey] = "true"
	return pod
}

func HasOtterizeServerLabel(pod *v1.Pod, labelValue string) bool {
	value, exists := pod.Labels[OtterizeServerLabelKey]
	return exists && value == labelValue
}

func cleanupOtterizeLabelsAndAnnotations(pod *v1.Pod) *v1.Pod {
	for k := range pod.Labels {
		if isOtterizeAccessLabel(k) {
			delete(pod.Labels, k)
		}
	}

	delete(pod.Annotations, AllIntentsRemovedAnnotation)

	return pod
}

func isOtterizeAccessLabel(s string) bool {
	return strings.HasPrefix(s, OtterizeAccessLabelPrefix)
}

func GetOtterizeLabelsFromPod(pod *v1.Pod) map[string]string {
	otterizeLabels := map[string]string{}
	for k, v := range pod.Labels {
		if isOtterizeAccessLabel(k) {
			otterizeLabels[k] = v
		}
	}

	return otterizeLabels
}
