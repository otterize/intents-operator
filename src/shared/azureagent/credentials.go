package azureagent

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

const (
	// AzureApplyOnPodLabel is used to mark pods that should be processed by the Azure agent to create an associated Azure identity & role assignment
	AzureApplyOnPodLabel = "credentials-operator.otterize.com/create-azure-role-assignment"

	// ServiceManagedByAzureAgentLabel is used to mark service accounts that are managed by the Azure agent
	ServiceManagedByAzureAgentLabel = "credentials-operator.otterize.com/managed-by-azure-agent"

	// AzureUseWorkloadIdentityLabel is used by the azure workload identity mechanism to mark pods that should use workload identity
	AzureUseWorkloadIdentityLabel = "azure.workload.identity/use"

	// AzureWorkloadIdentityClientIdAnnotation is used by the azure workload identity mechanism to link between service accounts and user assigned identities
	AzureWorkloadIdentityClientIdAnnotation = "azure.workload.identity/client-id"

	// OtterizeAzureWorkloadIdentityClientIdAnnotation is used to annotate the pod with its azure workload identity client ID, to trigger the azure workload identity webhook on it
	OtterizeAzureWorkloadIdentityClientIdAnnotation = "credentials-operator.otterize.com/azure-workload-identity-client-id"
)

func (a *Agent) AppliesOnPod(pod *corev1.Pod) bool {
	return pod.Labels != nil && pod.Labels[AzureApplyOnPodLabel] == "true"
}

func (a *Agent) OnPodAdmission(ctx context.Context, pod *corev1.Pod, serviceAccount *corev1.ServiceAccount) (updated bool, err error) {
	logger := logrus.WithFields(logrus.Fields{"serviceAccount": serviceAccount.Name, "namespace": serviceAccount.Namespace})

	if !a.AppliesOnPod(pod) {
		return false, nil
	}

	serviceAccount.Labels[ServiceManagedByAzureAgentLabel] = "true"

	//pod.Labels[AzureUseWorkloadIdentityLabel] = "true"

	// get or create the user assigned identity, ensuring the identity & federated credentials are in-place
	identity, err := a.getOrCreateUserAssignedIdentity(ctx, serviceAccount.Namespace, serviceAccount.Name)
	if err != nil {
		return false, errors.Errorf("failed to create user assigned identity: %w", err)
	}

	clientId := *identity.Properties.ClientID

	if serviceAccount.Annotations[AzureWorkloadIdentityClientIdAnnotation] == clientId {
		// existing identity matches the annotated identity
		return false, nil
	}

	logger.WithField("identity", *identity.Name).WithField("clientId", clientId).
		Info("Annotating service account with managed identity client ID")

	serviceAccount.Annotations[AzureWorkloadIdentityClientIdAnnotation] = clientId
	// we additionally annotate the pod with the client ID to trigger the azure workload identity webhook on it
	pod.Annotations[OtterizeAzureWorkloadIdentityClientIdAnnotation] = clientId
	return true, nil
}

func (a *Agent) OnServiceAccountUpdate(ctx context.Context, serviceAccount *corev1.ServiceAccount) (updated bool, requeue bool, err error) {
	logger := logrus.WithFields(logrus.Fields{"serviceAccount": serviceAccount.Name, "namespace": serviceAccount.Namespace})

	if serviceAccount.Labels == nil || serviceAccount.Labels[ServiceManagedByAzureAgentLabel] != "true" {
		logger.Debug("ServiceAccount is not managed by the Azure agent, skipping")
		return false, false, nil
	}

	return false, false, nil
}

func (a *Agent) OnServiceAccountTermination(ctx context.Context, serviceAccount *corev1.ServiceAccount) error {
	logger := logrus.WithFields(logrus.Fields{"serviceAccount": serviceAccount.Name, "namespace": serviceAccount.Namespace})

	if serviceAccount.Labels == nil || serviceAccount.Labels[ServiceManagedByAzureAgentLabel] != "true" {
		logger.Debug("ServiceAccount is not managed by the Azure agent, skipping")
		return nil
	}

	return a.deleteUserAssignedIdentity(ctx, serviceAccount.Namespace, serviceAccount.Name)
}
