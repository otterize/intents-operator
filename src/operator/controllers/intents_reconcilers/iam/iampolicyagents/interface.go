package iampolicyagents

import (
	"context"
	otterizev2 "github.com/otterize/intents-operator/src/operator/api/v2"
	corev1 "k8s.io/api/core/v1"
)

type IAMPolicyAgent interface {
	// IntentType returns the type of the intent that the agent is responsible for
	IntentType() otterizev2.IntentType
	// AppliesOnPod returns true if the agent should manage the pod
	AppliesOnPod(pod *corev1.Pod) bool
	// AddRolePolicyFromIntents adds the IAM-specific role policy for the kubernetes service account, based on the intents
	AddRolePolicyFromIntents(ctx context.Context, namespace string, accountName string, intentsServiceName string, intents []otterizev2.Target, pod corev1.Pod) error
	// DeleteRolePolicyFromIntents deletes the IAM-specific role policy for the kubernetes service account, based on the intents
	DeleteRolePolicyFromIntents(ctx context.Context, intents otterizev2.ClientIntents) error
}
