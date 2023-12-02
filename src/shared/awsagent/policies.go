package awsagent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
)

func (a *Agent) AddRolePolicy(ctx context.Context, namespace string, accountName string, policyName string, statements []StatementEntry) error {
	exists, role, err := a.GetOtterizeRole(ctx, namespace, accountName)

	if err != nil {
		return err
	}

	if !exists {
		return fmt.Errorf("role not found: %s", a.generateRoleName(namespace, accountName))
	}

	policyArn := a.generatePolicyArn(namespace, policyName)

	policyOutput, err := a.iamClient.GetPolicy(ctx, &iam.GetPolicyInput{
		PolicyArn: aws.String(policyArn),
	})
	if err != nil {
		if isNoSuchEntityException(err) {
			_, err := a.createPolicy(ctx, role, namespace, policyName, statements)

			return err
		}

		return err
	}

	// policy exists, update it
	policy := policyOutput.Policy

	err = a.updatePolicy(ctx, policy, statements)

	if err != nil {
		return err
	}

	err = a.attachPolicy(ctx, role, policy)

	if err != nil {
		return err
	}

	return nil
}

func (a *Agent) DeleteRolePolicy(ctx context.Context, namespace, policyName string) error {
	output, err := a.iamClient.GetPolicy(ctx, &iam.GetPolicyInput{
		PolicyArn: aws.String(a.generatePolicyArn(namespace, policyName)),
	})

	if err != nil {
		if isNoSuchEntityException(err) {
			return nil
		}

		return err
	}

	policy := output.Policy

	listEntitiesOutput, err := a.iamClient.ListEntitiesForPolicy(ctx, &iam.ListEntitiesForPolicyInput{
		PolicyArn: policy.Arn,
	})

	if err != nil {
		return err
	}

	for _, role := range listEntitiesOutput.PolicyRoles {
		_, err = a.iamClient.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{
			PolicyArn: policy.Arn,
			RoleName:  role.RoleName,
		})

		if err != nil {
			return err
		}
	}

	listPolicyVersionsOutput, err := a.iamClient.ListPolicyVersions(ctx, &iam.ListPolicyVersionsInput{
		PolicyArn: policy.Arn,
	})

	if err != nil {
		return err
	}

	for _, version := range listPolicyVersionsOutput.Versions {
		// default version is deleted with the policy
		if !version.IsDefaultVersion {
			_, err = a.iamClient.DeletePolicyVersion(ctx, &iam.DeletePolicyVersionInput{
				PolicyArn: policy.Arn,
				VersionId: version.VersionId,
			})

			if err != nil {
				return err
			}
		}
	}

	_, err = a.iamClient.DeletePolicy(ctx, &iam.DeletePolicyInput{
		PolicyArn: policy.Arn,
	})

	if err != nil {
		return err
	}

	return nil
}

func (a *Agent) SetRolePolicy(ctx context.Context, namespace, accountName string, statements []StatementEntry) error {
	logger := logrus.WithField("account", accountName).WithField("namespace", namespace)
	roleName := a.generateRoleName(namespace, accountName)

	exists, role, err := a.GetOtterizeRole(ctx, namespace, accountName)

	if err != nil {
		return err
	}

	if !exists {
		errorMessage := fmt.Sprintf("role not found: %s", roleName)
		logger.Error(errorMessage)
		return errors.New(errorMessage)
	}

	policyDoc, _, err := generatePolicyDocument(statements)

	if err != nil {
		logger.WithError(err).Errorf("failed to generate policy document")
		return err
	}

	_, err = a.iamClient.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		PolicyDocument: aws.String(policyDoc),
		PolicyName:     role.RoleName,
		RoleName:       role.RoleName,
	})

	if err != nil {
		return err
	}

	return nil
}

func (a *Agent) createPolicy(ctx context.Context, role *types.Role, namespace, policyName string, statements []StatementEntry) (*types.Policy, error) {
	fullPolicyName := generatePolicyName(namespace, policyName)
	policyDoc, policyHash, err := generatePolicyDocument(statements)

	if err != nil {
		return nil, err
	}

	policy, err := a.iamClient.CreatePolicy(ctx, &iam.CreatePolicyInput{
		PolicyDocument: aws.String(policyDoc),
		PolicyName:     aws.String(fullPolicyName),
		Tags: []types.Tag{
			{
				Key:   aws.String(policyNameTagKey),
				Value: aws.String(policyName),
			},
			{
				Key:   aws.String(policyNamespaceTagKey),
				Value: aws.String(namespace),
			},
			{
				Key:   aws.String(policyHashTagKey),
				Value: aws.String(policyHash),
			},
		},
	})

	if err != nil {
		return nil, err
	}

	err = a.attachPolicy(ctx, role, policy.Policy)

	if err != nil {
		return nil, err
	}

	return policy.Policy, nil
}

func (a *Agent) updatePolicy(ctx context.Context, policy *types.Policy, statements []StatementEntry) error {
	policyDoc, policyHash, err := generatePolicyDocument(statements)

	if err != nil {
		return err
	}

	existingHashTag, found := lo.Find(policy.Tags, func(item types.Tag) bool {
		return *item.Key == policyHashTagKey
	})

	if found && *existingHashTag.Value == policyHash {
		return nil
	}

	err = a.deleteOldestPolicyVersion(ctx, policy)

	if err != nil {
		return err
	}

	_, err = a.iamClient.CreatePolicyVersion(ctx, &iam.CreatePolicyVersionInput{
		PolicyArn:      policy.Arn,
		PolicyDocument: aws.String(policyDoc),
		SetAsDefault:   true,
	})

	if err != nil {
		return err
	}

	_, err = a.iamClient.TagPolicy(ctx, &iam.TagPolicyInput{
		PolicyArn: policy.Arn,
		Tags: []types.Tag{
			{
				Key:   aws.String(policyHashTagKey),
				Value: aws.String(policyHash),
			},
		},
	})

	if err != nil {
		return err
	}

	return nil
}

func (a *Agent) deleteOldestPolicyVersion(ctx context.Context, policy *types.Policy) error {
	output, err := a.iamClient.ListPolicyVersions(ctx, &iam.ListPolicyVersionsInput{
		PolicyArn: policy.Arn,
	})

	if err != nil {
		return err
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
	})

	if err != nil {
		return err
	}

	return nil
}

func (a *Agent) attachPolicy(ctx context.Context, role *types.Role, policy *types.Policy) error {
	_, err := a.iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
		PolicyArn: policy.Arn,
		RoleName:  role.RoleName,
	})

	return err
}

func generatePolicyDocument(statements []StatementEntry) (string, string, error) {
	policy := PolicyDocument{
		Version:   iamAPIVersion,
		Statement: statements,
	}
	serialized, err := json.Marshal(policy)

	if err != nil {
		return "", "", err
	}

	sum := sha256.Sum256(serialized)

	return string(serialized), fmt.Sprintf("%x", sum), nil
}

func generatePolicyName(ns, policyName string) string {
	return fmt.Sprintf("otterize-policy-%s-%s", ns, policyName)

}

func (a *Agent) generatePolicyArn(ns, policyName string) string {
	return fmt.Sprintf("arn:aws:iam::%s:policy/%s", a.accountID, generatePolicyName(ns, policyName))
}
