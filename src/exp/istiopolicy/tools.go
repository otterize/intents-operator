package istiopolicy

import corev1 "k8s.io/api/core/v1"

const (
	IstioProxyContainerName = "istio-proxy"
)

func IsPodPartOfIstioMesh(pod corev1.Pod) bool {
	for _, container := range pod.Spec.Containers {
		if container.Name == IstioProxyContainerName {
			return true
		}
	}
	return false
}
