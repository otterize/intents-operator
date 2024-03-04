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
)

func (a *Agent) OnPodAdmission(ctx context.Context, pod *corev1.Pod, serviceAccount *corev1.ServiceAccount) error {
	roleArn := a.GenerateRoleARN(serviceAccount.Namespace, serviceAccount.Name)
	serviceAccount.Annotations[ServiceAccountAWSRoleARNAnnotation] = roleArn
	pod.Annotations[OtterizeServiceAccountAWSRoleARNAnnotation] = roleArn

	return nil
}

func (a *Agent) ReconcileServiceIAMRole(ctx context.Context, serviceAccount *corev1.ServiceAccount) (updated bool, requeue bool, err error) {
	logger := logrus.WithFields(logrus.Fields{"serviceAccount": serviceAccount.Name, "namespace": serviceAccount.Namespace})

	if _, ok := serviceAccount.Annotations[ServiceAccountAWSRoleARNAnnotation]; ok {
		// validate the existing AWS role matches the annotated ARN
		found, role, err := a.GetOtterizeRole(ctx, serviceAccount.Namespace, serviceAccount.Name)
		if err != nil {
			return false, false, errors.Errorf("failed getting AWS role: %w", err)
		}

		if found {
			generatedRoleARN := *role.Arn
			if generatedRoleARN != serviceAccount.Annotations[ServiceAccountAWSRoleARNAnnotation] {
				logger.WithField("arn", generatedRoleARN).Debug("ServiceAccount AWS role exists, but annotation is misconfigured, should be updated")
				serviceAccount.Annotations[ServiceAccountAWSRoleARNAnnotation] = generatedRoleARN
				return true, false, nil
			}
			logger.WithField("arn", generatedRoleARN).Debug("ServiceAccount has matching AWS role")
			return false, false, nil
		}
	}

	// identity never created or not found, create it

	role, err := a.CreateOtterizeIAMRole(ctx, serviceAccount.Namespace, serviceAccount.Name)
	if err != nil {
		return false, false, errors.Errorf("failed creating AWS role for service account: %w", err)
	}
	logger.WithField("arn", *role.Arn).Info("created AWS role for ServiceAccount")

	serviceAccount.Annotations[ServiceAccountAWSRoleARNAnnotation] = *role.Arn
	return true, false, nil
}

func (a *Agent) DeleteServiceIAMRole(ctx context.Context, namespace string, name string) error {
	return a.DeleteOtterizeIAMRole(ctx, namespace, name)
}
