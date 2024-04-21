package awscredentialsagent

import (
	"context"
	"fmt"
	awstypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	rolesanywhereTypes "github.com/aws/aws-sdk-go-v2/service/rolesanywhere/types"
	"github.com/otterize/credentials-operator/src/shared/apiutils"
	"github.com/otterize/intents-operator/src/shared/awsagent"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

const (
	// AWSAgentFinalizer indicates that cleanup on AWS is needed upon termination.
	AWSAgentFinalizer = "credentials-operator.otterize.com/aws-role"

	// AWSOtterizeServiceAccountLabel is used to label service accounts managed by the credentials-operator
	AWSOtterizeServiceAccountLabel = "credentials-operator.otterize.com/aws-service-account-managed"

	// ServiceAccountAWSRoleARNAnnotation is used by EKS (Kubernetes at AWS) to link between service accounts
	// and IAM roles
	ServiceAccountAWSRoleARNAnnotation = "eks.amazonaws.com/role-arn"

	// OtterizeServiceAccountAWSRoleARNAnnotation is used to update a Pod in the mutating webhook with the role ARN
	// so that reinvocation is triggered for the EKS pod identity mutating webhook.
	OtterizeServiceAccountAWSRoleARNAnnotation = "credentials-operator.otterize.com/eks-role-arn"

	// OtterizeAWSUseSoftDeleteKey is used to mark workloads that should not have their corresponding roles deleted,
	// but should be tagged as deleted instead (aka soft delete strategy).
	OtterizeAWSUseSoftDeleteKey   = "credentials-operator.otterize.com/aws-use-soft-delete"
	OtterizeAWSUseSoftDeleteValue = "true"
)

type Agent struct {
	agent *awsagent.Agent
}

func NewAWSCredentialsAgent(awsAgent *awsagent.Agent) *Agent {
	return &Agent{awsAgent}
}

func (a *Agent) AppliesOnPod(pod *corev1.Pod) bool {
	return a.agent.AppliesOnPod(pod)
}

func (a *Agent) FinalizerName() string {
	return AWSAgentFinalizer
}

func (a *Agent) ServiceAccountLabel() string {
	return AWSOtterizeServiceAccountLabel
}

func (a *Agent) OnPodAdmission(ctx context.Context, pod *corev1.Pod, serviceAccount *corev1.ServiceAccount, dryRun bool) error {
	logger := logrus.WithFields(logrus.Fields{"serviceAccount": serviceAccount.Name, "namespace": serviceAccount.Namespace})

	roleArn := a.agent.GenerateRoleARN(serviceAccount.Namespace, serviceAccount.Name)
	apiutils.AddAnnotation(serviceAccount, ServiceAccountAWSRoleARNAnnotation, roleArn)
	apiutils.AddAnnotation(serviceAccount, awsagent.ServiceAccountAWSAccountIDAnnotation, a.agent.AccountID)
	apiutils.AddAnnotation(pod, OtterizeServiceAccountAWSRoleARNAnnotation, roleArn)

	podUseSoftDeleteLabelValue, podUseSoftDeleteLabelExists := pod.Labels[OtterizeAWSUseSoftDeleteKey]
	shouldMarkForSoftDelete := podUseSoftDeleteLabelExists && podUseSoftDeleteLabelValue == OtterizeAWSUseSoftDeleteValue
	logger.Debugf("should mark for soft delete: %v, labels: %v", shouldMarkForSoftDelete, pod.Labels)
	if shouldMarkForSoftDelete {
		logger.Debugf("Add soft-delete label to service account")
		apiutils.AddLabel(serviceAccount, OtterizeAWSUseSoftDeleteKey, OtterizeAWSUseSoftDeleteValue)
	} else {
		apiutils.RemoveLabel(serviceAccount, OtterizeAWSUseSoftDeleteKey)
	}

	if a.agent.RolesAnywhereEnabled {
		// In RolesAnywhere mode, the pod webhook, and not the reconciler, handles the role creation
		if pod.Spec.Volumes == nil {
			pod.Spec.Volumes = make([]corev1.Volume, 0)
		}

		_, role, profile, err := a.reconcileAWSRole(ctx, serviceAccount, dryRun)
		if err != nil {
			return errors.Errorf("failed reconciling AWS role: %w", err)
		}

		pod.Spec.Volumes = append(
			pod.Spec.Volumes,
			corev1.Volume{
				Name: "spiffe",
				VolumeSource: corev1.VolumeSource{
					CSI: &corev1.CSIVolumeSource{
						Driver:   "spiffe.csi.cert-manager.io",
						ReadOnly: lo.ToPtr(true),
						VolumeAttributes: map[string]string{
							"aws.spiffe.csi.cert-manager.io/trust-profile": *profile.ProfileArn,
							"aws.spiffe.csi.cert-manager.io/trust-anchor":  a.agent.TrustAnchorArn,
							"aws.spiffe.csi.cert-manager.io/role":          *role.Arn,
							"aws.spiffe.csi.cert-manager.io/enable":        "true",
						},
					},
				},
			},
			corev1.Volume{
				Name: "otterize-aws-credentials",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		)

		const credentialProcessPath = "/otterize/aws/rolesanywhere-credential-helper"
		configFile := buildAWSConfigFile(
			credentialProcessPath,
			"/spiffe/tls.crt",
			"/spiffe/tls.key",
			*profile.ProfileArn,
			*role.Arn,
			a.agent.TrustAnchorArn,
		)

		pod.Spec.InitContainers = append(
			pod.Spec.InitContainers,
			corev1.Container{
				Name:  "otterize-aws-roles-anywhere-credential-loader",
				Image: "otterize/rolesanywhere-credential-helper:latest",
				Command: []string{
					"sh",
					"-c",
					"cp /rolesanywhere-credential-helper /mnt/rolesanywhere-credential-helper && echo '" + configFile + "' > /mnt/config",
				},
				VolumeMounts: []corev1.VolumeMount{
					corev1.VolumeMount{
						Name:      "otterize-aws-credentials",
						MountPath: "/mnt",
					},
				},
			},
		)

		for i := range pod.Spec.Containers {
			pod.Spec.Containers[i].VolumeMounts = append(
				pod.Spec.Containers[i].VolumeMounts,
				corev1.VolumeMount{
					Name:      "spiffe",
					MountPath: "/spiffe",
					ReadOnly:  true,
				},
				corev1.VolumeMount{
					Name:      "otterize-aws-credentials",
					MountPath: "/otterize/aws",
					ReadOnly:  true,
				},
			)
			pod.Spec.Containers[i].Env = append(
				pod.Spec.Containers[i].Env,
				corev1.EnvVar{
					Name:  "AWS_CONFIG_FILE",
					Value: "/otterize/aws/config",
				},
			)
		}
	}

	return nil
}

func (a *Agent) reconcileAWSRole(ctx context.Context, serviceAccount *corev1.ServiceAccount, dryRun bool) (updateAnnotation bool, role *awstypes.Role, profile *rolesanywhereTypes.ProfileDetail, err error) {
	logger := logrus.WithFields(logrus.Fields{"serviceAccount": serviceAccount.Name, "namespace": serviceAccount.Namespace})
	if dryRun {
		return false, &awstypes.Role{
				Arn: lo.ToPtr("dry-run-role-arn"),
			}, &rolesanywhereTypes.ProfileDetail{
				ProfileArn: lo.ToPtr("dry-run-profile-arn"),
			}, nil
	}

	if roleARN, ok := hasAWSAnnotation(serviceAccount); ok {
		generatedRoleARN := a.agent.GenerateRoleARN(serviceAccount.Namespace, serviceAccount.Name)
		found, role, err := a.agent.GetOtterizeRole(ctx, serviceAccount.Namespace, serviceAccount.Name)

		if err != nil {
			return false, nil, nil, errors.Errorf("failed getting AWS role: %w", err)
		}

		foundProfile, profile, err := a.agent.GetOtterizeProfile(ctx, serviceAccount.Namespace, serviceAccount.Name)
		if err != nil {
			return false, nil, nil, errors.Errorf("failed getting AWS profile: %w", err)
		}

		if found && foundProfile {
			if generatedRoleARN != roleARN {
				logger.WithField("arn", *role.Arn).Debug("ServiceAccount AWS role exists, but annotation is misconfigured, should be updated")
				return true, role, profile, nil
			}
			logger.WithField("arn", *role.Arn).Debug("ServiceAccount has matching AWS role")

			return false, role, profile, nil
		}
	}

	role, err = a.agent.CreateOtterizeIAMRole(ctx, serviceAccount.Namespace, serviceAccount.Name, a.shouldUseSoftDeleteStrategy(serviceAccount))
	if err != nil {
		return false, nil, nil, errors.Errorf("failed creating AWS role for service account: %w", err)
	}
	logger.WithField("arn", *role.Arn).Info("created AWS role for ServiceAccount")

	profile, err = a.agent.CreateRolesAnywhereProfileForRole(ctx, *role, serviceAccount.Namespace, serviceAccount.Name)
	if err != nil {
		return false, nil, nil, errors.Errorf("failed creating rolesanywhere profile for role: %w", err)
	}
	logger.WithField("arn", *profile.ProfileId).Info("created AWS profile for ServiceAccount")

	return true, role, profile, nil
}

func (a *Agent) OnPodUpdate(ctx context.Context, pod *corev1.Pod, serviceAccount *corev1.ServiceAccount) (updated bool, requeue bool, err error) {
	return false, false, nil
}

func (a *Agent) OnServiceAccountUpdate(ctx context.Context, serviceAccount *corev1.ServiceAccount) (updated bool, requeue bool, err error) {
	logger := logrus.WithFields(logrus.Fields{"serviceAccount": serviceAccount.Name, "namespace": serviceAccount.Namespace})

	roleARN, ok := hasAWSAnnotation(serviceAccount)

	if a.agent.RolesAnywhereEnabled {
		// In RolesAnywhere mode, the SPIFFE pod webhook, and not the reconciler, handles the role creation
		return false, false, nil
	}

	role, err := a.agent.CreateOtterizeIAMRole(ctx, serviceAccount.Namespace, serviceAccount.Name, a.shouldUseSoftDeleteStrategy(serviceAccount))

	if err != nil {
		return false, false, fmt.Errorf("failed creating AWS role for service account: %w", err)
	}
	logger.WithField("arn", *role.Arn).Info("created AWS role for ServiceAccount")

	// update annotation if it doesn't exist or if it is misconfigured
	shouldUpdateAnnotation := !ok || roleARN != *role.Arn
	if !shouldUpdateAnnotation {
		return false, false, nil
	}

	apiutils.AddAnnotation(serviceAccount, ServiceAccountAWSRoleARNAnnotation, *role.Arn)
	return true, false, nil
}

func (a *Agent) shouldUseSoftDeleteStrategy(serviceAccount *corev1.ServiceAccount) bool {
	if a.agent.MarkRolesAsUnusedInsteadOfDelete {
		return true
	}
	if serviceAccount.Labels == nil {
		return false
	}

	softDeleteValue, shouldSoftDelete := serviceAccount.Labels[OtterizeAWSUseSoftDeleteKey]
	return shouldSoftDelete && softDeleteValue == OtterizeAWSUseSoftDeleteValue
}

func hasAWSAnnotation(serviceAccount *corev1.ServiceAccount) (string, bool) {
	if serviceAccount.Annotations == nil {
		return "", false
	}

	roleARN, ok := serviceAccount.Annotations[ServiceAccountAWSRoleARNAnnotation]
	return roleARN, ok
}

func (a *Agent) OnServiceAccountTermination(ctx context.Context, serviceAccount *corev1.ServiceAccount) error {
	logger := logrus.WithFields(logrus.Fields{"serviceAccount": serviceAccount.Name, "namespace": serviceAccount.Namespace})

	err := a.agent.DeleteOtterizeIAMRole(ctx, serviceAccount.Namespace, serviceAccount.Name)
	if err != nil {
		return fmt.Errorf("failed to remove service account: %w", err)
	}

	if a.agent.RolesAnywhereEnabled {
		deleted, err := a.agent.DeleteRolesAnywhereProfileForServiceAccount(ctx, serviceAccount.Namespace, serviceAccount.Name)
		if err != nil {
			return fmt.Errorf("failed to remove rolesanywhere profile for service account: %w", err)
		}

		if !deleted {
			logger.Debug("rolesanywhere profile for service account did not exist when deletion was attempted")
		}

		logger.Debug("deleted rolesanywhere profile for service account")
	}

	return nil
}

func buildAWSConfigFile(
	credentialProcessPath string,
	certificatePath string,
	privateKeyPath string,
	profileARN string,
	roleARN string,
	trustAnchorARN string,
) string {
	return fmt.Sprintf(
		`[default]
		credential_process = %s credential-process --certificate %s --private-key %s --profile-arn %s --role-arn %s --trust-anchor-arn %s`,
		credentialProcessPath,
		certificatePath,
		privateKeyPath,
		profileARN,
		roleARN,
		trustAnchorARN,
	)
}
