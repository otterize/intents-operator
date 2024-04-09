package v1alpha3

import (
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
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
// The pod is also labeled with "otterize-client=<hashed-client-name>" to mark it as having intents or being the client-side of an egress netpol
func UpdateOtterizeAccessLabels(pod *v1.Pod, serviceIdentity serviceidentity.ServiceIdentity, otterizeAccessLabels map[string]string) *v1.Pod {
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	// TODO: We should understand what kind
	pod = cleanupOtterizeLabelsAndAnnotations(pod)
	for k, v := range otterizeAccessLabels {
		pod.Labels[k] = v
	}
	pod.Labels[OtterizeClientLabelKey] = serviceIdentity.GetFormattedOtterizeIdentity()
	return pod
}

func HasOtterizeServiceLabel(pod *v1.Pod, labelValue string) bool {
	value, exists := pod.Labels[OtterizeServiceLabelKey]
	return exists && value == labelValue
}

func HasOtterizeDeprecatedServerLabel(pod *v1.Pod) bool {
	_, exists := pod.Labels[OtterizeServerLabelKeyDeprecated]
	return exists
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
	return strings.HasPrefix(s, OtterizeAccessLabelPrefix) || strings.HasPrefix(s, OtterizeServiceAccessLabelPrefix)
}

func CleanupOtterizeKubernetesServiceLabels(pod *v1.Pod) *v1.Pod {
	for k := range pod.Labels {
		if isOtterizeKubernetesServiceLabel(k) {
			delete(pod.Labels, k)
		}
	}

	return pod
}

func isOtterizeKubernetesServiceLabel(s string) bool {
	return strings.HasPrefix(s, OtterizeKubernetesServiceLabelKeyPrefix)
}

func GetOtterizeLabelsFromPod(pod *v1.Pod) map[string]string {
	otterizeLabels := make(map[string]string)
	for k, v := range pod.Labels {
		if isOtterizeAccessLabel(k) {
			otterizeLabels[k] = v
		}
	}

	return otterizeLabels
}
