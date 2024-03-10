package azureagent

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

const (
	// AzureApplyOnPodLabel is used to mark pods that should be processed by the Azure agent to create an associated Azure identity & role assignment
	AzureApplyOnPodLabel = "credentials-operator.otterize.com/create-azure-workload-identity"

	// ServiceManagedByAzureAgentLabel is used to mark service accounts that are managed by the Azure agent
	ServiceManagedByAzureAgentLabel = "credentials-operator.otterize.com/managed-by-azure-agent"

	// AzureWorkloadIdentityClientIdAnnotation is used by the azure workload identity mechanism to link between service accounts and user assigned identities
	AzureWorkloadIdentityClientIdAnnotation = "azure.workload.identity/client-id"

	// OtterizeAzureWorkloadIdentityClientIdAnnotation is used to annotate the pod with its azure workload identity client ID, to trigger the azure workload identity webhook on it
	OtterizeAzureWorkloadIdentityClientIdAnnotation = "credentials-operator.otterize.com/azure-workload-identity-client-id"

	// AzureClientIDEnvVar is originally set by the Azure workload identity webhook, and is overridden by us to contain the generated client ID
	AzureClientIDEnvVar = "AZURE_CLIENT_ID"
)

func (a *Agent) AppliesOnPod(pod *corev1.Pod) bool {
	return pod.Labels != nil && pod.Labels[AzureApplyOnPodLabel] == "true"
}

func (a *Agent) OnPodAdmission(ctx context.Context, pod *corev1.Pod, serviceAccount *corev1.ServiceAccount) (updated bool, err error) {
	logger := logrus.WithFields(logrus.Fields{"serviceAccount": serviceAccount.Name, "namespace": serviceAccount.Namespace})

	if !a.AppliesOnPod(pod) {
		logger.Debug("Pod is not marked for Azure role assignment, skipping")
		return false, nil
	}

	serviceAccount.Labels[ServiceManagedByAzureAgentLabel] = "true"

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

	// update the pod's containers with the new client ID
	// it would be great if the azure workload identity webhook could do this for us, but it doesn't,
	// because it had already set its client ID to an empty string on its first invocation,
	// before we had a chance to set it on the service account.
	pod.Spec.InitContainers = mutateContainers(pod.Spec.InitContainers, clientId)
	pod.Spec.Containers = mutateContainers(pod.Spec.Containers, clientId)

	return true, nil
}

func mutateContainers(containers []corev1.Container, clientId string) []corev1.Container {
	return lo.Map(containers, func(container corev1.Container, _ int) corev1.Container {
		container.Env = lo.Map(container.Env, func(env corev1.EnvVar, _ int) corev1.EnvVar {
			if env.Name == AzureClientIDEnvVar {
				logrus.WithField("container", container.Name).WithField("envVar", env.Name).WithField("value", clientId).
					Debug("Updating container env var")
				env.Value = clientId
			}
			return env
		})
		return container
	})
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
