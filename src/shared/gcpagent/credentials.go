package gcpagent

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

const (
	// GCPApplyOnPodLabel is used to mark pods that should be processed by the GCP agent to create an associated GCP service account
	GCPApplyOnPodLabel = "credentials-operator.otterize.com/create-gcp-sa"

	// ServiceManagedByGCPAgentLabel is used to mark service accounts that are managed by the GCP agent
	ServiceManagedByGCPAgentLabel = "credentials-operator.otterize.com/managed-by-gcp-agent"

	// GCPWorkloadIdentityAnnotation is used by GCP workload identity to link between service accounts
	GCPWorkloadIdentityAnnotation = "iam.gke.io/gcp-service-account"
	GCPWorkloadIdentityNotSet     = "false"
)

func (a *Agent) AppliesOnPod(pod *corev1.Pod) bool {
	return pod.Labels != nil && pod.Labels[GCPApplyOnPodLabel] == "true"
}

func (a *Agent) OnPodAdmission(ctx context.Context, pod *corev1.Pod, serviceAccount *corev1.ServiceAccount) (updated bool, err error) {
	if !a.AppliesOnPod(pod) {
		return false, nil
	}

	serviceAccount.Labels[ServiceManagedByGCPAgentLabel] = "true"
	serviceAccount.Annotations[GCPWorkloadIdentityAnnotation] = GCPWorkloadIdentityNotSet

	return true, nil
}

func (a *Agent) OnServiceAccountUpdate(ctx context.Context, serviceAccount *corev1.ServiceAccount) (updated bool, requeue bool, err error) {
	logger := logrus.WithFields(logrus.Fields{"serviceAccount": serviceAccount.Name, "namespace": serviceAccount.Namespace})

	if serviceAccount.Labels == nil || serviceAccount.Labels[ServiceManagedByGCPAgentLabel] != "true" {
		logger.Debug("ServiceAccount is not managed by the GCP agent, skipping")
		return false, false, nil
	}

	// Check if we should update the service account - if the annotation is not set
	if value, ok := serviceAccount.Annotations[GCPWorkloadIdentityAnnotation]; !ok || value != GCPWorkloadIdentityNotSet {
		logger.Debug("ServiceAccount GCP workload identity annotation is already set, skipping")
		return false, false, nil
	}

	// Annotate the namespace to connect workload identity
	requeue, err = a.AnnotateGKENamespace(ctx, serviceAccount.Namespace)
	if err != nil {
		return false, false, errors.Errorf("failed to annotate namespace: %w", err)
	}
	if requeue {
		// TODO: maybe do apierrors.IsConflict(err) check instead?
		return false, true, nil
	}

	// Create IAMServiceAccount (Creates a GCP service account)
	err = a.CreateAndConnectGSA(ctx, serviceAccount.Namespace, serviceAccount.Name)
	if err != nil {
		return false, false, errors.Errorf("failed to create and connect GSA: %w", err)
	}

	// Annotate the service account with the GCP IAM role
	gsaFullName := a.GetGSAFullName(serviceAccount.Namespace, serviceAccount.Name)
	serviceAccount.Annotations[GCPWorkloadIdentityAnnotation] = gsaFullName
	return true, false, nil
}

func (a *Agent) OnServiceAccountTermination(ctx context.Context, serviceAccount *corev1.ServiceAccount) error {
	logger := logrus.WithFields(logrus.Fields{"serviceAccount": serviceAccount.Name, "namespace": serviceAccount.Namespace})

	if serviceAccount.Labels == nil || serviceAccount.Labels[ServiceManagedByGCPAgentLabel] != "true" {
		logger.Debug("ServiceAccount is not managed by the GCP agent, skipping")
		return nil
	}

	return a.DeleteGSA(ctx, serviceAccount.Namespace, serviceAccount.Name)
}
