package azurepolicyagent

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/otterize/intents-operator/src/shared/azureagent"
	mock_azureagent "github.com/otterize/intents-operator/src/shared/azureagent/mocks"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"k8s.io/client-go/tools/record"
	"sync"
	"testing"
)

type AzureCleanupCustomRolesTestCase struct {
	Name                string
	ExistingRoles       []*armauthorization.RoleDefinition
	ExistingAssignments []*armauthorization.RoleAssignment
	CleanupRole         *string
	CleanupAssignment   *string
}

type AzureAgentPeriodicTasksSuite struct {
	suite.Suite

	mockResourcesClient                    *mock_azureagent.MockAzureARMResourcesClient
	mockProviderResourceTypesClient        *mock_azureagent.MockAzureARMResourcesProviderResourceTypesClient
	mockSubscriptionsClient                *mock_azureagent.MockAzureARMSubscriptionsClient
	mockResourceGroupsClient               *mock_azureagent.MockAzureARMResourcesResourceGroupsClient
	mockManagedClustersClient              *mock_azureagent.MockAzureARMContainerServiceManagedClustersClient
	mockUserAssignedIdentitiesClient       *mock_azureagent.MockAzureARMMSIUserAssignedIdentitiesClient
	mockFederatedIdentityCredentialsClient *mock_azureagent.MockAzureARMMSIFederatedIdentityCredentialsClient
	mockRoleDefinitionsClient              *mock_azureagent.MockAzureARMAuthorizationRoleDefinitionsClient
	mockRoleAssignmentsClient              *mock_azureagent.MockAzureARMAuthorizationRoleAssignmentsClient
	mockVaultsClient                       *mock_azureagent.MockAzureARMKeyVaultVaultsClient

	subscriptionToResourceClient        map[string]azureagent.AzureARMResourcesClient
	subscriptionToRoleAssignmentsClient map[string]azureagent.AzureARMAuthorizationRoleAssignmentsClient

	agent *Agent
}

func (s *AzureAgentPeriodicTasksSuite) expectListSubscriptionsReturnsPager() {
	s.mockSubscriptionsClient.EXPECT().NewListPager(nil).Return(azureagent.NewListPager[armsubscriptions.ClientListResponse](
		armsubscriptions.ClientListResponse{
			SubscriptionListResult: armsubscriptions.SubscriptionListResult{
				Value: []*armsubscriptions.Subscription{
					{
						ID:             lo.ToPtr(testSubscriptionID),
						SubscriptionID: lo.ToPtr(testSubscriptionID),
					},
				},
			},
		},
	))
}

func (s *AzureAgentPeriodicTasksSuite) expectListRoleDefinitionsReturnsPager(roles []*armauthorization.RoleDefinition) {
	s.mockRoleDefinitionsClient.EXPECT().NewListPager(gomock.Any(), gomock.Any()).Return(azureagent.NewListPager[armauthorization.RoleDefinitionsClientListResponse](
		armauthorization.RoleDefinitionsClientListResponse{
			RoleDefinitionListResult: armauthorization.RoleDefinitionListResult{
				Value: roles,
			},
		},
	))
}

func (s *AzureAgentPeriodicTasksSuite) expectListRoleAssignmentsReturnsPager(assignments []*armauthorization.RoleAssignment) {
	s.mockRoleAssignmentsClient.EXPECT().NewListForSubscriptionPager(nil).Return(azureagent.NewListPager[armauthorization.RoleAssignmentsClientListForSubscriptionResponse](
		armauthorization.RoleAssignmentsClientListForSubscriptionResponse{
			RoleAssignmentListResult: armauthorization.RoleAssignmentListResult{
				Value: assignments,
			},
		},
	))
}

func (s *AzureAgentPeriodicTasksSuite) expectDeleteRoleAssignmentSuccess(scope string) {
	s.mockRoleAssignmentsClient.EXPECT().Delete(gomock.Any(), scope, gomock.Any(), gomock.Any()).Return(
		armauthorization.RoleAssignmentsClientDeleteResponse{}, nil,
	)
}

func (s *AzureAgentPeriodicTasksSuite) expectDeleteCustomRoleDefinitionSuccess(roleDefinitionID string) {
	s.mockRoleDefinitionsClient.EXPECT().Delete(gomock.Any(), gomock.Any(), roleDefinitionID, nil).Return(
		armauthorization.RoleDefinitionsClientDeleteResponse{}, nil,
	)
}

func (s *AzureAgentPeriodicTasksSuite) SetupTest() {
	controller := gomock.NewController(s.T())

	s.mockResourcesClient = mock_azureagent.NewMockAzureARMResourcesClient(controller)
	s.mockProviderResourceTypesClient = mock_azureagent.NewMockAzureARMResourcesProviderResourceTypesClient(controller)
	s.mockSubscriptionsClient = mock_azureagent.NewMockAzureARMSubscriptionsClient(controller)
	s.mockResourceGroupsClient = mock_azureagent.NewMockAzureARMResourcesResourceGroupsClient(controller)
	s.mockManagedClustersClient = mock_azureagent.NewMockAzureARMContainerServiceManagedClustersClient(controller)
	s.mockUserAssignedIdentitiesClient = mock_azureagent.NewMockAzureARMMSIUserAssignedIdentitiesClient(controller)
	s.mockFederatedIdentityCredentialsClient = mock_azureagent.NewMockAzureARMMSIFederatedIdentityCredentialsClient(controller)
	s.mockRoleDefinitionsClient = mock_azureagent.NewMockAzureARMAuthorizationRoleDefinitionsClient(controller)
	s.mockRoleAssignmentsClient = mock_azureagent.NewMockAzureARMAuthorizationRoleAssignmentsClient(controller)
	s.mockVaultsClient = mock_azureagent.NewMockAzureARMKeyVaultVaultsClient(controller)

	s.subscriptionToResourceClient = make(map[string]azureagent.AzureARMResourcesClient)
	s.subscriptionToResourceClient[testSubscriptionID] = s.mockResourcesClient

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
			s.mockProviderResourceTypesClient,
			s.mockSubscriptionsClient,
			s.mockResourceGroupsClient,
			s.mockManagedClustersClient,
			s.mockUserAssignedIdentitiesClient,
			s.mockFederatedIdentityCredentialsClient,
			s.mockRoleDefinitionsClient,
			s.mockRoleAssignmentsClient,
			s.mockVaultsClient,
			s.subscriptionToResourceClient,
			s.subscriptionToRoleAssignmentsClient,
		),
		sync.Mutex{},
		sync.Mutex{},
		&record.FakeRecorder{},
	}
}

var cleanupCustomRolesTestCases = []AzureCleanupCustomRolesTestCase{
	{
		Name:                "Nothing to clean up - empty list of custom roles",
		ExistingRoles:       nil,
		ExistingAssignments: nil,
		CleanupRole:         nil,
		CleanupAssignment:   nil,
	},
	{
		Name: "Nothing to clean up - all custom roles are in use",
		ExistingRoles: []*armauthorization.RoleDefinition{
			{
				ID:   to.Ptr("definition-1"),
				Name: to.Ptr("otterizeCustomRole"),
				Properties: &armauthorization.RoleDefinitionProperties{
					Permissions: []*armauthorization.Permission{
						{
							Actions: []*string{
								to.Ptr("Microsoft.Storage/storageAccounts/blobServices/containers/read"),
							},
						},
					},
					Description: lo.ToPtr(azureagent.OtterizeCustomRoleTag),
					RoleType:    lo.ToPtr(azureagent.AzureCustomRole),
				},
			},
		},
		ExistingAssignments: []*armauthorization.RoleAssignment{
			{
				ID:   to.Ptr("1"),
				Name: to.Ptr("otterizeCustomRole"),
				Properties: &armauthorization.RoleAssignmentProperties{
					Scope:            to.Ptr("/subscriptions/scope"),
					Description:      lo.ToPtr(azureagent.OtterizeRoleAssignmentTag),
					RoleDefinitionID: to.Ptr("definition-1"),
				},
			},
		},
		CleanupRole:       nil,
		CleanupAssignment: nil,
	},
	{
		Name:          "Cleanup role assignment - no connected custom role",
		ExistingRoles: []*armauthorization.RoleDefinition{},
		ExistingAssignments: []*armauthorization.RoleAssignment{
			{
				ID:   to.Ptr("assignment-1"),
				Name: to.Ptr("otterizeCustomRole"),
				Properties: &armauthorization.RoleAssignmentProperties{
					Scope:            to.Ptr("/subscriptions/scope/storage/container"),
					Description:      lo.ToPtr(azureagent.OtterizeRoleAssignmentTag),
					RoleDefinitionID: to.Ptr("/subscriptions/scope/role/definition-1"),
				},
			},
		},
		CleanupRole:       lo.ToPtr("definition-1"),
		CleanupAssignment: lo.ToPtr("/subscriptions/scope/storage/container"),
	},
	{
		Name: "Cleanup custom role - no assignment found",
		ExistingRoles: []*armauthorization.RoleDefinition{
			{
				ID:   to.Ptr("/subscriptions/scope/role/definition-1"),
				Name: to.Ptr("otterizeCustomRole"),
				Properties: &armauthorization.RoleDefinitionProperties{
					Permissions: []*armauthorization.Permission{
						{
							Actions: []*string{
								to.Ptr("Microsoft.Storage/storageAccounts/blobServices/containers/read"),
							},
						},
					},
					Description: lo.ToPtr(azureagent.OtterizeCustomRoleTag),
					RoleType:    lo.ToPtr(azureagent.AzureCustomRole),
				},
			},
		},
		ExistingAssignments: []*armauthorization.RoleAssignment{},
		CleanupRole:         lo.ToPtr("definition-1"),
		CleanupAssignment:   nil,
	},
}

func TestAzureAgentPeriodicTasksSuiteSuite(t *testing.T) {
	suite.Run(t, new(AzureAgentPeriodicTasksSuite))
}
