package azureagent

import (
	"context"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/google/uuid"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	mock_azureagent "github.com/otterize/intents-operator/src/shared/azureagent/mocks"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

type AzureAgentPoliciesKeyVaultSuite struct {
	suite.Suite

	mockSubscriptionsClient                *mock_azureagent.MockAzureARMSubscriptionsClient
	mockResourceGroupsClient               *mock_azureagent.MockAzureARMResourcesResourceGroupsClient
	mockManagedClustersClient              *mock_azureagent.MockAzureARMContainerServiceManagedClustersClient
	mockUserAssignedIdentitiesClient       *mock_azureagent.MockAzureARMMSIUserAssignedIdentitiesClient
	mockFederatedIdentityCredentialsClient *mock_azureagent.MockAzureARMMSIFederatedIdentityCredentialsClient
	mockRoleDefinitionsClient              *mock_azureagent.MockAzureARMAuthorizationRoleDefinitionsClient
	mockRoleAssignmentsClient              *mock_azureagent.MockAzureARMAuthorizationRoleAssignmentsClient
	mockVaultsClient                       *mock_azureagent.MockAzureARMKeyVaultVaultsClient

	agent *Agent
}

func (s *AzureAgentPoliciesKeyVaultSuite) SetupTest() {
	controller := gomock.NewController(s.T())
	s.mockSubscriptionsClient = mock_azureagent.NewMockAzureARMSubscriptionsClient(controller)
	s.mockResourceGroupsClient = mock_azureagent.NewMockAzureARMResourcesResourceGroupsClient(controller)
	s.mockManagedClustersClient = mock_azureagent.NewMockAzureARMContainerServiceManagedClustersClient(controller)
	s.mockUserAssignedIdentitiesClient = mock_azureagent.NewMockAzureARMMSIUserAssignedIdentitiesClient(controller)
	s.mockFederatedIdentityCredentialsClient = mock_azureagent.NewMockAzureARMMSIFederatedIdentityCredentialsClient(controller)
	s.mockRoleDefinitionsClient = mock_azureagent.NewMockAzureARMAuthorizationRoleDefinitionsClient(controller)
	s.mockRoleAssignmentsClient = mock_azureagent.NewMockAzureARMAuthorizationRoleAssignmentsClient(controller)
	s.mockVaultsClient = mock_azureagent.NewMockAzureARMKeyVaultVaultsClient(controller)

	s.agent = &Agent{
		conf: Config{
			SubscriptionID:          testSubscriptionID,
			ResourceGroup:           testResourceGroup,
			AKSClusterName:          testAKSClusterName,
			TenantID:                testTenantID,
			Location:                testLocation,
			AKSClusterOIDCIssuerURL: testOIDCIssuerURL,
		},
		credentials:                        nil,
		subscriptionClient:                 s.mockSubscriptionsClient,
		resourceGroupsClient:               s.mockResourceGroupsClient,
		managedClustersClient:              s.mockManagedClustersClient,
		userAssignedIdentitiesClient:       s.mockUserAssignedIdentitiesClient,
		federatedIdentityCredentialsClient: s.mockFederatedIdentityCredentialsClient,
		roleDefinitionsClient:              s.mockRoleDefinitionsClient,
		roleAssignmentsClient:              s.mockRoleAssignmentsClient,
		vaultsClient:                       s.mockVaultsClient,
	}

}

func (s *AzureAgentPoliciesKeyVaultSuite) expectGetUserAssignedIdentityReturnsClientID(clientId string) {
	userAssignedIndentityName := s.agent.generateUserAssignedIdentityName(testNamespace, testIntentsServiceName)
	s.mockUserAssignedIdentitiesClient.EXPECT().Get(gomock.Any(), testResourceGroup, userAssignedIndentityName, nil).Return(
		armmsi.UserAssignedIdentitiesClientGetResponse{
			Identity: armmsi.Identity{
				Name: &userAssignedIndentityName,
				Properties: &armmsi.UserAssignedIdentityProperties{
					ClientID: &clientId,
				},
			},
		}, nil)
}

func (s *AzureAgentPoliciesKeyVaultSuite) expectListRoleAssignmentsReturnsEmpty() {
	s.mockRoleAssignmentsClient.EXPECT().NewListForSubscriptionPager(nil).Return(NewListPager[armauthorization.RoleAssignmentsClientListForSubscriptionResponse]())
}

func (s *AzureAgentPoliciesKeyVaultSuite) expectListKeyVaultsReturnsNames(names ...string) {
	s.mockVaultsClient.EXPECT().NewListByResourceGroupPager(testResourceGroup, nil).Return(NewListPager[armkeyvault.VaultsClientListByResourceGroupResponse](
		armkeyvault.VaultsClientListByResourceGroupResponse{
			VaultListResult: armkeyvault.VaultListResult{
				Value: lo.Map(names, func(name string, _ int) *armkeyvault.Vault {
					return &armkeyvault.Vault{
						Name: lo.ToPtr(testKeyVaultName),
					}
				}),
			},
		},
	))
}

func (s *AzureAgentPoliciesKeyVaultSuite) expectGetKeyVaultReturnsAccessPolicies(keyVaultName string, policies []*armkeyvault.AccessPolicyEntry) {
	s.mockVaultsClient.EXPECT().Get(gomock.Any(), testResourceGroup, keyVaultName, nil).Return(armkeyvault.VaultsClientGetResponse{
		Vault: armkeyvault.Vault{
			Name: lo.ToPtr(keyVaultName),
			Properties: &armkeyvault.VaultProperties{
				AccessPolicies: policies,
			},
		},
	}, nil)
}

func (s *AzureAgentPoliciesKeyVaultSuite) expectUpdateKeyVaultAccessPolicyWritesPolicy(keyVaultName string, updateKind armkeyvault.AccessPolicyUpdateKind, updatedPolicy *armkeyvault.VaultAccessPolicyParameters) {
	s.mockVaultsClient.EXPECT().UpdateAccessPolicy(gomock.Any(), testResourceGroup, keyVaultName, updateKind,
		gomock.AssignableToTypeOf(armkeyvault.VaultAccessPolicyParameters{}), nil).DoAndReturn(
		func(ctx context.Context, resourceGroupName string, vaultName string, operationKind armkeyvault.AccessPolicyUpdateKind, parameters armkeyvault.VaultAccessPolicyParameters, options *armkeyvault.VaultsClientUpdateAccessPolicyOptions) (armkeyvault.VaultsClientUpdateAccessPolicyResponse, error) {
			*updatedPolicy = parameters
			return armkeyvault.VaultsClientUpdateAccessPolicyResponse{}, nil
		})

}

type AzureKeyVaultPolicyTestCase struct {
	Name                string
	IntentPolicy        *otterizev1alpha3.AzureKeyVaultPolicy
	ExisingAccessPolicy []*armkeyvault.AccessPolicyEntry
	UpdateExpected      bool
	UpdateKind          armkeyvault.AccessPolicyUpdateKind
}

var azureKeyVaultPolicyTestCases = []AzureKeyVaultPolicyTestCase{
	{
		Name: "AddsNewPolicy",
		IntentPolicy: &otterizev1alpha3.AzureKeyVaultPolicy{
			CertificatePermissions: []otterizev1alpha3.AzureKeyVaultCertificatePermission{otterizev1alpha3.AzureKeyVaultCertificatePermissionAll},
			KeyPermissions:         []otterizev1alpha3.AzureKeyVaultKeyPermission{otterizev1alpha3.AzureKeyVaultKeyPermissionAll},
			SecretPermissions:      []otterizev1alpha3.AzureKeyVaultSecretPermission{otterizev1alpha3.AzureKeyVaultSecretPermissionAll},
			StoragePermissions:     []otterizev1alpha3.AzureKeyVaultStoragePermission{otterizev1alpha3.AzureKeyVaultStoragePermissionAll},
		},
		ExisingAccessPolicy: []*armkeyvault.AccessPolicyEntry{},
		UpdateExpected:      true,
		UpdateKind:          armkeyvault.AccessPolicyUpdateKindAdd,
	},
	{
		Name: "UpdatesExistingPolicy",
		IntentPolicy: &otterizev1alpha3.AzureKeyVaultPolicy{
			CertificatePermissions: []otterizev1alpha3.AzureKeyVaultCertificatePermission{otterizev1alpha3.AzureKeyVaultCertificatePermissionAll},
			KeyPermissions:         []otterizev1alpha3.AzureKeyVaultKeyPermission{otterizev1alpha3.AzureKeyVaultKeyPermissionAll},
			SecretPermissions:      []otterizev1alpha3.AzureKeyVaultSecretPermission{otterizev1alpha3.AzureKeyVaultSecretPermissionAll},
			StoragePermissions:     []otterizev1alpha3.AzureKeyVaultStoragePermission{otterizev1alpha3.AzureKeyVaultStoragePermissionAll},
		},
		ExisingAccessPolicy: []*armkeyvault.AccessPolicyEntry{
			{
				ObjectID: nil, // the test will set it
				Permissions: &armkeyvault.Permissions{
					Certificates: []*armkeyvault.CertificatePermissions{lo.ToPtr(armkeyvault.CertificatePermissionsGet)},
					Keys:         []*armkeyvault.KeyPermissions{lo.ToPtr(armkeyvault.KeyPermissionsGet)},
					Secrets:      []*armkeyvault.SecretPermissions{lo.ToPtr(armkeyvault.SecretPermissionsGet)},
					Storage:      []*armkeyvault.StoragePermissions{lo.ToPtr(armkeyvault.StoragePermissionsGet)},
				},
				TenantID: lo.ToPtr(testTenantID),
			},
		},
		UpdateExpected: true,
		UpdateKind:     armkeyvault.AccessPolicyUpdateKindReplace,
	},
	{
		Name: "IgnoreMatchingExistingPolicy",
		IntentPolicy: &otterizev1alpha3.AzureKeyVaultPolicy{
			CertificatePermissions: []otterizev1alpha3.AzureKeyVaultCertificatePermission{otterizev1alpha3.AzureKeyVaultCertificatePermissionAll},
			KeyPermissions:         []otterizev1alpha3.AzureKeyVaultKeyPermission{otterizev1alpha3.AzureKeyVaultKeyPermissionAll},
			SecretPermissions:      []otterizev1alpha3.AzureKeyVaultSecretPermission{otterizev1alpha3.AzureKeyVaultSecretPermissionAll},
			StoragePermissions:     []otterizev1alpha3.AzureKeyVaultStoragePermission{otterizev1alpha3.AzureKeyVaultStoragePermissionAll},
		},
		ExisingAccessPolicy: []*armkeyvault.AccessPolicyEntry{
			{
				ObjectID: nil, // the test will set it
				Permissions: &armkeyvault.Permissions{
					Certificates: []*armkeyvault.CertificatePermissions{lo.ToPtr(armkeyvault.CertificatePermissionsAll)},
					Keys:         []*armkeyvault.KeyPermissions{lo.ToPtr(armkeyvault.KeyPermissionsAll)},
					Secrets:      []*armkeyvault.SecretPermissions{lo.ToPtr(armkeyvault.SecretPermissionsAll)},
					Storage:      []*armkeyvault.StoragePermissions{lo.ToPtr(armkeyvault.StoragePermissionsAll)},
				},
				TenantID: lo.ToPtr(testTenantID),
			},
		},
		UpdateExpected: false,
	},
	{
		Name:         "RemovesExistingPolicy",
		IntentPolicy: nil,
		ExisingAccessPolicy: []*armkeyvault.AccessPolicyEntry{
			{
				ObjectID: nil, // the test will set it
				Permissions: &armkeyvault.Permissions{
					Certificates: []*armkeyvault.CertificatePermissions{lo.ToPtr(armkeyvault.CertificatePermissionsGet)},
					Keys:         []*armkeyvault.KeyPermissions{lo.ToPtr(armkeyvault.KeyPermissionsGet)},
					Secrets:      []*armkeyvault.SecretPermissions{lo.ToPtr(armkeyvault.SecretPermissionsGet)},
					Storage:      []*armkeyvault.StoragePermissions{lo.ToPtr(armkeyvault.StoragePermissionsGet)},
				},
			},
		},
		UpdateExpected: true,
		UpdateKind:     armkeyvault.AccessPolicyUpdateKindRemove,
	},
}

func (s *AzureAgentPoliciesKeyVaultSuite) TestAddRolePolicyFromIntents_AzureKeyVaultPolicy() {
	for _, testCase := range azureKeyVaultPolicyTestCases {
		s.Run(testCase.Name, func() {
			scope := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.KeyVault/vaults/%s", testSubscriptionID, testResourceGroup, testKeyVaultName)
			intents := []otterizev1alpha3.Intent{
				{
					Name:                scope,
					AzureKeyVaultPolicy: testCase.IntentPolicy,
				},
			}

			clientId := uuid.NewString()
			s.expectGetUserAssignedIdentityReturnsClientID(clientId)
			s.expectListRoleAssignmentsReturnsEmpty()
			s.expectListKeyVaultsReturnsNames(testKeyVaultName)

			for _, policy := range testCase.ExisingAccessPolicy {
				policy.ObjectID = &clientId
			}
			s.expectGetKeyVaultReturnsAccessPolicies(testKeyVaultName, testCase.ExisingAccessPolicy)

			var updatedPolicy armkeyvault.VaultAccessPolicyParameters
			if testCase.UpdateExpected {
				s.expectUpdateKeyVaultAccessPolicyWritesPolicy(testKeyVaultName, testCase.UpdateKind, &updatedPolicy)
			}

			// Act
			err := s.agent.AddRolePolicyFromIntents(context.Background(), testNamespace, testAccountName, testIntentsServiceName, intents)

			// Assert
			s.NoError(err)

			if testCase.UpdateExpected {
				s.Require().Len(updatedPolicy.Properties.AccessPolicies, 1)

				updatedAccessPolicyEntry := updatedPolicy.Properties.AccessPolicies[0]
				s.Require().Equal(clientId, *updatedAccessPolicyEntry.ObjectID)

				updatedPermissions := updatedAccessPolicyEntry.Permissions
				if testCase.UpdateKind == armkeyvault.AccessPolicyUpdateKindRemove {
					s.Require().Nil(updatedPermissions)
				} else {
					s.Require().ElementsMatch([]*armkeyvault.CertificatePermissions{lo.ToPtr(armkeyvault.CertificatePermissionsAll)}, updatedPermissions.Certificates)
					s.Require().ElementsMatch([]*armkeyvault.KeyPermissions{lo.ToPtr(armkeyvault.KeyPermissionsAll)}, updatedPermissions.Keys)
					s.Require().ElementsMatch([]*armkeyvault.SecretPermissions{lo.ToPtr(armkeyvault.SecretPermissionsAll)}, updatedPermissions.Secrets)
					s.Require().ElementsMatch([]*armkeyvault.StoragePermissions{lo.ToPtr(armkeyvault.StoragePermissionsAll)}, updatedPermissions.Storage)
				}
			}
		})
	}
}

func (s *AzureAgentPoliciesKeyVaultSuite) TestDeleteRolePolicyFromIntents_ClearsKeyVaults() {
	// Arrange
	intents := []otterizev1alpha3.Intent{
		{
			Name: "/subscriptions/test-subscriptionid/resourceGroups/test-resourcegroup/providers/Microsoft.KeyVault/vaults/test-keyvaultname",
		},
	}

	clientId := uuid.NewString()
	s.expectGetUserAssignedIdentityReturnsClientID(clientId)
	s.expectListRoleAssignmentsReturnsEmpty()
	s.expectListKeyVaultsReturnsNames(testKeyVaultName)

	s.expectGetKeyVaultReturnsAccessPolicies(testKeyVaultName, []*armkeyvault.AccessPolicyEntry{
		{
			ObjectID: &clientId,
			Permissions: &armkeyvault.Permissions{
				Certificates: []*armkeyvault.CertificatePermissions{lo.ToPtr(armkeyvault.CertificatePermissionsAll)},
				Keys:         []*armkeyvault.KeyPermissions{lo.ToPtr(armkeyvault.KeyPermissionsAll)},
				Secrets:      []*armkeyvault.SecretPermissions{lo.ToPtr(armkeyvault.SecretPermissionsAll)},
				Storage:      []*armkeyvault.StoragePermissions{lo.ToPtr(armkeyvault.StoragePermissionsAll)},
			},
		},
	})

	var updatedPolicy armkeyvault.VaultAccessPolicyParameters
	s.expectUpdateKeyVaultAccessPolicyWritesPolicy(testKeyVaultName, armkeyvault.AccessPolicyUpdateKindRemove, &updatedPolicy)

	// Act
	clientIntents := otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testAccountName,
			Namespace: testNamespace,
		},
		Spec: &otterizev1alpha3.IntentsSpec{
			Service: otterizev1alpha3.Service{Name: testIntentsServiceName},
			Calls:   intents,
		},
	}
	err := s.agent.DeleteRolePolicyFromIntents(context.Background(), clientIntents)

	// Assert
	s.NoError(err)
	s.Require().Len(updatedPolicy.Properties.AccessPolicies, 1)

	updatedAccessPolicyEntry := updatedPolicy.Properties.AccessPolicies[0]
	s.Require().Equal(clientId, *updatedAccessPolicyEntry.ObjectID)
	s.Require().Nil(updatedAccessPolicyEntry.Permissions)
}

func TestAzureAgentPoliciesKeyVaultSuite(t *testing.T) {
	suite.Run(t, new(AzureAgentPoliciesKeyVaultSuite))
}
