package multi_account_aws_agent

import (
	"github.com/otterize/intents-operator/src/shared/awsagent"
	corev1 "k8s.io/api/core/v1"
)

func AccountFromPod(pod *corev1.Pod) (string, bool) {
	value, found := pod.Labels[awsagent.ApplyOnPodLabel]
	if !found {
		return "", false
	}

	if value == "true" {
		return "", false
	}

	return value, true
}

func AccountFromServiceAccount(serviceAccount *corev1.ServiceAccount) (string, bool) {
	value, found := serviceAccount.Annotations[awsagent.ServiceAccountAWSAccountIDAnnotation]
	if !found {
		return "", false
	}

	if value == "true" {
		return "", false
	}

	return value, true
}
