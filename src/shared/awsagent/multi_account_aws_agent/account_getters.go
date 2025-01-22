package multi_account_aws_agent

import (
	"github.com/otterize/intents-operator/src/shared/awsagent"
	corev1 "k8s.io/api/core/v1"
	"regexp"
)

var AWSAccountIDRegex = regexp.MustCompile(`^(\d{12})$`)

func AppliesOnPod(pod *corev1.Pod) bool {
	if pod.Labels == nil {
		return false
	}
	value, found := pod.Labels[awsagent.AWSApplyOnPodLabel]
	// in multi-account-mode, the 'ApplyOnPodLabel' is used to specify the account ID
	return found && AWSAccountIDRegex.MatchString(value)
}

func AccountFromPod(pod *corev1.Pod) (string, bool) {
	value, found := pod.Labels[awsagent.AWSApplyOnPodLabel]
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
