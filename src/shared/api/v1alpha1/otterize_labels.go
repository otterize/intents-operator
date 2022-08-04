package v1alpha1

import (
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"strings"
)

const OtterizeServerLabelKey = "otterize/server"

// OtterizeLabelsDiffExists checks if a pod's labels need updating
func OtterizeLabelsDiffExists(pod *v1.Pod, otterizeAccessLabels map[string]string) bool {
	podLabels := pod.Labels
	for k, v := range podLabels {
		if isOtterizeAccessLabel(k) {
			if _, ok := otterizeAccessLabels[k]; !ok {
				logrus.Infof("Found Otterize access label to be removed %s=%s", k, v)
				// The pod has a label that needs to be removed
				return true
			}
		}
	}
	for k, v := range otterizeAccessLabels {
		if _, ok := pod.Labels[k]; !ok {
			logrus.Infof("Found missing label: %s=%s", k, v)
			// The pod is missing a label
			return true
		}
	}
	return false
}

// UpdateOtterizeAccessLabels updates a pod's labels with Otterize labels representing their intents
// The pod is also labeled with "otterize-client=true" to mark it as having intents
func UpdateOtterizeAccessLabels(pod *v1.Pod, otterizeAccessLabels map[string]string) *v1.Pod {
	cleanupOtterizeLabels(pod)
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

func cleanupOtterizeLabels(pod *v1.Pod) {
	for k, _ := range pod.Labels {
		if isOtterizeAccessLabel(k) {
			delete(pod.Labels, k)
		}
	}
}

func isOtterizeAccessLabel(s string) bool {
	if strings.Contains(s, "otterize/access") {
		return true
	}

	return false
}
