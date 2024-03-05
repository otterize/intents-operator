package azureagent

import (
	"context"
	azureerrors "github.com/Azure/azure-sdk-for-go-extensions/pkg/errors"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

const (
	AzureUseWorkloadIdentityLabel           = "azure.workload.identity/use"
	AzureUseWorkloadIdentityValue           = "true"
	AzureWorkloadIdentityClientIdAnnotation = "azure.workload.identity/client-id"
	AzureWorkloadIdentityClientIdNotSet     = "false"
)

func (a *Agent) OnPodAdmission(pod *corev1.Pod, serviceAccount *corev1.ServiceAccount) (updated bool) {
	value, ok := pod.Labels[AzurePodLabel]
	if !ok || value != "true" {
		return false
	}

	serviceAccount.Labels[ServiceManagedByAzureAgentAnnotation] = "true"

	pod.Labels[AzureUseWorkloadIdentityLabel] = AzureUseWorkloadIdentityValue
	serviceAccount.Annotations[AzureWorkloadIdentityClientIdAnnotation] = AzureWorkloadIdentityClientIdNotSet

	return true
}

func (a *Agent) OnServiceAccountUpdate(ctx context.Context, serviceAccount *corev1.ServiceAccount) (updated bool, requeue bool, err error) {
	logger := logrus.WithFields(logrus.Fields{"serviceAccount": serviceAccount.Name, "namespace": serviceAccount.Namespace})

	if serviceAccount.Labels == nil || serviceAccount.Labels[ServiceManagedByAzureAgentAnnotation] != "true" {
		logger.Debug("ServiceAccount is not managed by the Azure agent, skipping")
		return false, false, nil
	}

	if value, ok := serviceAccount.Annotations[AzureWorkloadIdentityClientIdAnnotation]; ok && value != AzureWorkloadIdentityClientIdNotSet {
		// validate that the existing user assigned identity matches the annotated identity
		identity, err := a.findUserAssignedIdentity(ctx, serviceAccount.Namespace, serviceAccount.Name)
		if err != nil && !azureerrors.IsNotFoundErr(err) {
			return false, false, errors.Errorf("failed to find user assigned identity: %w", err)
		}
		found := !azureerrors.IsNotFoundErr(err)

		if found {
			if *identity.Properties.ClientID != serviceAccount.Annotations[AzureWorkloadIdentityClientIdAnnotation] {
				logger.WithField("identity", *identity.Name).Debug("ServiceAccount has a different workload identity client ID, updating")
				serviceAccount.Annotations[AzureWorkloadIdentityClientIdAnnotation] = *identity.Properties.ClientID
			}

			logger.Debug("ServiceAccount already has a workload identity client ID, skipping")
			return false, false, nil
		}
	}

	// identity never created or not found, create it

	identity, err := a.createUserAssignedIdentity(ctx, serviceAccount.Namespace, serviceAccount.Name)
	if err != nil {
		return false, false, errors.Errorf("failed to create user assigned identity: %w", err)
	}
	serviceAccount.Annotations[AzureWorkloadIdentityClientIdAnnotation] = *identity.Properties.ClientID

	return true, false, nil
}

func (a *Agent) OnServiceAccountTermination(ctx context.Context, serviceAccount *corev1.ServiceAccount) error {
	logger := logrus.WithFields(logrus.Fields{"serviceAccount": serviceAccount.Name, "namespace": serviceAccount.Namespace})

	if serviceAccount.Labels == nil || serviceAccount.Labels[ServiceManagedByAzureAgentAnnotation] != "true" {
		logger.Debug("ServiceAccount is not managed by the Azure agent, skipping")
		return nil
	}

	return a.deleteUserAssignedIdentity(ctx, serviceAccount.Namespace, serviceAccount.Name)
}
