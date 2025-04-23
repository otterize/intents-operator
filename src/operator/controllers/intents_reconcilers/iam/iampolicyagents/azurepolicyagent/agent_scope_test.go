package azurepolicyagent

import (
	"context"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/azureagent"
	mock_azureagent "github.com/otterize/intents-operator/src/shared/azureagent/mocks"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"k8s.io/client-go/tools/record"
	"sync"
	"testing"
)

type AzureAgentScopeSuite struct {
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
	testRecoder                         record.FakeRecorder
	agent                               *Agent
}

func (s *AzureAgentScopeSuite) SetupTest() {
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
		Agent: azureagent.NewAzureAgentFromClients(
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
		roleMutex:       sync.Mutex{},
		assignmentMutex: sync.Mutex{},
	}
	s.testRecoder = *record.NewFakeRecorder(100)
	s.agent.InjectRecorder(&s.testRecoder)
}

func (s *AzureAgentScopeSuite) expectListSubscriptionsReturnsPager() {
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

func (s *AzureAgentScopeSuite) expectListResourcesReturnsPager(resource string) {
	s.mockResourcesClient.EXPECT().NewListPager(gomock.Any()).Return(azureagent.NewListPager[armresources.ClientListResponse](
		armresources.ClientListResponse{
			ResourceListResult: armresources.ResourceListResult{
				Value: []*armresources.GenericResourceExpanded{
					{
						ID: lo.ToPtr(resource),
					},
				},
			},
		},
	))
}

func (s *AzureAgentScopeSuite) TestGetIntentScopeFullScope() {
	// Arrange
	targetScope := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/test/blobServices/default/containers/container", testSubscriptionID, testResourceGroup)
	intent := otterizev2alpha1.Target{
		Azure: &otterizev2alpha1.AzureTarget{
			Scope: targetScope,
		},
	}

	// Act
	scope, err := s.agent.getIntentScope(context.Background(), intent)

	// Assert
	s.Require().NoError(err)
	s.Equal(scope, targetScope)
}

func (s *AzureAgentScopeSuite) TestGetIntentScopePartialScope() {
	// Arrange
	partialScope := "/providers/Microsoft.Storage/storageAccounts/test/blobServices/default/containers/container"
	storageAccountScope := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/test", testSubscriptionID, testResourceGroup)
	expectedScope := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s%s", testSubscriptionID, testResourceGroup, partialScope)

	intent := otterizev2alpha1.Target{
		Azure: &otterizev2alpha1.AzureTarget{
			Scope: partialScope,
		},
	}

	s.expectListSubscriptionsReturnsPager()
	s.expectListResourcesReturnsPager(storageAccountScope)

	// Act
	scope, err := s.agent.getIntentScope(context.Background(), intent)

	// Assert
	s.Require().NoError(err)
	s.Equal(scope, expectedScope)
}

func (s *AzureAgentScopeSuite) TestGetIntentScopeInvalidScope() {
	// Arrange
	intent := otterizev2alpha1.Target{
		Azure: &otterizev2alpha1.AzureTarget{
			Scope: "invalid-scope",
		},
	}

	// Act
	scope, err := s.agent.getIntentScope(context.Background(), intent)

	// Assert
	s.Require().Error(err)
	s.Empty(scope)
}

func (s *AzureAgentScopeSuite) TestGetIntentScopeNotFoundPartialScope() {
	// Arrange
	targetScope := "/providers/Microsoft.Storage/storageAccounts/test/blobServices/default/containers/container"
	intent := otterizev2alpha1.Target{
		Azure: &otterizev2alpha1.AzureTarget{
			Scope: targetScope,
		},
	}

	s.expectListSubscriptionsReturnsPager()
	s.expectListResourcesReturnsPager("")

	// Act
	scope, err := s.agent.getIntentScope(context.Background(), intent)

	// Assert
	s.Require().Error(err)
	s.ErrorContains(err, "resource not found")
	s.Empty(scope)
}

func TestAzureAgentScopeSuite(t *testing.T) {
	suite.Run(t, new(AzureAgentScopeSuite))
}
