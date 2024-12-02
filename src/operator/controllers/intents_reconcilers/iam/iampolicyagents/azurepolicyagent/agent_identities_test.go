package azurepolicyagent

import (
	"context"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/google/uuid"
	"github.com/otterize/intents-operator/src/shared/azureagent"
	mock_azureagent "github.com/otterize/intents-operator/src/shared/azureagent/mocks"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"sync"
	"testing"
)

type AzureAgentIdentitiesSuite struct {
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

func (s *AzureAgentIdentitiesSuite) SetupTest() {
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

func (s *AzureAgentIdentitiesSuite) expectListRoleAssignmentsReturnsAssignments(assignments []*armauthorization.RoleAssignment) {
	s.mockRoleAssignmentsClient.EXPECT().NewListForSubscriptionPager(nil).Return(azureagent.NewListPager[armauthorization.RoleAssignmentsClientListForSubscriptionResponse](
		armauthorization.RoleAssignmentsClientListForSubscriptionResponse{
			RoleAssignmentListResult: armauthorization.RoleAssignmentListResult{
				Value: assignments,
			},
		},
	))
}

func (s *AzureAgentIdentitiesSuite) expectDeleteRoleAssignmentSuccess(scope string) {
	s.mockRoleAssignmentsClient.EXPECT().Delete(gomock.Any(), scope, gomock.Any(), gomock.Any()).Return(
		armauthorization.RoleAssignmentsClientDeleteResponse{}, nil,
	)
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

func (s *AzureAgentIdentitiesSuite) TestDeleteUserAssignedIdentityWithRoles() {
	clientId := uuid.NewString()
	scope := "scope-1"
	s.expectGetUserAssignedIdentityReturnsClientID(clientId)

	// 1 role assigned to the identity
	s.expectListRoleAssignmentsReturnsAssignments([]*armauthorization.RoleAssignment{
		{
			ID:   to.Ptr("role-assignment-1"),
			Name: to.Ptr("role-assignment-1"),
			Properties: &armauthorization.RoleAssignmentProperties{
				PrincipalID: to.Ptr(clientId),
				Scope:       &scope,
			},
		},
	})

	s.expectDeleteRoleAssignmentSuccess(scope)
	s.expectDeleteFederatedIdentityCredentialsSuccess()
	s.expectDeleteUserAssignedIdentitiesSuccess()

	err := s.agent.DeleteUserAssignedIdentity(context.Background(), testNamespace, testIntentsServiceName)
	s.NoError(err)
}

func TestAzureAgentIdentitiesSuite(t *testing.T) {
	suite.Run(t, new(AzureAgentIdentitiesSuite))
}
