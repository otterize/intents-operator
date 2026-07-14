package istiopolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsPodPartOfIstioMesh_NoSidecar(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
				},
			},
		},
	}

	assert.False(t, IsPodPartOfIstioMesh(pod))
}

func TestIsPodPartOfIstioMesh_ContainerSidecar(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
				},
				{
					Name: "istio-proxy",
				},
			},
		},
	}

	assert.True(t, IsPodPartOfIstioMesh(pod))
}

func TestIsPodPartOfIstioMesh_InitContainerSidecar(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
				},
			},
			InitContainers: []corev1.Container{
				{
					Name: "istio-proxy",
				},
			},
		},
	}

	assert.True(t, IsPodPartOfIstioMesh(pod))
}

func TestIsPodPartOfIstioMesh_Annotation(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			Annotations: map[string]string{
				"sidecar.istio.io/status": "some-status-json",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
				},
			},
		},
	}

	assert.True(t, IsPodPartOfIstioMesh(pod))
}
