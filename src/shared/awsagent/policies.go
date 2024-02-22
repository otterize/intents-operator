package awsagent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
)

func (a *Agent) IntentType() otterizev1alpha3.IntentType {
	return otterizev1alpha3.IntentTypeAWS
}

func (a *Agent) ApplyOnPodLabel() string {
	return "credentials-operator.otterize.com/create-aws-role"
}

func (a *Agent) createPolicyFromIntents(intents []otterizev1alpha3.Intent) PolicyDocument {
	policy := PolicyDocument{
		Version: "2012-10-17",
	}

	for _, intent := range intents {
		awsResource := intent.Name
		actions := intent.AWSActions

		policy.Statement = append(policy.Statement, StatementEntry{
			Effect:   "Allow",
			Resource: awsResource,
			Action:   actions,
		})
	}

	return policy
}

func (a *Agent) AddRolePolicyFromIntents(ctx context.Context, namespace string, accountName string, intentsServiceName string, intents []otterizev1alpha3.Intent) error {
	policyDoc := a.createPolicyFromIntents(intents)
	return a.AddRolePolicy(ctx, namespace, accountName, intentsServiceName, policyDoc.Statement)
}

func (a *Agent) AddRolePolicy(ctx context.Context, namespace string, accountName string, intentsServiceName string, statements []StatementEntry) error {
	exists, role, err := a.GetOtterizeRole(ctx, namespace, accountName)

	if err != nil {
		return errors.Wrap(err)
	}

	if !exists {
		return errors.Errorf("role not found: %s", a.generateRoleName(namespace, accountName))
	}

	policyArn := a.generatePolicyArn(a.generatePolicyName(namespace, intentsServiceName))

	policyOutput, err := a.iamClient.GetPolicy(ctx, &iam.GetPolicyInput{
		PolicyArn: aws.String(policyArn),
	})
	if err != nil {
		if isNoSuchEntityException(err) {
			_, err := a.createPolicy(ctx, role, namespace, intentsServiceName, statements)

			return errors.Wrap(err)
		}

		return errors.Wrap(err)
	}

	// policy exists, update it
	policy := policyOutput.Policy

	err = a.updatePolicy(ctx, policy, statements)

	if err != nil {
		return errors.Wrap(err)
	}

	err = a.attachPolicy(ctx, role, policy)

	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) DeletePolicyFromIntents(ctx context.Context, intents v1alpha3.ClientIntents) error {
	return a.DeleteRolePolicyFromIntents(ctx, intents)
}

func (a *Agent) DeleteRolePolicyFromIntents(ctx context.Context, intents v1alpha3.ClientIntents) error {
	return a.DeleteRolePolicy(ctx, a.generatePolicyName(intents.Namespace, intents.Spec.Service.Name))
}

func (a *Agent) DeleteRolePolicy(ctx context.Context, policyName string) error {
	output, err := a.iamClient.GetPolicy(ctx, &iam.GetPolicyInput{
		PolicyArn: aws.String(a.generatePolicyArn(policyName)),
	})

	if err != nil {
		if isNoSuchEntityException(err) {
			return nil
		}

		return errors.Wrap(err)
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
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) SetRolePolicy(ctx context.Context, namespace, accountName string, statements []StatementEntry) error {
	logger := logrus.WithField("account", accountName).WithField("namespace", namespace)
	roleName := a.generateRoleName(namespace, accountName)

	exists, role, err := a.GetOtterizeRole(ctx, namespace, accountName)

	if err != nil {
		return errors.Wrap(err)
	}

	if !exists {
		errorMessage := fmt.Sprintf("role not found: %s", roleName)
		return errors.New(errorMessage)
	}

	policyDoc, _, err := generatePolicyDocument(statements)

	if err != nil {
		logger.WithError(err).Errorf("failed to generate policy document")
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

func (a *Agent) createPolicy(ctx context.Context, role *types.Role, namespace string, intentsServiceName string, statements []StatementEntry) (*types.Policy, error) {
	fullPolicyName := a.generatePolicyName(namespace, intentsServiceName)
	policyDoc, policyHash, err := generatePolicyDocument(statements)

	if err != nil {
		return nil, errors.Wrap(err)
	}

	policy, err := a.iamClient.CreatePolicy(ctx, &iam.CreatePolicyInput{
		PolicyDocument: aws.String(policyDoc),
		PolicyName:     aws.String(fullPolicyName),
		Tags: []types.Tag{
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
		},
	})

	if err != nil {
		return nil, errors.Wrap(err)
	}

	err = a.attachPolicy(ctx, role, policy.Policy)

	if err != nil {
		return nil, errors.Wrap(err)
	}

	return policy.Policy, nil
}

func (a *Agent) updatePolicy(ctx context.Context, policy *types.Policy, statements []StatementEntry) error {
	policyDoc, policyHash, err := generatePolicyDocument(statements)

	if err != nil {
		return errors.Wrap(err)
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
	})

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
	})

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
	})

	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) attachPolicy(ctx context.Context, role *types.Role, policy *types.Policy) error {
	_, err := a.iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
		PolicyArn: policy.Arn,
		RoleName:  role.RoleName,
	})

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

func (a *Agent) generatePolicyName(ns, intentsServiceName string) string {
	return fmt.Sprintf("otterize-policy-%s-%s", ns, intentsServiceName)

}

func (a *Agent) generatePolicyArn(policyName string) string {
	return fmt.Sprintf("arn:aws:iam::%s:policy/%s", a.accountID, policyName)
}
