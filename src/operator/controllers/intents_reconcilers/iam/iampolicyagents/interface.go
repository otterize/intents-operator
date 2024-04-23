package iampolicyagents

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	corev1 "k8s.io/api/core/v1"
)

type IAMPolicyAgent interface {
	// IntentType returns the type of the intent that the agent is responsible for
	IntentType() otterizev1alpha3.IntentType
	// AppliesOnPod returns true if the agent should manage the pod
	AppliesOnPod(pod *corev1.Pod) bool
	// AddRolePolicyFromIntents adds the IAM-specific role policy for the kubernetes service account, based on the intents
	AddRolePolicyFromIntents(ctx context.Context, namespace string, accountName string, intentsServiceName string, intents []otterizev1alpha3.Intent, pod corev1.Pod) error
	// DeleteRolePolicyFromIntents deletes the IAM-specific role policy for the kubernetes service account, based on the intents
	DeleteRolePolicyFromIntents(ctx context.Context, intents otterizev1alpha3.ClientIntents) error
}
