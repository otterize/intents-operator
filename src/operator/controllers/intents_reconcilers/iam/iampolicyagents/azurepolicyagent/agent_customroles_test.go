package azurepolicyagent

import (
	"context"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/google/uuid"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/azureagent"
	mock_azureagent "github.com/otterize/intents-operator/src/shared/azureagent/mocks"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	"sync"
	"testing"
)

type AzureCustomRoleTestCase struct {
	Name                   string
	Roles                  []string
	Actions                []otterizev2alpha1.AzureAction
	DataActions            []otterizev2alpha1.AzureDataAction
	ExisingRoles           []*armauthorization.RoleDefinition
	UpdateExpected         bool
	ShouldCreateAssignment bool
}

type AzureAgentPoliciesCustomRolesSuite struct {
	suite.Suite

	mockResourcesClient                    *mock_azureagent.MockAzureARMResourcesClient
	mockSubscriptionsClient                *mock_azureagent.MockAzureARMSubscriptionsClient
	mockResourceGroupsClient               *mock_azureagent.MockAzureARMResourcesResourceGroupsClient
	mockManagedClustersClient              *mock_azureagent.MockAzureARMContainerServiceManagedClustersClient
	mockUserAssignedIdentitiesClient       *mock_azureagent.MockAzureARMMSIUserAssignedIdentitiesClient
	mockFederatedIdentityCredentialsClient *mock_azureagent.MockAzureARMMSIFederatedIdentityCredentialsClient
	mockRoleDefinitionsClient              *mock_azureagent.MockAzureARMAuthorizationRoleDefinitionsClient
	mockRoleAssignmentsClient              *mock_azureagent.MockAzureARMAuthorizationRoleAssignmentsClient
	mockVaultsClient                       *mock_azureagent.MockAzureARMKeyVaultVaultsClient

	subscriptionToRoleAssignmentsClient map[string]azureagent.AzureARMAuthorizationRoleAssignmentsClient

	agent *Agent
}

func (s *AzureAgentPoliciesCustomRolesSuite) expectGetByIDReturnsResource(scope string) {
	s.mockResourcesClient.EXPECT().GetByID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(armresources.ClientGetByIDResponse{
		GenericResource: armresources.GenericResource{
			ID: &scope,
		},
	}, nil)
}

func (s *AzureAgentPoliciesCustomRolesSuite) expectListSubscriptionsReturnsPager() {
	s.mockSubscriptionsClient.EXPECT().NewListPager(nil).Return(azureagent.NewListPager[armsubscriptions.ClientListResponse](
		armsubscriptions.ClientListResponse{
			SubscriptionListResult: armsubscriptions.SubscriptionListResult{
				Value: []*armsubscriptions.Subscription{
					{
						SubscriptionID: lo.ToPtr(testSubscriptionID),
					},
				},
			},
		},
	))
}

func (s *AzureAgentPoliciesCustomRolesSuite) expectListRoleDefinitionsReturnsPager(roles []*armauthorization.RoleDefinition) {
	s.mockRoleDefinitionsClient.EXPECT().NewListPager(gomock.Any(), gomock.Any()).Return(azureagent.NewListPager[armauthorization.RoleDefinitionsClientListResponse](
		armauthorization.RoleDefinitionsClientListResponse{
			RoleDefinitionListResult: armauthorization.RoleDefinitionListResult{
				Value: roles,
			},
		},
	))
}

func (s *AzureAgentPoliciesCustomRolesSuite) expectCreateOrUpdateRoleDefinitionWriteRoleDefinition(customRoleDefinition *armauthorization.RoleDefinition) {
	s.mockRoleDefinitionsClient.EXPECT().CreateOrUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, scope string, roleDefinitionID string, roleDefinition armauthorization.RoleDefinition, options *armauthorization.RoleDefinitionsClientCreateOrUpdateOptions) (armauthorization.RoleDefinitionsClientCreateOrUpdateResponse, error) {
			*customRoleDefinition = roleDefinition
			return armauthorization.RoleDefinitionsClientCreateOrUpdateResponse{
				RoleDefinition: roleDefinition,
			}, nil
		},
	)
}

func (s *AzureAgentPoliciesCustomRolesSuite) expectCreateRoleAssignmentReturnsEmpty() {
	s.mockRoleAssignmentsClient.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(armauthorization.RoleAssignmentsClientCreateResponse{
		RoleAssignment: armauthorization.RoleAssignment{
			Properties: &armauthorization.RoleAssignmentProperties{},
		},
	}, nil)
}

func (s *AzureAgentPoliciesCustomRolesSuite) expectListRoleAssignmentsReturnsEmpty() {
	s.mockRoleAssignmentsClient.EXPECT().NewListForSubscriptionPager(nil).Return(azureagent.NewListPager[armauthorization.RoleAssignmentsClientListForSubscriptionResponse]())
}

func (s *AzureAgentPoliciesCustomRolesSuite) expectGetUserAssignedIdentityReturnsClientID(clientId string) {
	userAssignedIndentityName := s.agent.GenerateUserAssignedIdentityName(testNamespace, testIntentsServiceName)
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

func (s *AzureAgentPoliciesCustomRolesSuite) expectListKeyVaultsReturnsEmpty() {
	s.mockVaultsClient.EXPECT().NewListByResourceGroupPager(testResourceGroup, nil).Return(azureagent.NewListPager[armkeyvault.VaultsClientListByResourceGroupResponse](
		armkeyvault.VaultsClientListByResourceGroupResponse{},
	))
}

func (s *AzureAgentPoliciesCustomRolesSuite) SetupTest() {
	controller := gomock.NewController(s.T())

	s.mockResourcesClient = mock_azureagent.NewMockAzureARMResourcesClient(controller)
	s.mockSubscriptionsClient = mock_azureagent.NewMockAzureARMSubscriptionsClient(controller)
	s.mockResourceGroupsClient = mock_azureagent.NewMockAzureARMResourcesResourceGroupsClient(controller)
	s.mockManagedClustersClient = mock_azureagent.NewMockAzureARMContainerServiceManagedClustersClient(controller)
	s.mockUserAssignedIdentitiesClient = mock_azureagent.NewMockAzureARMMSIUserAssignedIdentitiesClient(controller)
	s.mockFederatedIdentityCredentialsClient = mock_azureagent.NewMockAzureARMMSIFederatedIdentityCredentialsClient(controller)
	s.mockRoleDefinitionsClient = mock_azureagent.NewMockAzureARMAuthorizationRoleDefinitionsClient(controller)
	s.mockRoleAssignmentsClient = mock_azureagent.NewMockAzureARMAuthorizationRoleAssignmentsClient(controller)
	s.mockVaultsClient = mock_azureagent.NewMockAzureARMKeyVaultVaultsClient(controller)

	s.subscriptionToRoleAssignmentsClient = make(map[string]azureagent.AzureARMAuthorizationRoleAssignmentsClient)
	s.subscriptionToRoleAssignmentsClient[testSubscriptionID] = s.mockRoleAssignmentsClient

	s.agent = &Agent{
		azureagent.NewAzureAgentFromClients(
			azureagent.Config{
				SubscriptionID:          testSubscriptionID,
				ResourceGroup:           testResourceGroup,
				AKSClusterName:          testAKSClusterName,
				TenantID:                testTenantID,
				Location:                testLocation,
				AKSClusterOIDCIssuerURL: testOIDCIssuerURL,
			},
			nil,
			s.mockResourcesClient,
			s.mockSubscriptionsClient,
			s.mockResourceGroupsClient,
			s.mockManagedClustersClient,
			s.mockUserAssignedIdentitiesClient,
			s.mockFederatedIdentityCredentialsClient,
			s.mockRoleDefinitionsClient,
			s.mockRoleAssignmentsClient,
			s.mockVaultsClient,
			s.subscriptionToRoleAssignmentsClient,
		),
		sync.Mutex{},
		sync.Mutex{},
	}
}

var azureCustomRoleTestCases = []AzureCustomRoleTestCase{
	{
		Name: "AddsNewPolicy",
		Actions: []otterizev2alpha1.AzureAction{
			"Microsoft.Storage/storageAccounts/blobServices/containers/read",
		},
		DataActions: []otterizev2alpha1.AzureDataAction{
			"Microsoft.Storage/storageAccounts/blobServices/containers/blobs/read",
			"Microsoft.Storage/storageAccounts/blobServices/containers/blobs/add/action",
		},
		ExisingRoles:           nil,
		UpdateExpected:         true,
		ShouldCreateAssignment: true,
	},
	{
		Name: "UpdatesExistingPolicy",
		Actions: []otterizev2alpha1.AzureAction{
			"Microsoft.Storage/storageAccounts/blobServices/containers/read",
			"Microsoft.Storage/storageAccounts/blobServices/containers/write",
		},
		ExisingRoles: []*armauthorization.RoleDefinition{
			{
				Name: to.Ptr("otterizeCustomRole"),
				Properties: &armauthorization.RoleDefinitionProperties{
					Permissions: []*armauthorization.Permission{
						{
							Actions: []*string{
								to.Ptr("Microsoft.Storage/storageAccounts/blobServices/containers/read"),
							},
						},
					},
				},
			},
		},
		UpdateExpected:         true,
		ShouldCreateAssignment: false,
	},
	{
		// This test case is for backwards compatibility with the old built-in roles
		// The storage blob is a builtin (existing role) that is being used in the new custom role
		// No custom role should be created - only assignment
		Name: "BackwardsCompatibility",
		Roles: []string{
			"Storage Blob Data Reader",
		},
		Actions: []otterizev2alpha1.AzureAction{
			"Microsoft.Storage/storageAccounts/blobServices/containers/read",
		},
		ExisingRoles: []*armauthorization.RoleDefinition{
			{
				ID:   to.Ptr("Storage Blob Data Reader"),
				Name: to.Ptr("Storage Blob Data Reader"),
				Properties: &armauthorization.RoleDefinitionProperties{
					RoleName: to.Ptr("Storage Blob Data Reader"),
					Permissions: []*armauthorization.Permission{
						{
							Actions: []*string{
								to.Ptr("Microsoft.Storage/storageAccounts/blobServices/containers/read"),
							},
						},
					},
				},
			},
		},
		UpdateExpected:         false,
		ShouldCreateAssignment: true,
	},
}

func (s *AzureAgentPoliciesCustomRolesSuite) TestAddRolePolicyFromIntents_CustomRoles() {
	for _, testCase := range azureCustomRoleTestCases {
		targetScope := "/providers/Microsoft.Storage/storageAccounts/test/blobServices/default/containers/container"

		s.Run(testCase.Name, func() {
			intents := []otterizev2alpha1.Target{
				{
					Azure: &otterizev2alpha1.AzureTarget{
						Scope:       targetScope,
						Roles:       testCase.Roles,
						Actions:     testCase.Actions,
						DataActions: testCase.DataActions,
					},
				},
			}

			clientId := uuid.NewString()
			s.expectGetUserAssignedIdentityReturnsClientID(clientId)

			s.expectListKeyVaultsReturnsEmpty()

			// Two calls - one from custom roles and one from backwards compatibility to built-in roles
			s.expectListSubscriptionsReturnsPager()
			s.expectListSubscriptionsReturnsPager()
			s.expectListRoleAssignmentsReturnsEmpty()
			s.expectListRoleAssignmentsReturnsEmpty()

			// CustomRole related calls
			s.expectListRoleDefinitionsReturnsPager(testCase.ExisingRoles)
			if testCase.ShouldCreateAssignment {
				s.expectCreateRoleAssignmentReturnsEmpty()
			}

			// Make sure the custom role is created
			var customRoleDefinition armauthorization.RoleDefinition
			if testCase.UpdateExpected {
				s.expectGetByIDReturnsResource(targetScope)
				s.expectCreateOrUpdateRoleDefinitionWriteRoleDefinition(&customRoleDefinition)
			}

			err := s.agent.AddRolePolicyFromIntents(context.Background(), testNamespace, testAccountName, testIntentsServiceName, intents, corev1.Pod{})
			s.NoError(err)

			if testCase.UpdateExpected {
				s.Require().Len(customRoleDefinition.Properties.Permissions, 1)
				s.Require().Len(customRoleDefinition.Properties.Permissions[0].Actions, len(testCase.Actions))
				s.Require().Len(customRoleDefinition.Properties.Permissions[0].DataActions, len(testCase.DataActions))
			}
		})
	}
}

func TestAzureAgentPoliciesCustomRolesSuite(t *testing.T) {
	suite.Run(t, new(AzureAgentPoliciesCustomRolesSuite))
}
