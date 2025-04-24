package azurepolicyagent

import (
	"context"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/google/uuid"
	"github.com/otterize/intents-operator/src/shared/azureagent"
	mock_azureagent "github.com/otterize/intents-operator/src/shared/azureagent/mocks"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"k8s.io/client-go/tools/record"
	"sync"
	"testing"
)

type AzureAgentIdentitiesSuite struct {
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

	testRecoder record.FakeRecorder
	agent       *Agent
}

func (s *AzureAgentIdentitiesSuite) SetupTest() {
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

func (s *AzureAgentIdentitiesSuite) expectGetUserAssignedIdentityReturnsClientID(clientId string) {
	userAssignedIndentityName := s.agent.GenerateUserAssignedIdentityName(testNamespace, testIntentsServiceName)
	s.mockUserAssignedIdentitiesClient.EXPECT().Get(gomock.Any(), testResourceGroup, userAssignedIndentityName, nil).Return(
		armmsi.UserAssignedIdentitiesClientGetResponse{
			Identity: armmsi.Identity{
				Name: &userAssignedIndentityName,
				Properties: &armmsi.UserAssignedIdentityProperties{
					ClientID:    &clientId,
					PrincipalID: &clientId,
				},
			},
		}, nil)
}

func (s *AzureAgentIdentitiesSuite) expectDeleteFederatedIdentityCredentialsSuccess() {
	userAssignedIndentityName := s.agent.GenerateUserAssignedIdentityName(testNamespace, testIntentsServiceName)
	s.mockFederatedIdentityCredentialsClient.EXPECT().Delete(gomock.Any(), testResourceGroup, userAssignedIndentityName, gomock.Any(), gomock.Any()).Return(
		armmsi.FederatedIdentityCredentialsClientDeleteResponse{}, nil,
	)
}

func (s *AzureAgentIdentitiesSuite) expectDeleteUserAssignedIdentitiesSuccess() {
	userAssignedIndentityName := s.agent.GenerateUserAssignedIdentityName(testNamespace, testIntentsServiceName)
	s.mockUserAssignedIdentitiesClient.EXPECT().Delete(gomock.Any(), testResourceGroup, userAssignedIndentityName, gomock.Any()).Return(
		armmsi.UserAssignedIdentitiesClientDeleteResponse{}, nil,
	)
}

func (s *AzureAgentIdentitiesSuite) TestDeleteUserAssignedIdentity() {
	clientId := uuid.NewString()
	s.expectGetUserAssignedIdentityReturnsClientID(clientId)
	s.expectDeleteFederatedIdentityCredentialsSuccess()
	s.expectDeleteUserAssignedIdentitiesSuccess()

	err := s.agent.DeleteUserAssignedIdentity(context.Background(), testNamespace, testIntentsServiceName)
	s.NoError(err)
}

func TestAzureAgentIdentitiesSuite(t *testing.T) {
	suite.Run(t, new(AzureAgentIdentitiesSuite))
}
