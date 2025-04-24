package azurepolicyagent

import (
	"context"
	"fmt"
	armerrros "github.com/Azure/azure-sdk-for-go-extensions/pkg/errors"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/google/uuid"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/agentutils"
	"github.com/otterize/intents-operator/src/shared/azureagent"
	mock_azureagent "github.com/otterize/intents-operator/src/shared/azureagent/mocks"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sync"
	"testing"
)

type AzureCustomRoleTestCase struct {
	Name                   string
	Roles                  []string
	Actions                []otterizev2alpha1.AzureAction
	DataActions            []otterizev2alpha1.AzureDataAction
	ExistingRoles          []*armauthorization.RoleDefinition
	UpdateExpected         bool
	ShouldCreateAssignment bool
}

type AzureAgentPoliciesCustomRolesSuite struct {
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

func (s *AzureAgentPoliciesCustomRolesSuite) expectGetByIDReturnsResource(scope string) {
	s.mockProviderResourceTypesClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(armresources.ProviderResourceTypesClientListResponse{
		ProviderResourceTypeListResult: armresources.ProviderResourceTypeListResult{
			Value: []*armresources.ProviderResourceType{
				{
					ResourceType:      to.Ptr("storageAccounts"),
					DefaultAPIVersion: lo.ToPtr("2022-09-01"),
				},
			},
		},
	}, nil)
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

func (s *AzureAgentPoliciesCustomRolesSuite) expectListRoleAssignmentReturnsPager(assignments []*armauthorization.RoleAssignment) {
	s.mockRoleAssignmentsClient.EXPECT().NewListForSubscriptionPager(gomock.Any()).Return(azureagent.NewListPager[armauthorization.RoleAssignmentsClientListForSubscriptionResponse](
		armauthorization.RoleAssignmentsClientListForSubscriptionResponse{
			RoleAssignmentListResult: armauthorization.RoleAssignmentListResult{
				Value: assignments,
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

func (s *AzureAgentPoliciesCustomRolesSuite) expectGetUserAssignedIdentity(clientId, principalId string) {
	userAssignedIndentityName := s.agent.GenerateUserAssignedIdentityName(testNamespace, testIntentsServiceName)
	s.mockUserAssignedIdentitiesClient.EXPECT().Get(gomock.Any(), testResourceGroup, userAssignedIndentityName, nil).Return(
		armmsi.UserAssignedIdentitiesClientGetResponse{
			Identity: armmsi.Identity{
				Name: &userAssignedIndentityName,
				Properties: &armmsi.UserAssignedIdentityProperties{
					ClientID:    &clientId,
					PrincipalID: &principalId,
				},
			},
		}, nil)
}

func (s *AzureAgentPoliciesCustomRolesSuite) expectGetUserAssignedIdentityReturnsNotFoundError() {
	notFoundError := &azcore.ResponseError{ErrorCode: armerrros.ResourceNotFound}
	s.mockUserAssignedIdentitiesClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), nil).Return(
		armmsi.UserAssignedIdentitiesClientGetResponse{},
		notFoundError,
	)
}

func (s *AzureAgentPoliciesCustomRolesSuite) expectListKeyVaultsReturnsEmpty() {
	s.mockVaultsClient.EXPECT().NewListByResourceGroupPager(testResourceGroup, nil).Return(azureagent.NewListPager[armkeyvault.VaultsClientListByResourceGroupResponse](
		armkeyvault.VaultsClientListByResourceGroupResponse{},
	))
}

func (s *AzureAgentPoliciesCustomRolesSuite) SetupTest() {
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
		ExistingRoles:          nil,
		UpdateExpected:         true,
		ShouldCreateAssignment: true,
	},
	{
		Name: "UpdatesExistingPolicy",
		Actions: []otterizev2alpha1.AzureAction{
			"Microsoft.Storage/storageAccounts/blobServices/containers/read",
			"Microsoft.Storage/storageAccounts/blobServices/containers/write",
		},
		ExistingRoles: []*armauthorization.RoleDefinition{
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
		ExistingRoles: []*armauthorization.RoleDefinition{
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
		targetScope := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/test/blobServices/default/containers/container", testSubscriptionID, testResourceGroup)

		s.Run(testCase.Name, func() {
			targets := []otterizev2alpha1.Target{
				{
					Azure: &otterizev2alpha1.AzureTarget{
						Scope:       targetScope,
						Roles:       testCase.Roles,
						Actions:     testCase.Actions,
						DataActions: testCase.DataActions,
					},
				},
			}

			clientIntents := otterizev2alpha1.ClientIntents{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testIntentsServiceName,
					Namespace: testNamespace,
				},
				Spec: &otterizev2alpha1.IntentsSpec{
					Workload: otterizev2alpha1.Workload{Name: testIntentsServiceName},
					Targets:  targets,
				},
			}

			clientId := uuid.NewString()
			s.expectGetUserAssignedIdentity(clientId, "")

			s.expectListKeyVaultsReturnsEmpty()

			// Two calls - one from custom roles and one from backwards compatibility to built-in roles
			s.expectListSubscriptionsReturnsPager()
			s.expectListSubscriptionsReturnsPager()
			s.expectListRoleAssignmentsReturnsEmpty()
			s.expectListRoleAssignmentsReturnsEmpty()

			// CustomRole related calls
			s.expectListRoleDefinitionsReturnsPager(testCase.ExistingRoles)
			if testCase.ShouldCreateAssignment {
				s.expectCreateRoleAssignmentReturnsEmpty()
			}

			if testCase.ExistingRoles == nil {
				s.expectGetByIDReturnsResource(targetScope)
			}

			// Make sure the custom role is created
			var customRoleDefinition armauthorization.RoleDefinition
			if testCase.UpdateExpected {
				s.expectCreateOrUpdateRoleDefinitionWriteRoleDefinition(&customRoleDefinition)
			}

			err := s.agent.AddRolePolicyFromIntents(context.Background(), testNamespace, testAccountName, testIntentsServiceName, clientIntents, clientIntents.GetTargetList(), corev1.Pod{})
			s.Require().NoError(err)

			if testCase.UpdateExpected {
				s.Require().Len(customRoleDefinition.Properties.Permissions, 1)
				s.Require().Len(customRoleDefinition.Properties.Permissions[0].Actions, len(testCase.Actions))
				s.Require().Len(customRoleDefinition.Properties.Permissions[0].DataActions, len(testCase.DataActions))
			}
		})
	}
}

func (s *AzureAgentPoliciesCustomRolesSuite) TestAddRolePolicyWithExistingRolesNotTryingToRecreateAssignments() {
	targetScope := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/test/blobServices/default/containers/container", testSubscriptionID, testResourceGroup)
	targets := []otterizev2alpha1.Target{
		{
			Azure: &otterizev2alpha1.AzureTarget{
				Scope: targetScope,
				Roles: []string{"Storage Blob Data Reader"},
			},
		},
	}

	clientIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testIntentsServiceName,
			Namespace: testNamespace,
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{Name: testIntentsServiceName},
			Targets:  targets,
		},
	}
	existingRoles := []*armauthorization.RoleDefinition{
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
	}

	clientId := uuid.NewString()
	s.expectGetUserAssignedIdentity(clientId, "")

	s.expectListKeyVaultsReturnsEmpty()

	// Two calls - one from custom roles and one from backwards compatibility to built-in roles
	s.expectListSubscriptionsReturnsPager()
	s.expectListSubscriptionsReturnsPager()
	s.expectListRoleAssignmentsReturnsEmpty()
	s.expectListRoleAssignmentsReturnsEmpty()
	s.expectListRoleDefinitionsReturnsPager(existingRoles) // Pager should return the roles from the intents
	s.expectCreateRoleAssignmentReturnsEmpty()
	// We expect s.mockRoleDefinitionsClient.CreateOrUpdate() to not be called (hence .Times(0))
	s.mockRoleDefinitionsClient.EXPECT().CreateOrUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err := s.agent.AddRolePolicyFromIntents(context.Background(), testNamespace, testAccountName, testIntentsServiceName, clientIntents, clientIntents.GetTargetList(), corev1.Pod{})
	s.Require().NoError(err)

	// All expectations should be called again
	s.expectGetUserAssignedIdentity(clientId, "")
	s.expectListKeyVaultsReturnsEmpty()
	s.expectListSubscriptionsReturnsPager()
	s.expectListSubscriptionsReturnsPager()
	s.expectListRoleAssignmentsReturnsEmpty()
	s.expectListRoleAssignmentsReturnsEmpty()
	s.expectListRoleDefinitionsReturnsPager(existingRoles) // Pager should return the roles from the intents
	s.expectCreateRoleAssignmentReturnsEmpty()

	err = s.agent.AddRolePolicyFromIntents(context.Background(), testNamespace, testAccountName, testIntentsServiceName, clientIntents, clientIntents.GetTargetList(), corev1.Pod{})
	s.Require().NoError(err)
}

func (s *AzureAgentPoliciesCustomRolesSuite) TestAddRolePolicyWithMadeUpRoleRaisesEventButDoesntError() {
	targetScope := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/test/blobServices/default/containers/container", testSubscriptionID, testResourceGroup)
	targets := []otterizev2alpha1.Target{
		{
			Azure: &otterizev2alpha1.AzureTarget{
				Scope: targetScope,
				Roles: []string{"Role That Does Not Exist"},
			},
		},
	}

	clientIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testIntentsServiceName,
			Namespace: testNamespace,
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{Name: testIntentsServiceName},
			Targets:  targets,
		},
	}
	existingRoles := []*armauthorization.RoleDefinition{
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
	}

	clientId := uuid.NewString()
	s.expectGetUserAssignedIdentity(clientId, "")
	s.expectListKeyVaultsReturnsEmpty()
	// Two calls - one from custom roles and one from backwards compatibility to built-in roles
	s.expectListSubscriptionsReturnsPager()
	s.expectListSubscriptionsReturnsPager()
	s.expectListRoleAssignmentsReturnsEmpty()
	s.expectListRoleAssignmentsReturnsEmpty()
	s.expectListRoleDefinitionsReturnsPager(existingRoles) // Pager should return the roles from the intents

	err := s.agent.AddRolePolicyFromIntents(context.Background(), testNamespace, testAccountName, testIntentsServiceName, clientIntents, clientIntents.GetTargetList(), corev1.Pod{})
	s.Require().NoError(err)

	select {
	case event := <-s.testRecoder.Events:
		s.Require().Contains(event, "RoleNotFound")
	default:
		s.Fail("event not raised")
	}
}

func (s *AzureAgentPoliciesCustomRolesSuite) TestAddRolePolicyFromIntents_IdentityNotFound() {
	s.expectGetUserAssignedIdentityReturnsNotFoundError()

	clientIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testIntentsServiceName,
			Namespace: testNamespace,
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{Name: testIntentsServiceName},
		},
	}

	err := s.agent.AddRolePolicyFromIntents(context.Background(), testNamespace, testAccountName, testIntentsServiceName, clientIntents, clientIntents.GetTargetList(), corev1.Pod{})
	s.Require().ErrorIs(err, agentutils.ErrCloudIdentityNotFound)
}

func (s *AzureAgentPoliciesCustomRolesSuite) TestDeleteRolePolicyFromIntents_IdentityNotFound() {
	s.expectGetUserAssignedIdentityReturnsNotFoundError()
	intent := otterizev2alpha1.ClientIntents{Spec: &otterizev2alpha1.IntentsSpec{Workload: otterizev2alpha1.Workload{Name: testIntentsServiceName}}}
	err := s.agent.DeleteRolePolicyFromIntents(context.Background(), intent)
	s.Require().ErrorIs(err, agentutils.ErrCloudIdentityNotFound)
}

func (s *AzureAgentPoliciesCustomRolesSuite) TestDeleteCustomRoles() {
	targetScope := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/test/blobServices/default/containers/container", testSubscriptionID, testResourceGroup)
	targets := []otterizev2alpha1.Target{
		{
			Azure: &otterizev2alpha1.AzureTarget{
				Scope: targetScope,
				DataActions: []otterizev2alpha1.AzureDataAction{
					"Microsoft.Storage/storageAccounts/blobServices/containers/blobs/read",
				},
			},
		},
	}

	clientIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testIntentsServiceName,
			Namespace: testNamespace,
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{Name: testIntentsServiceName},
			Targets:  targets,
		},
	}
	clientId := uuid.NewString()
	principalId := uuid.NewString()
	roleDefId := uuid.NewString()

	customRole := &armauthorization.RoleDefinition{
		Name: to.Ptr("otterizeCustomRole"),
		ID:   to.Ptr(roleDefId),
		Properties: &armauthorization.RoleDefinitionProperties{
			RoleName:    to.Ptr(s.agent.GenerateCustomRoleName(clientIntents, targetScope)),
			Description: to.Ptr(azureagent.OtterizeRoleAssignmentTag),
		},
	}

	roleAssignment := &armauthorization.RoleAssignment{
		Name: to.Ptr("roleAssignmentName"),
		Properties: &armauthorization.RoleAssignmentProperties{
			RoleDefinitionID: to.Ptr("roleDefinitionID"),
			PrincipalID:      to.Ptr(principalId),
			PrincipalType:    to.Ptr(armauthorization.PrincipalTypeServicePrincipal),
			Scope:            to.Ptr(targetScope),
		},
	}
	s.expectGetUserAssignedIdentity(clientId, principalId)
	s.expectListKeyVaultsReturnsEmpty()
	s.expectListSubscriptionsReturnsPager()
	s.expectListRoleDefinitionsReturnsPager([]*armauthorization.RoleDefinition{customRole})
	s.expectListRoleAssignmentReturnsPager([]*armauthorization.RoleAssignment{roleAssignment})

	// Expect deletion of assignment
	s.mockRoleAssignmentsClient.EXPECT().Delete(context.Background(), targetScope, *roleAssignment.Name, nil)

	// Expect deletion of custom role
	subscriptionScope := s.agent.GetSubscriptionScope(targetScope)
	s.mockRoleDefinitionsClient.EXPECT().Delete(context.Background(), subscriptionScope, *customRole.Name, nil)

	err := s.agent.DeleteRolePolicyFromIntents(context.Background(), clientIntents)
	s.Require().NoError(err)
}

func TestAzureAgentPoliciesCustomRolesSuite(t *testing.T) {
	suite.Run(t, new(AzureAgentPoliciesCustomRolesSuite))
}
