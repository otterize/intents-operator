package iampolicyagents

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	corev1 "k8s.io/api/core/v1"
)

type IAMPolicyAgent interface {
	IntentType() otterizev1alpha3.IntentType
	AppliesOnPod(pod *corev1.Pod) bool
	AddRolePolicyFromIntents(ctx context.Context, namespace string, accountName string, intentsServiceName string, intents []otterizev1alpha3.Intent) error
	DeleteRolePolicyFromIntents(ctx context.Context, intents otterizev1alpha3.ClientIntents) error
}
