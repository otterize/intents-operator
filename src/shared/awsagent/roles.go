package awsagent

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/smithy-go"
	"github.com/otterize/intents-operator/src/shared/agentutils"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"strings"
	"time"
)
import "github.com/aws/aws-sdk-go-v2/service/iam"

// GetOtterizeRole gets the matching IAM role for given service
// Return value is:
// - found (bool), true if role exists, false if not
// - role (types.Role), aws role
// - error
func (a *Agent) GetOtterizeRole(ctx context.Context, namespaceName, accountName string) (bool, *types.Role, error) {
	logger := logrus.WithField("namespace", namespaceName).WithField("account", accountName)
	roleName := a.generateRoleName(namespaceName, accountName)

	role, err := a.iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	})

	if err != nil {
		var apiError smithy.APIError
		if errors.As(err, &apiError) {
			var noSuchEntityException *types.NoSuchEntityException
			switch {
			case errors.As(apiError, &noSuchEntityException):
				logger.WithError(err).Debug("role not found")
				return false, nil, nil
			default:
				return false, nil, errors.Wrap(err)
			}
		}
	}

	return true, role.Role, nil
}

// CreateOtterizeIAMRole creates a new IAM role for service, if one doesn't exist yet
func (a *Agent) CreateOtterizeIAMRole(ctx context.Context, namespaceName, accountName string, useSoftDeleteStrategy bool) (*types.Role, error) {
	logger := logrus.WithField("namespace", namespaceName).WithField("account", accountName)
	exists, role, err := a.GetOtterizeRole(ctx, namespaceName, accountName)

	if err != nil {
		return nil, errors.Wrap(err)
	}

	if exists {
		logger.Debugf("found existing role, arn: %s", *role.Arn)
		// check if it is soft deleted - if so remove soft deleted tag
		if hasSoftDeletedTagSet(role.Tags) {
			logger.Debug("role is tagged unused, untagging")
			// There is no need to untag the role's policies from this context, It will happen if the policy will be created again
			_, err := a.iamClient.UntagRole(ctx, &iam.UntagRoleInput{RoleName: role.RoleName, TagKeys: []string{softDeletedTagKey}})
			if err != nil {
				return nil, errors.Wrap(err)
			}
			// Intentionally not returning here, as we want to continue and check if we need to mark as soft delete only
		}
		if useSoftDeleteStrategy {
			err = a.setRoleSoftDeleteStrategyTag(ctx, role)
			if err != nil {
				return nil, errors.Wrap(err)
			}
		}
		if !useSoftDeleteStrategy && HasSoftDeleteStrategyTagSet(role.Tags) {
			err = a.UnsetRoleSoftDeleteStrategyTag(ctx, role)
			if err != nil {
				return nil, errors.Wrap(err)
			}
		}

		return role, nil
	}

	trustPolicy, err := a.generateTrustPolicy(namespaceName, accountName)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	tags := []types.Tag{
		{
			Key:   aws.String(serviceAccountNameTagKey),
			Value: aws.String(accountName),
		},
		{
			Key:   aws.String(serviceAccountNamespaceTagKey),
			Value: aws.String(namespaceName),
		},
		{
			Key:   aws.String(clusterNameTagKey),
			Value: aws.String(a.clusterName),
		},
	}
	if useSoftDeleteStrategy {
		tags = append(tags, types.Tag{Key: aws.String(softDeletionStrategyTagKey), Value: aws.String(softDeletionStrategyTagValue)})
	}
	createRoleOutput, createRoleError := a.iamClient.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(a.generateRoleName(namespaceName, accountName)),
		AssumeRolePolicyDocument: aws.String(trustPolicy),
		Tags:                     tags,
		Description:              aws.String(iamRoleDescription),
		PermissionsBoundary:      aws.String(fmt.Sprintf("arn:aws:iam::%s:policy/%s-limit-iam-permission-boundary", a.accountID, a.clusterName)),
	})

	if createRoleError != nil {
		return nil, errors.Wrap(createRoleError)
	}

	logger.Debugf("created new role, arn: %s", *createRoleOutput.Role.Arn)
	return createRoleOutput.Role, nil

}

func (a *Agent) DeleteOtterizeIAMRole(ctx context.Context, namespaceName, accountName string) error {
	logger := logrus.WithField("namespace", namespaceName).WithField("account", accountName)
	exists, role, err := a.GetOtterizeRole(ctx, namespaceName, accountName)

	if err != nil {
		return errors.Wrap(err)
	}

	if !exists {
		logger.Info("role does not exist, not doing anything")
		return nil
	}

	tags := lo.SliceToMap(role.Tags, func(item types.Tag) (string, string) {
		return *item.Key, *item.Value
	})

	if taggedServiceAccountName, ok := tags[serviceAccountNameTagKey]; !ok || taggedServiceAccountName != accountName {
		return errors.Errorf("attempted to delete role with incorrect service account name: expected '%s' but got '%s'", accountName, taggedServiceAccountName)
	}
	if taggedNamespace, ok := tags[serviceAccountNamespaceTagKey]; !ok || taggedNamespace != namespaceName {
		return errors.Errorf("attempted to delete role with incorrect namespace: expected '%s' but got '%s'", namespaceName, taggedNamespace)
	}
	if taggedClusterName, ok := tags[clusterNameTagKey]; !ok || taggedClusterName != a.clusterName {
		return errors.Errorf("attempted to delete role with incorrect cluster name: expected '%s' but got '%s'", a.clusterName, taggedClusterName)
	}

	if HasSoftDeleteStrategyTagSet(role.Tags) {
		logger.Debug("role has softDeleteStrategy tag, tagging as unused")
		return a.SoftDeleteOtterizeIAMRole(ctx, role)
	}

	err = a.deleteAllRolePolicies(ctx, role)

	if err != nil {
		return errors.Wrap(err)
	}

	_, err = a.iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{
		RoleName: role.RoleName,
	})

	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) SoftDeleteOtterizeIAMRole(ctx context.Context, role *types.Role) error {
	err := a.SoftDeleteAllRolePolicies(ctx, role)
	if err != nil {
		return errors.Wrap(err)
	}

	_, err = a.iamClient.TagRole(ctx, &iam.TagRoleInput{
		RoleName: role.RoleName,
		Tags:     []types.Tag{{Key: aws.String(softDeletedTagKey), Value: aws.String(time.Now().String())}},
	})

	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

// deleteAllRolePolicies deletes the inline role policy, if exists, in preparation to delete the role.
func (a *Agent) deleteAllRolePolicies(ctx context.Context, role *types.Role) error {
	listOutput, err := a.iamClient.ListAttachedRolePolicies(ctx, &iam.ListAttachedRolePoliciesInput{RoleName: role.RoleName})

	if err != nil {
		return errors.Errorf("failed to list role attached policies: %w", err)
	}

	for _, policy := range listOutput.AttachedPolicies {
		if strings.HasPrefix(*policy.PolicyName, "otterize-") || strings.HasPrefix(*policy.PolicyName, "otr-") {
			err = a.DeleteRolePolicy(ctx, *policy.PolicyName)
			if err != nil {
				return errors.Errorf("failed to delete policy: %w", err)
			}
		}
	}

	return nil
}

// SoftDeleteAllRolePolicies marks all inline role policies as deleted.
func (a *Agent) SoftDeleteAllRolePolicies(ctx context.Context, role *types.Role) error {
	listOutput, err := a.iamClient.ListAttachedRolePolicies(ctx, &iam.ListAttachedRolePoliciesInput{RoleName: role.RoleName})

	if err != nil {
		return errors.Errorf("failed to list role attached policies: %w", err)
	}

	for _, policy := range listOutput.AttachedPolicies {
		if strings.HasPrefix(*policy.PolicyName, "otterize-") || strings.HasPrefix(*policy.PolicyName, "otr-") {
			err = a.softDeletePolicy(ctx, *policy.PolicyName)
			if err != nil {
				return errors.Errorf("failed to delete policy: %w", err)
			}
		}
	}

	return nil
}

// setRoleSoftDeleteStrategyTag sets the soft delete strategy tag on the role
func (a *Agent) setRoleSoftDeleteStrategyTag(ctx context.Context, role *types.Role) error {
	err := a.setAllRolePoliciesSoftDeleteStrategyTag(ctx, role)
	if err != nil {
		return errors.Wrap(err)
	}
	_, err = a.iamClient.TagRole(ctx, &iam.TagRoleInput{RoleName: role.RoleName, Tags: []types.Tag{{Key: aws.String(softDeletionStrategyTagKey), Value: aws.String(softDeletionStrategyTagValue)}}})
	return errors.Wrap(err)
}

// setAllRolePoliciesSoftDeleteStrategyTag sets the soft delete strategy tag on all inline role policies
func (a *Agent) setAllRolePoliciesSoftDeleteStrategyTag(ctx context.Context, role *types.Role) error {
	listOutput, err := a.iamClient.ListAttachedRolePolicies(ctx, &iam.ListAttachedRolePoliciesInput{RoleName: role.RoleName})

	if err != nil {
		return errors.Errorf("failed to list role attached policies: %w", err)
	}

	for _, policy := range listOutput.AttachedPolicies {
		if strings.HasPrefix(*policy.PolicyName, "otterize-") || strings.HasPrefix(*policy.PolicyName, "otr-") {
			_, err = a.iamClient.TagPolicy(ctx, &iam.TagPolicyInput{PolicyArn: policy.PolicyArn, Tags: []types.Tag{{Key: aws.String(softDeletionStrategyTagKey), Value: aws.String(softDeletionStrategyTagValue)}}})
			if err != nil {
				return errors.Errorf("failed to set policy soft delete strategy tag: %w", err)
			}
		}
	}

	return nil
}

// UnsetRoleSoftDeleteStrategyTag removes the soft delete strategy tag from the role
func (a *Agent) UnsetRoleSoftDeleteStrategyTag(ctx context.Context, role *types.Role) error {
	err := a.unsetAllRolePoliciesSoftDeleteStrategyTag(ctx, role)
	if err != nil {
		return errors.Wrap(err)
	}
	_, err = a.iamClient.UntagRole(ctx, &iam.UntagRoleInput{RoleName: role.RoleName, TagKeys: []string{softDeletionStrategyTagKey}})
	return errors.Wrap(err)
}

// unsetAllRolePoliciesSoftDeleteStrategyTag removes the soft delete strategy tag from all inline role policies
func (a *Agent) unsetAllRolePoliciesSoftDeleteStrategyTag(ctx context.Context, role *types.Role) error {
	listOutput, err := a.iamClient.ListAttachedRolePolicies(ctx, &iam.ListAttachedRolePoliciesInput{RoleName: role.RoleName})

	if err != nil {
		return errors.Errorf("failed to list role attached policies: %w", err)
	}

	for _, policy := range listOutput.AttachedPolicies {
		if strings.HasPrefix(*policy.PolicyName, "otterize-") || strings.HasPrefix(*policy.PolicyName, "otr-") {
			_, err = a.iamClient.UntagPolicy(ctx, &iam.UntagPolicyInput{PolicyArn: policy.PolicyArn, TagKeys: []string{softDeletionStrategyTagKey}})
			if err != nil {
				return errors.Errorf("failed to unset policy soft delete strategy tag: %w", err)
			}
		}
	}

	return nil
}

func (a *Agent) generateTrustPolicy(namespaceName, accountName string) (string, error) {
	oidc := strings.TrimPrefix(a.oidcURL, "https://")

	policy := PolicyDocument{
		Version: iamAPIVersion,
		Statement: []StatementEntry{
			{
				Effect: iamEffectAllow,
				Action: []string{"sts:AssumeRoleWithWebIdentity"},
				Principal: map[string]string{
					"Federated": fmt.Sprintf("arn:aws:iam::%s:oidc-provider/%s", a.accountID, a.oidcURL),
				},
				Condition: map[string]any{
					"StringEquals": map[string]string{
						fmt.Sprintf("%s:sub", oidc): fmt.Sprintf("system:serviceaccount:%s:%s", namespaceName, accountName),
						fmt.Sprintf("%s:aud", oidc): "sts.amazonaws.com",
					},
				},
			},
		},
	}

	serialized, err := json.Marshal(policy)

	if err != nil {
		logrus.WithError(err).Error("failed to create trust policy")
		return "", errors.Wrap(err)
	}

	return string(serialized), errors.Wrap(err)
}

func (a *Agent) generateRoleName(namespace string, accountName string) string {
	fullName := fmt.Sprintf("otr-%s.%s@%s", namespace, accountName, a.clusterName)
	return agentutils.TruncateHashName(fullName, maxAWSNameLength)
}

func (a *Agent) GenerateRoleARN(namespace string, accountName string) string {
	roleName := a.generateRoleName(namespace, accountName)
	return fmt.Sprintf("arn:aws:iam::%s:role/%s", a.accountID, roleName)
}
