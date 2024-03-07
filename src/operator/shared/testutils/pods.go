package testutils

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func GetTestPodRequestSchema() ctrl.Request {
	return ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testPodName},
	}
}

func GetTestPodSchema() corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              testPodName,
			Namespace:         testNamespace,
			UID:               testPodUID,
			DeletionTimestamp: nil,
		},
		Spec: corev1.PodSpec{ServiceAccountName: testServiceAccountName},
	}
}
