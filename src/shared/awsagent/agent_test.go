package awsagent

import (
	"context"
	"encoding/json"
	"github.com/samber/lo"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	awstypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/otterize/intents-operator/src/shared/awsagent/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
)

func makeServiceAccount() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-sa",
			Namespace:   "test-ns",
			Annotations: map[string]string{},
			Labels:      map[string]string{},
		},
	}
}

func TestCreateOtterizeIAMRole_NoRoleExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockIAM := mock_awsagent.NewMockIAMClient(ctrl)
	agent := &Agent{
		iamClient:   mockIAM,
		AccountID:   "123456789012",
		OidcURL:     "oidc.eks.region.amazonaws.com/id/EXAMPLED539D4633E53DE1B716D3041E",
		ClusterName: "test-cluster",
	}

	sa := makeServiceAccount()
	trustStatements := []StatementEntry{
		{
			Effect: "Allow",
			Principal: map[string]string{
				"Federated": "arn:aws:iam::123456789012:oidc-provider/oidc.eks.region.amazonaws.com/id/EXAMPLED539D4633E53DE1B716D3041E",
			},
			Action: []string{"sts:AssumeRoleWithWebIdentity"},
			Condition: map[string]any{
				"StringEquals": map[string]string{
					"oidc.eks.region.amazonaws.com/id/EXAMPLED539D4633E53DE1B716D3041E:aud": "sts.amazonaws.com",
					"oidc.eks.region.amazonaws.com/id/EXAMPLED539D4633E53DE1B716D3041E:sub": "system:serviceaccount:test-ns:test-sa",
				},
			},
		},
	}
	trustPolicy := PolicyDocument{
		Version:   iamAPIVersion,
		Statement: trustStatements,
	}

	trustPolicyJSON, err := json.Marshal(trustPolicy)
	require.NoError(t, err)

	// Simulate GetRole returns NoSuchEntity
	mockIAM.EXPECT().
		GetRole(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, &awstypes.NoSuchEntityException{})

	// Simulate CreateRole is called, and check the AssumeRolePolicyDocument argument
	mockIAM.EXPECT().
		CreateRole(
			gomock.Any(),
			gomock.AssignableToTypeOf(&iam.CreateRoleInput{}),
			gomock.Any(),
		).
		DoAndReturn(
			func(ctx context.Context, input *iam.CreateRoleInput, opts ...func(*iam.Options)) (*iam.CreateRoleOutput, error) {
				require.JSONEq(t, string(trustPolicyJSON), *input.AssumeRolePolicyDocument)
				return &iam.CreateRoleOutput{
					Role: &awstypes.Role{
						Arn:                      input.RoleName,
						RoleName:                 input.RoleName,
						AssumeRolePolicyDocument: input.AssumeRolePolicyDocument,
					},
				}, nil
			},
		)

	role, err := agent.CreateOtterizeIAMRole(context.Background(), sa.Namespace, sa.Name, false, nil)
	require.NoError(t, err)
	require.NotNil(t, role)
}

func TestCreateOtterizeIAMRole_RoleExistsDifferentTrust(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockIAM := mock_awsagent.NewMockIAMClient(ctrl)
	agent := &Agent{
		iamClient:   mockIAM,
		AccountID:   "123456789012",
		OidcURL:     "oidc.eks.region.amazonaws.com/id/EXAMPLED539D4633E53DE1B716D3041E",
		ClusterName: "test-cluster",
	}

	sa := makeServiceAccount()
	additionalTrust := []StatementEntry{
		{
			Effect: "Allow",
			Principal: map[string]string{
				"Service": "ec2.amazonaws.com",
			},
			Action: []string{"sts:AssumeRole"},
		},
	}
	expectedTrust := PolicyDocument{
		Version: iamAPIVersion,
		Statement: []StatementEntry{
			{
				Effect: "Allow",
				Principal: map[string]string{
					"Federated": "arn:aws:iam::123456789012:oidc-provider/oidc.eks.region.amazonaws.com/id/EXAMPLED539D4633E53DE1B716D3041E",
				},
				Action: []string{"sts:AssumeRoleWithWebIdentity"},
				Condition: map[string]any{
					"StringEquals": map[string]string{
						"oidc.eks.region.amazonaws.com/id/EXAMPLED539D4633E53DE1B716D3041E:aud": "sts.amazonaws.com",
						"oidc.eks.region.amazonaws.com/id/EXAMPLED539D4633E53DE1B716D3041E:sub": "system:serviceaccount:test-ns:test-sa",
					},
				},
			},
			additionalTrust[0],
		},
	}
	existingTrust := PolicyDocument{
		Version: iamAPIVersion,
		Statement: []StatementEntry{
			{
				Effect: "Allow",
				Principal: map[string]string{
					"Federated": "arn:aws:iam::123456789012:oidc-provider/oidc.eks.region.amazonaws.com/id/EXAMPLED539D4633E53DE1B716D3041E",
				},
				Action: []string{"sts:AssumeRoleWithWebIdentity"},
				Condition: map[string]any{
					"StringEquals": map[string]string{
						"oidc.eks.region.amazonaws.com/id/EXAMPLED539D4633E53DE1B716D3041E:aud": "sts.amazonaws.com",
						"oidc.eks.region.amazonaws.com/id/EXAMPLED539D4633E53DE1B716D3041E:sub": "system:serviceaccount:test-ns:test-sa",
					},
				},
			},
		},
	}

	existingTrustJSON, err := json.Marshal(existingTrust)
	require.NoError(t, err)
	expectedTrustJSON, err := json.Marshal(expectedTrust)
	require.NoError(t, err)

	// Simulate GetRole returns a role with different trust relationship
	mockIAM.EXPECT().
		GetRole(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&iam.GetRoleOutput{
			Role: &awstypes.Role{
				Arn:                      lo.ToPtr("arn:aws:iam::123456789012:role/test-role"),
				RoleName:                 lo.ToPtr("test-role"),
				AssumeRolePolicyDocument: lo.ToPtr(string(existingTrustJSON)),
			},
		}, nil)

	// Simulate UpdateAssumeRolePolicy is called and check PolicyDocument argument
	mockIAM.EXPECT().
		UpdateAssumeRolePolicy(
			gomock.Any(),
			gomock.AssignableToTypeOf(&iam.UpdateAssumeRolePolicyInput{}),
			gomock.Any(),
		).
		DoAndReturn(
			func(ctx context.Context, input *iam.UpdateAssumeRolePolicyInput, opts ...func(*iam.Options)) (*iam.UpdateAssumeRolePolicyOutput, error) {
				require.JSONEq(t, string(expectedTrustJSON), *input.PolicyDocument)
				return &iam.UpdateAssumeRolePolicyOutput{}, nil
			},
		)

	role, err := agent.CreateOtterizeIAMRole(context.Background(), sa.Namespace, sa.Name, false, additionalTrust)
	require.NoError(t, err)
	require.NotNil(t, role)
	require.JSONEq(t, string(expectedTrustJSON), *role.AssumeRolePolicyDocument)
}
