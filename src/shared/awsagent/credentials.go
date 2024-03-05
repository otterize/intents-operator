package awsagent

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

const (
	// ServiceAccountAWSRoleARNAnnotation is used by EKS (Kubernetes at AWS) to link between service accounts
	// and IAM roles
	ServiceAccountAWSRoleARNAnnotation = "eks.amazonaws.com/role-arn"

	// OtterizeServiceAccountAWSRoleARNAnnotation is used to update a Pod in the mutating webhook with the role ARN
	// so that reinvocation is triggered for the EKS pod identity mutating webhook.
	OtterizeServiceAccountAWSRoleARNAnnotation = "credentials-operator.otterize.com/eks-role-arn"

	// OtterizeAWSUseSoftDeleteKey is used to mark workloads that should not have their corresponding roles deleted,
	// but should be tagged as deleted instead (aka soft delete strategy).
	OtterizeAWSUseSoftDeleteKey = "credentials-operator.otterize.com/aws-use-soft-delete"
)

func (a *Agent) OnServiceAccountUpdate(ctx context.Context, serviceAccount *corev1.ServiceAccount) (updated bool, requeue bool, err error) {
	logger := logrus.WithFields(logrus.Fields{"serviceAccount": serviceAccount.Name, "namespace": serviceAccount.Namespace})

	if serviceAccount.Labels == nil || serviceAccount.Labels[ServiceManagedByAWSAgentAnnotation] != "true" {
		logger.Debug("ServiceAccount is not managed by the AWS agent, skipping")
		return false, false, nil
	}

	useSoftDeleteStrategy := a.shouldUseSoftDeleteStrategy(serviceAccount)

	// calling create in any case because this way we validate it is not soft-deleted and it is configured with the correct soft-delete strategy
	role, err := a.CreateOtterizeIAMRole(ctx, serviceAccount.Namespace, serviceAccount.Name, useSoftDeleteStrategy)
	if err != nil {
		return false, false, errors.Errorf("failed creating AWS role for service account: %w", err)
	}
	logger.WithField("arn", *role.Arn).Info("created AWS role for ServiceAccount")

	roleARN, ok := serviceAccount.Annotations[ServiceAccountAWSRoleARNAnnotation]

	// update annotation if it doesn't exist or if it is misconfigured
	shouldUpdate := !ok || roleARN != *role.Arn

	serviceAccount.Annotations[ServiceAccountAWSRoleARNAnnotation] = *role.Arn
	return shouldUpdate, false, nil
}

func (a *Agent) shouldUseSoftDeleteStrategy(serviceAccount *corev1.ServiceAccount) bool {
	if a.markRolesAsUnusedInsteadOfDelete {
		return true
	}
	if serviceAccount.Labels == nil {
		return false
	}

	softDeleteValue, shouldSoftDelete := serviceAccount.Labels[OtterizeAWSUseSoftDeleteKey]
	return shouldSoftDelete && softDeleteValue == "true"
}

func (a *Agent) OnPodAdmission(pod *corev1.Pod, serviceAccount *corev1.ServiceAccount) (updated bool) {
	value, ok := pod.Labels[AWSPodLabel]
	if !ok || value != "true" {
		return false
	}

	serviceAccount.Labels[ServiceManagedByAWSAgentAnnotation] = "true"

	roleArn := a.GenerateRoleARN(serviceAccount.Namespace, serviceAccount.Name)
	serviceAccount.Annotations[ServiceAccountAWSRoleARNAnnotation] = roleArn
	pod.Annotations[OtterizeServiceAccountAWSRoleARNAnnotation] = roleArn

	podUseSoftDeleteLabelValue, podUseSoftDeleteLabelExists := pod.Labels[OtterizeAWSUseSoftDeleteKey]
	shouldMarkForSoftDelete := podUseSoftDeleteLabelExists && podUseSoftDeleteLabelValue == "true"
	if shouldMarkForSoftDelete {
		serviceAccount.Labels[OtterizeAWSUseSoftDeleteKey] = "true"
	} else {
		delete(serviceAccount.Labels, OtterizeAWSUseSoftDeleteKey)
	}

	return true
}

func (a *Agent) OnServiceAccountTermination(ctx context.Context, serviceAccount *corev1.ServiceAccount) error {
	logger := logrus.WithFields(logrus.Fields{"serviceAccount": serviceAccount.Name, "namespace": serviceAccount.Namespace})

	if serviceAccount.Labels == nil || serviceAccount.Labels[ServiceManagedByAWSAgentAnnotation] != "true" {
		logger.Debug("ServiceAccount is not managed by the Azure agent, skipping")
		return nil
	}

	return a.DeleteOtterizeIAMRole(ctx, serviceAccount.Namespace, serviceAccount.Name)
}
