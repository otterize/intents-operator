package awsagent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/otterize/intents-operator/src/shared/agentutils"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"time"
)

func (a *Agent) AddRolePolicy(ctx context.Context, namespace string, accountName string, intentsServiceName string, statements []StatementEntry) error {
	exists, role, err := a.GetOtterizeRole(ctx, namespace, accountName)

	if err != nil {
		return errors.Wrap(err)
	}

	if !exists {
		// Allow sentinel comparison + dynamic error message
		roleName := a.generateRoleName(namespace, accountName)
		return errors.Errorf("%w: %s", agentutils.ErrRoleNotFound, roleName)
	}

	softDeletionStrategyEnabled := HasSoftDeleteStrategyTagSet(role.Tags)

	policyArn := a.generatePolicyArn(a.generatePolicyName(namespace, intentsServiceName))

	policyOutput, err := a.iamClient.GetPolicy(ctx, &iam.GetPolicyInput{
		PolicyArn: aws.String(policyArn),
	})
	if err != nil {
		if isNoSuchEntityException(err) {
			if len(statements) == 0 {
				// nothing to do
				return nil
			}
			err = a.createPolicy(ctx, role, namespace, intentsServiceName, statements, softDeletionStrategyEnabled)
			return errors.Wrap(err)
		}

		return errors.Wrap(err)
	}

	// policy exists, update it
	policy := policyOutput.Policy

	err = a.updatePolicy(ctx, policy, statements, softDeletionStrategyEnabled)

	if err != nil {
		return errors.Wrap(err)
	}

	err = a.attachPolicy(ctx, role, policy)

	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) DeleteRolePolicyByNamespacedName(ctx context.Context, namespace string, accountName string) error {
	return a.DeleteRolePolicy(ctx, a.generatePolicyName(namespace, accountName))
}

func (a *Agent) DeleteRolePolicy(ctx context.Context, policyName string) error {
	logrus.WithField("policy", policyName).Info("deleting IAM role policy")
	output, err := a.iamClient.GetPolicy(ctx, &iam.GetPolicyInput{
		PolicyArn: aws.String(a.generatePolicyArn(policyName)),
	})

	if err != nil {
		if isNoSuchEntityException(err) {
			return nil
		}

		return errors.Wrap(err)
	}

	if HasSoftDeleteStrategyTagSet(output.Policy.Tags) {
		return a.softDeletePolicy(ctx, policyName)
	}

	policy := output.Policy

	listEntitiesOutput, err := a.iamClient.ListEntitiesForPolicy(ctx, &iam.ListEntitiesForPolicyInput{
		PolicyArn: policy.Arn,
	})

	if err != nil {
		return errors.Wrap(err)
	}

	for _, role := range listEntitiesOutput.PolicyRoles {
		_, err = a.iamClient.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{
			PolicyArn: policy.Arn,
			RoleName:  role.RoleName,
		})
		if isNoSuchEntityException(err) {
			return nil
		}

		if err != nil {
			return errors.Wrap(err)
		}
	}

	listPolicyVersionsOutput, err := a.iamClient.ListPolicyVersions(ctx, &iam.ListPolicyVersionsInput{
		PolicyArn: policy.Arn,
	})

	if err != nil {
		return errors.Wrap(err)
	}

	for _, version := range listPolicyVersionsOutput.Versions {
		// default version is deleted with the policy
		if !version.IsDefaultVersion {
			_, err = a.iamClient.DeletePolicyVersion(ctx, &iam.DeletePolicyVersionInput{
				PolicyArn: policy.Arn,
				VersionId: version.VersionId,
			})

			if err != nil {
				return errors.Wrap(err)
			}
		}
	}

	_, err = a.iamClient.DeletePolicy(ctx, &iam.DeletePolicyInput{
		PolicyArn: policy.Arn,
	})

	if err != nil {
		if isNoSuchEntityException(err) {
			return nil
		}
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) softDeletePolicy(ctx context.Context, policyName string) error {
	output, err := a.iamClient.GetPolicy(ctx, &iam.GetPolicyInput{
		PolicyArn: aws.String(a.generatePolicyArn(policyName)),
	})

	if err != nil {
		return errors.Wrap(err)
	}

	policy := output.Policy
	_, err = a.iamClient.TagPolicy(ctx, &iam.TagPolicyInput{
		PolicyArn: policy.Arn,
		Tags:      []types.Tag{{Key: aws.String(softDeletedTagKey), Value: aws.String(time.Now().String())}},
	})
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) SetRolePolicy(ctx context.Context, namespace, accountName string, statements []StatementEntry) error {
	roleName := a.generateRoleName(namespace, accountName)

	exists, role, err := a.GetOtterizeRole(ctx, namespace, accountName)

	if err != nil {
		return errors.Wrap(err)
	}

	if !exists {
		return errors.Errorf("role not found: %s", roleName)
	}

	policyDoc, _, err := generatePolicyDocument(statements)

	if err != nil {
		return errors.Wrap(err)
	}

	_, err = a.iamClient.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		PolicyDocument: aws.String(policyDoc),
		PolicyName:     role.RoleName,
		RoleName:       role.RoleName,
	})

	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) createPolicy(ctx context.Context, role *types.Role, namespace string, intentsServiceName string, statements []StatementEntry, useSoftDeleteStrategy bool) error {
	fullPolicyName := a.generatePolicyName(namespace, intentsServiceName)
	policyDoc, policyHash, err := generatePolicyDocument(statements)

	if err != nil {
		return errors.Wrap(err)
	}

	tags := []types.Tag{
		{
			Key:   aws.String(policyNameTagKey),
			Value: aws.String(intentsServiceName),
		},
		{
			Key:   aws.String(policyNamespaceTagKey),
			Value: aws.String(namespace),
		},
		{
			Key:   aws.String(policyHashTagKey),
			Value: aws.String(policyHash),
		},
	}
	if useSoftDeleteStrategy {
		tags = append(tags, types.Tag{Key: aws.String(softDeletionStrategyTagKey), Value: aws.String(softDeletionStrategyTagValue)})
	}

	policy, err := a.iamClient.CreatePolicy(ctx, &iam.CreatePolicyInput{
		PolicyDocument: aws.String(policyDoc),
		PolicyName:     aws.String(fullPolicyName),
		Tags:           tags,
	}, AWSBackoffRetryerOptions)

	if err != nil {
		if isEntityAlreadyExistsException(err) {
			return nil
		}
		return errors.Wrap(err)
	}

	err = a.attachPolicy(ctx, role, policy.Policy)

	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) updatePolicy(ctx context.Context, policy *types.Policy, statements []StatementEntry, useSoftDeleteStrategy bool) error {
	policyDoc, policyHash, err := generatePolicyDocument(statements)

	if err != nil {
		return errors.Wrap(err)
	}

	if hasSoftDeletedTagSet(policy.Tags) {
		logrus.Debugf("removing unused tag from policy: %s", *policy.PolicyName)
		_, err = a.iamClient.UntagPolicy(ctx, &iam.UntagPolicyInput{
			PolicyArn: policy.Arn,
			TagKeys:   []string{softDeletedTagKey},
		}, AWSBackoffRetryerOptions)
		if err != nil {
			return errors.Wrap(err)
		}
	}

	if HasSoftDeleteStrategyTagSet(policy.Tags) && !useSoftDeleteStrategy {
		logrus.Debugf("removing soft delete stratergy tag from policy: %s", *policy.PolicyName)
		_, err = a.iamClient.UntagPolicy(ctx, &iam.UntagPolicyInput{
			PolicyArn: policy.Arn,
			TagKeys:   []string{softDeletionStrategyTagKey},
		}, AWSBackoffRetryerOptions)
		if err != nil {
			return errors.Wrap(err)
		}
	}

	if !HasSoftDeleteStrategyTagSet(policy.Tags) && useSoftDeleteStrategy {
		logrus.Debugf("adding soft delete stratergy tag to policy: %s", *policy.PolicyName)
		_, err = a.iamClient.TagPolicy(ctx, &iam.TagPolicyInput{
			PolicyArn: policy.Arn,
			Tags:      []types.Tag{{Key: aws.String(softDeletionStrategyTagKey), Value: aws.String(softDeletionStrategyTagValue)}},
		}, AWSBackoffRetryerOptions)
		if err != nil {
			return errors.Wrap(err)
		}
	}

	existingHashTag, found := lo.Find(policy.Tags, func(item types.Tag) bool {
		return *item.Key == policyHashTagKey
	})

	if found && *existingHashTag.Value == policyHash {
		return nil
	}

	err = a.deleteOldestPolicyVersion(ctx, policy)

	if err != nil {
		return errors.Wrap(err)
	}

	_, err = a.iamClient.CreatePolicyVersion(ctx, &iam.CreatePolicyVersionInput{
		PolicyArn:      policy.Arn,
		PolicyDocument: aws.String(policyDoc),
		SetAsDefault:   true,
	}, AWSBackoffRetryerOptions)

	if err != nil {
		return errors.Wrap(err)
	}

	_, err = a.iamClient.TagPolicy(ctx, &iam.TagPolicyInput{
		PolicyArn: policy.Arn,
		Tags: []types.Tag{
			{
				Key:   aws.String(policyHashTagKey),
				Value: aws.String(policyHash),
			},
		},
	}, AWSBackoffRetryerOptions)

	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) deleteOldestPolicyVersion(ctx context.Context, policy *types.Policy) error {
	output, err := a.iamClient.ListPolicyVersions(ctx, &iam.ListPolicyVersionsInput{
		PolicyArn: policy.Arn,
	})

	if err != nil {
		return errors.Wrap(err)
	}

	if len(output.Versions) < 4 {
		return nil
	}

	versions := output.Versions
	oldest := lo.MinBy(versions, func(a types.PolicyVersion, b types.PolicyVersion) bool {
		return a.CreateDate.Before(*b.CreateDate)
	})

	_, err = a.iamClient.DeletePolicyVersion(ctx, &iam.DeletePolicyVersionInput{
		PolicyArn: policy.Arn,
		VersionId: oldest.VersionId,
	}, AWSBackoffRetryerOptions)

	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) attachPolicy(ctx context.Context, role *types.Role, policy *types.Policy) error {
	_, err := a.iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
		PolicyArn: policy.Arn,
		RoleName:  role.RoleName,
	}, AWSBackoffRetryerOptions)

	return errors.Wrap(err)
}

func generatePolicyDocument(statements []StatementEntry) (string, string, error) {
	policy := PolicyDocument{
		Version:   iamAPIVersion,
		Statement: statements,
	}
	serialized, err := json.Marshal(policy)

	if err != nil {
		return "", "", errors.Wrap(err)
	}

	sum := sha256.Sum256(serialized)

	return string(serialized), fmt.Sprintf("%x", sum), nil
}

func (a *Agent) generatePolicyName(namespace string, intentsServiceName string) string {
	return fmt.Sprintf("otr-%s.%s@%s", namespace, intentsServiceName, a.ClusterName)

}

func (a *Agent) generatePolicyArn(policyName string) string {
	return fmt.Sprintf("arn:aws:iam::%s:policy/%s", a.AccountID, policyName)
}
