package iamcredentialsagents

import (
	"context"
	corev1 "k8s.io/api/core/v1"
)

type IAMCredentialsAgent interface {
	// FinalizerName returns the name of the finalizer that the agent will add to pods & service accounts
	FinalizerName() string

	// ServiceAccountLabel returns the label that the agent will use to mark service accounts with having / not having pods
	// managed by this agent
	ServiceAccountLabel() string

	// AppliesOnPod returns true if the agent should manage the pod
	AppliesOnPod(pod *corev1.Pod) bool

	// OnPodAdmission is called by the pod admission webhook when a pod is created or updated
	OnPodAdmission(ctx context.Context, pod *corev1.Pod, serviceAccount *corev1.ServiceAccount, dryRun bool) error

	// OnPodUpdate is called by the pod controller when a pod is updated
	OnPodUpdate(ctx context.Context, pod *corev1.Pod, serviceAccount *corev1.ServiceAccount) (updated bool, requeue bool, err error)

	// OnServiceAccountUpdate is called by the service account controller when a service account is updated
	OnServiceAccountUpdate(ctx context.Context, serviceAccount *corev1.ServiceAccount) (updated bool, requeue bool, err error)

	// OnServiceAccountTermination is called by the service account controller when a service account is terminated
	OnServiceAccountTermination(ctx context.Context, serviceAccount *corev1.ServiceAccount) error
}
