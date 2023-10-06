package awsagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/sirupsen/logrus"
)

func (a *Agent) SetRolePolicy(ctx context.Context, accountName string, statements []StatementEntry) error {
	logger := logrus.WithField("account", accountName)

	exists, role, err := a.GetOtterizeRole(ctx, accountName)

	if err != nil {
		return err
	}

	if !exists {
		errorMessage := fmt.Sprintf("role not found: %s", generateRoleName(accountName))
		logger.Error(errorMessage)
		return errors.New(errorMessage)
	}

	policyDoc, err := generatePolicyDocument(statements)

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

func generatePolicyDocument(statements []StatementEntry) (string, error) {
	policy := PolicyDocument{
		Version:   iamAPIVersion,
		Statement: statements,
	}
	serialized, err := json.Marshal(policy)

	if err != nil {
		return "", err
	}

	return string(serialized), nil
}
