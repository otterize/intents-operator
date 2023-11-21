package awsagent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/smithy-go"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"strings"
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
				logger.WithError(err).Error("unable to get role")
				return false, nil, err
			}
		}
	}

	return true, role.Role, nil
}

// CreateOtterizeIAMRole creates a new IAM role for service, if one doesn't exist yet
func (a *Agent) CreateOtterizeIAMRole(ctx context.Context, namespaceName, accountName string) (*types.Role, error) {
	logger := logrus.WithField("namespace", namespaceName).WithField("account", accountName)
	exists, role, err := a.GetOtterizeRole(ctx, namespaceName, accountName)

	if err != nil {
		return nil, err
	}

	if exists {
		logger.Debugf("found existing role, arn: %s", *role.Arn)
		return role, nil
	} else {
		trustPolicy, err := a.generateTrustPolicy(namespaceName, accountName)

		if err != nil {
			return nil, err
		}

		createRoleOutput, createRoleError := a.iamClient.CreateRole(ctx, &iam.CreateRoleInput{
			RoleName:                 aws.String(a.generateRoleName(namespaceName, accountName)),
			AssumeRolePolicyDocument: aws.String(trustPolicy),
			Tags: []types.Tag{
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
			},
			Description: aws.String(iamRoleDescription),
		})

		if createRoleError != nil {
			logger.WithError(createRoleError).Error("failed to create role")
			return nil, createRoleError
		}

		logger.Debugf("created new role, arn: %s", *createRoleOutput.Role.Arn)
		return createRoleOutput.Role, nil
	}
}

func (a *Agent) DeleteOtterizeIAMRole(ctx context.Context, namespaceName, accountName string) error {
	logger := logrus.WithField("namespace", namespaceName).WithField("account", accountName)
	exists, role, err := a.GetOtterizeRole(ctx, namespaceName, accountName)

	if err != nil {
		return err
	}

	if !exists {
		logger.Info("role does not exist, not doing anything")
		return nil
	}

	_, found := lo.Find(role.Tags, func(tag types.Tag) bool {
		response := true
		response = response && (*tag.Key == serviceAccountNameTagKey && *tag.Value == accountName)
		response = response && (*tag.Key == serviceAccountNamespaceTagKey && *tag.Value == namespaceName)
		response = response && (*tag.Key == clusterNameTagKey && *tag.Value == a.clusterName)

		return response
	})

	if !found {
		errorMessage := "refusing to delete role that's not tagged with with appropriate tags"
		logger.WithField("arn", *role.Arn).Error(errorMessage)
		return errors.New(errorMessage)
	}

	err = a.deleteInlineRolePolicy(ctx, role)

	if err != nil {
		return err
	}

	_, err = a.iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{
		RoleName: role.RoleName,
	})

	if err != nil {
		logger.WithError(err).Errorf("failed to delete role")
		return err
	}

	return nil
}

// deleteInlineRolePolicy deletes the inline role policy, if exists, in preparation to delete the role.
func (a *Agent) deleteInlineRolePolicy(ctx context.Context, role *types.Role) error {
	_, err := a.iamClient.GetRolePolicy(ctx, &iam.GetRolePolicyInput{
		PolicyName: role.RoleName,
		RoleName:   role.RoleName,
	})

	if err != nil {
		var nse *types.NoSuchEntityException
		if errors.As(err, &nse) {
			return nil
		}

		return err
	}

	_, err = a.iamClient.DeleteRolePolicy(ctx, &iam.DeleteRolePolicyInput{
		PolicyName: role.RoleName,
		RoleName:   role.RoleName,
	})

	return err
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
		return "", err
	}

	return string(serialized), err
}

func (a *Agent) generateRoleName(namespace string, accountName string) string {
	fullName := fmt.Sprintf("otr-%s.%s@%s", namespace, accountName, a.clusterName)

	var truncatedName string
	if len(fullName) >= (maxTruncatedLength) {
		truncatedName = fullName[:maxTruncatedLength]
	} else {
		truncatedName = fullName
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(fullName)))
	hash = hash[:truncatedHashLength]

	return fmt.Sprintf("%s-%s", truncatedName, hash)
}

func (a *Agent) GenerateRoleARN(namespace string, accountName string) string {
	roleName := a.generateRoleName(namespace, accountName)
	return fmt.Sprintf("arn:aws:iam::%s:role/%s", a.accountID, roleName)
}
