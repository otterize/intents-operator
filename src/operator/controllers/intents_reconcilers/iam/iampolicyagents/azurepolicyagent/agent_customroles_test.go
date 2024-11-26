package azurepolicyagent

import (
	"context"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/google/uuid"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/azureagent"
	mock_azureagent "github.com/otterize/intents-operator/src/shared/azureagent/mocks"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	"sync"
	"testing"
)

type AzureCustomRoleTestCase struct {
	Name                   string
	Actions                []otterizev2alpha1.AzureAction
	DataActions            []otterizev2alpha1.AzureDataAction
	ExisingCustomRoles     []*armauthorization.RoleDefinition
	UpdateExpected         bool
	ShouldCreateAssignment bool
}

type AzureAgentPoliciesCustomRolesSuite struct {
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

func (s *AzureAgentPoliciesCustomRolesSuite) expectListRoleDefinitionsReturnsEmpty(scope string, roles []*armauthorization.RoleDefinition) {
	s.mockRoleDefinitionsClient.EXPECT().NewListPager(scope, gomock.Any()).Return(azureagent.NewListPager[armauthorization.RoleDefinitionsClientListResponse](
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
	s.mockSubscriptionsClient = mock_azureagent.NewMockAzureARMSubscriptionsClient(controller)
	s.mockResourceGroupsClient = mock_azureagent.NewMockAzureARMResourcesResourceGroupsClient(controller)
	s.mockManagedClustersClient = mock_azureagent.NewMockAzureARMContainerServiceManagedClustersClient(controller)
	s.mockUserAssignedIdentitiesClient = mock_azureagent.NewMockAzureARMMSIUserAssignedIdentitiesClient(controller)
	s.mockFederatedIdentityCredentialsClient = mock_azureagent.NewMockAzureARMMSIFederatedIdentityCredentialsClient(controller)
	s.mockRoleDefinitionsClient = mock_azureagent.NewMockAzureARMAuthorizationRoleDefinitionsClient(controller)
	s.mockRoleAssignmentsClient = mock_azureagent.NewMockAzureARMAuthorizationRoleAssignmentsClient(controller)
	s.mockVaultsClient = mock_azureagent.NewMockAzureARMKeyVaultVaultsClient(controller)

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
			s.mockSubscriptionsClient,
			s.mockResourceGroupsClient,
			s.mockManagedClustersClient,
			s.mockUserAssignedIdentitiesClient,
			s.mockFederatedIdentityCredentialsClient,
			s.mockRoleDefinitionsClient,
			s.mockRoleAssignmentsClient,
			s.mockVaultsClient,
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
		ExisingCustomRoles:     nil,
		UpdateExpected:         true,
		ShouldCreateAssignment: true,
	},
	{
		Name: "AddsNewPolicy",
		Actions: []otterizev2alpha1.AzureAction{
			"Microsoft.Storage/storageAccounts/blobServices/containers/read",
			"Microsoft.Storage/storageAccounts/blobServices/containers/write",
		},
		ExisingCustomRoles: []*armauthorization.RoleDefinition{
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
}

func (s *AzureAgentPoliciesCustomRolesSuite) TestAddRolePolicyFromIntents_CustomRoles() {
	for _, testCase := range azureCustomRoleTestCases {
		s.Run(testCase.Name, func() {
			intents := []otterizev2alpha1.Target{
				{
					Azure: &otterizev2alpha1.AzureTarget{
						Scope:       "/providers/Microsoft.Storage/storageAccounts/test/blobServices/default/containers/container",
						Actions:     testCase.Actions,
						DataActions: testCase.DataActions,
					},
				},
			}

			customRoleScope := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", testSubscriptionID, testResourceGroup)

			clientId := uuid.NewString()
			s.expectGetUserAssignedIdentityReturnsClientID(clientId)

			s.expectListKeyVaultsReturnsEmpty()

			// Two calls - one from custom roles and one from backwards compatibility to built-in roles
			s.expectListRoleAssignmentsReturnsEmpty()
			s.expectListRoleAssignmentsReturnsEmpty()

			// CustomRole related calls
			s.expectListRoleDefinitionsReturnsEmpty(customRoleScope, testCase.ExisingCustomRoles)
			if testCase.ShouldCreateAssignment {
				s.expectCreateRoleAssignmentReturnsEmpty()
			}

			// Make sure the custom role is created
			var customRoleDefinition armauthorization.RoleDefinition
			s.expectCreateOrUpdateRoleDefinitionWriteRoleDefinition(&customRoleDefinition)

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
