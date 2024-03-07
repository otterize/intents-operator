package testutils

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func GetTestServiceRequestSchema() ctrl.Request {
	return ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testServiceAccountName},
	}
}

func GetTestServiceSchema() corev1.ServiceAccount {
	return corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testServiceAccountName,
			Namespace: testNamespace,
		},
	}
}
