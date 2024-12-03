// Code generated by MockGen. DO NOT EDIT.
// Source: azureclients.go

// Package mock_azureagent is a generated GoMock package.
package mock_azureagent

import (
	context "context"
	reflect "reflect"

	runtime "github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	armauthorization "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	armcontainerservice "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	armkeyvault "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	armmsi "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	armresources "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	armsubscriptions "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	gomock "go.uber.org/mock/gomock"
)

// MockAzureARMResourcesClient is a mock of AzureARMResourcesClient interface.
type MockAzureARMResourcesClient struct {
	ctrl     *gomock.Controller
	recorder *MockAzureARMResourcesClientMockRecorder
}

// MockAzureARMResourcesClientMockRecorder is the mock recorder for MockAzureARMResourcesClient.
type MockAzureARMResourcesClientMockRecorder struct {
	mock *MockAzureARMResourcesClient
}

// NewMockAzureARMResourcesClient creates a new mock instance.
func NewMockAzureARMResourcesClient(ctrl *gomock.Controller) *MockAzureARMResourcesClient {
	mock := &MockAzureARMResourcesClient{ctrl: ctrl}
	mock.recorder = &MockAzureARMResourcesClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockAzureARMResourcesClient) EXPECT() *MockAzureARMResourcesClientMockRecorder {
	return m.recorder
}

// GetByID mocks base method.
func (m *MockAzureARMResourcesClient) GetByID(ctx context.Context, resourceID, apiVersion string, options *armresources.ClientGetByIDOptions) (armresources.ClientGetByIDResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetByID", ctx, resourceID, apiVersion, options)
	ret0, _ := ret[0].(armresources.ClientGetByIDResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetByID indicates an expected call of GetByID.
func (mr *MockAzureARMResourcesClientMockRecorder) GetByID(ctx, resourceID, apiVersion, options interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetByID", reflect.TypeOf((*MockAzureARMResourcesClient)(nil).GetByID), ctx, resourceID, apiVersion, options)
}

// MockAzureARMSubscriptionsClient is a mock of AzureARMSubscriptionsClient interface.
type MockAzureARMSubscriptionsClient struct {
	ctrl     *gomock.Controller
	recorder *MockAzureARMSubscriptionsClientMockRecorder
}

// MockAzureARMSubscriptionsClientMockRecorder is the mock recorder for MockAzureARMSubscriptionsClient.
type MockAzureARMSubscriptionsClientMockRecorder struct {
	mock *MockAzureARMSubscriptionsClient
}

// NewMockAzureARMSubscriptionsClient creates a new mock instance.
func NewMockAzureARMSubscriptionsClient(ctrl *gomock.Controller) *MockAzureARMSubscriptionsClient {
	mock := &MockAzureARMSubscriptionsClient{ctrl: ctrl}
	mock.recorder = &MockAzureARMSubscriptionsClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockAzureARMSubscriptionsClient) EXPECT() *MockAzureARMSubscriptionsClientMockRecorder {
	return m.recorder
}

// Get mocks base method.
func (m *MockAzureARMSubscriptionsClient) Get(ctx context.Context, subscriptionID string, options *armsubscriptions.ClientGetOptions) (armsubscriptions.ClientGetResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", ctx, subscriptionID, options)
	ret0, _ := ret[0].(armsubscriptions.ClientGetResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Get indicates an expected call of Get.
func (mr *MockAzureARMSubscriptionsClientMockRecorder) Get(ctx, subscriptionID, options interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockAzureARMSubscriptionsClient)(nil).Get), ctx, subscriptionID, options)
}

// MockAzureARMResourcesResourceGroupsClient is a mock of AzureARMResourcesResourceGroupsClient interface.
type MockAzureARMResourcesResourceGroupsClient struct {
	ctrl     *gomock.Controller
	recorder *MockAzureARMResourcesResourceGroupsClientMockRecorder
}

// MockAzureARMResourcesResourceGroupsClientMockRecorder is the mock recorder for MockAzureARMResourcesResourceGroupsClient.
type MockAzureARMResourcesResourceGroupsClientMockRecorder struct {
	mock *MockAzureARMResourcesResourceGroupsClient
}

// NewMockAzureARMResourcesResourceGroupsClient creates a new mock instance.
func NewMockAzureARMResourcesResourceGroupsClient(ctrl *gomock.Controller) *MockAzureARMResourcesResourceGroupsClient {
	mock := &MockAzureARMResourcesResourceGroupsClient{ctrl: ctrl}
	mock.recorder = &MockAzureARMResourcesResourceGroupsClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockAzureARMResourcesResourceGroupsClient) EXPECT() *MockAzureARMResourcesResourceGroupsClientMockRecorder {
	return m.recorder
}

// Get mocks base method.
func (m *MockAzureARMResourcesResourceGroupsClient) Get(ctx context.Context, resourceGroupName string, options *armresources.ResourceGroupsClientGetOptions) (armresources.ResourceGroupsClientGetResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", ctx, resourceGroupName, options)
	ret0, _ := ret[0].(armresources.ResourceGroupsClientGetResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Get indicates an expected call of Get.
func (mr *MockAzureARMResourcesResourceGroupsClientMockRecorder) Get(ctx, resourceGroupName, options interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockAzureARMResourcesResourceGroupsClient)(nil).Get), ctx, resourceGroupName, options)
}

// MockAzureARMContainerServiceManagedClustersClient is a mock of AzureARMContainerServiceManagedClustersClient interface.
type MockAzureARMContainerServiceManagedClustersClient struct {
	ctrl     *gomock.Controller
	recorder *MockAzureARMContainerServiceManagedClustersClientMockRecorder
}

// MockAzureARMContainerServiceManagedClustersClientMockRecorder is the mock recorder for MockAzureARMContainerServiceManagedClustersClient.
type MockAzureARMContainerServiceManagedClustersClientMockRecorder struct {
	mock *MockAzureARMContainerServiceManagedClustersClient
}

// NewMockAzureARMContainerServiceManagedClustersClient creates a new mock instance.
func NewMockAzureARMContainerServiceManagedClustersClient(ctrl *gomock.Controller) *MockAzureARMContainerServiceManagedClustersClient {
	mock := &MockAzureARMContainerServiceManagedClustersClient{ctrl: ctrl}
	mock.recorder = &MockAzureARMContainerServiceManagedClustersClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockAzureARMContainerServiceManagedClustersClient) EXPECT() *MockAzureARMContainerServiceManagedClustersClientMockRecorder {
	return m.recorder
}

// Get mocks base method.
func (m *MockAzureARMContainerServiceManagedClustersClient) Get(ctx context.Context, resourceGroupName, resourceName string, options *armcontainerservice.ManagedClustersClientGetOptions) (armcontainerservice.ManagedClustersClientGetResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", ctx, resourceGroupName, resourceName, options)
	ret0, _ := ret[0].(armcontainerservice.ManagedClustersClientGetResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Get indicates an expected call of Get.
func (mr *MockAzureARMContainerServiceManagedClustersClientMockRecorder) Get(ctx, resourceGroupName, resourceName, options interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockAzureARMContainerServiceManagedClustersClient)(nil).Get), ctx, resourceGroupName, resourceName, options)
}

// MockAzureARMMSIUserAssignedIdentitiesClient is a mock of AzureARMMSIUserAssignedIdentitiesClient interface.
type MockAzureARMMSIUserAssignedIdentitiesClient struct {
	ctrl     *gomock.Controller
	recorder *MockAzureARMMSIUserAssignedIdentitiesClientMockRecorder
}

// MockAzureARMMSIUserAssignedIdentitiesClientMockRecorder is the mock recorder for MockAzureARMMSIUserAssignedIdentitiesClient.
type MockAzureARMMSIUserAssignedIdentitiesClientMockRecorder struct {
	mock *MockAzureARMMSIUserAssignedIdentitiesClient
}

// NewMockAzureARMMSIUserAssignedIdentitiesClient creates a new mock instance.
func NewMockAzureARMMSIUserAssignedIdentitiesClient(ctrl *gomock.Controller) *MockAzureARMMSIUserAssignedIdentitiesClient {
	mock := &MockAzureARMMSIUserAssignedIdentitiesClient{ctrl: ctrl}
	mock.recorder = &MockAzureARMMSIUserAssignedIdentitiesClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockAzureARMMSIUserAssignedIdentitiesClient) EXPECT() *MockAzureARMMSIUserAssignedIdentitiesClientMockRecorder {
	return m.recorder
}

// CreateOrUpdate mocks base method.
func (m *MockAzureARMMSIUserAssignedIdentitiesClient) CreateOrUpdate(ctx context.Context, resourceGroupName, resourceName string, parameters armmsi.Identity, options *armmsi.UserAssignedIdentitiesClientCreateOrUpdateOptions) (armmsi.UserAssignedIdentitiesClientCreateOrUpdateResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateOrUpdate", ctx, resourceGroupName, resourceName, parameters, options)
	ret0, _ := ret[0].(armmsi.UserAssignedIdentitiesClientCreateOrUpdateResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateOrUpdate indicates an expected call of CreateOrUpdate.
func (mr *MockAzureARMMSIUserAssignedIdentitiesClientMockRecorder) CreateOrUpdate(ctx, resourceGroupName, resourceName, parameters, options interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateOrUpdate", reflect.TypeOf((*MockAzureARMMSIUserAssignedIdentitiesClient)(nil).CreateOrUpdate), ctx, resourceGroupName, resourceName, parameters, options)
}

// Delete mocks base method.
func (m *MockAzureARMMSIUserAssignedIdentitiesClient) Delete(ctx context.Context, resourceGroupName, resourceName string, options *armmsi.UserAssignedIdentitiesClientDeleteOptions) (armmsi.UserAssignedIdentitiesClientDeleteResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Delete", ctx, resourceGroupName, resourceName, options)
	ret0, _ := ret[0].(armmsi.UserAssignedIdentitiesClientDeleteResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Delete indicates an expected call of Delete.
func (mr *MockAzureARMMSIUserAssignedIdentitiesClientMockRecorder) Delete(ctx, resourceGroupName, resourceName, options interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Delete", reflect.TypeOf((*MockAzureARMMSIUserAssignedIdentitiesClient)(nil).Delete), ctx, resourceGroupName, resourceName, options)
}

// Get mocks base method.
func (m *MockAzureARMMSIUserAssignedIdentitiesClient) Get(ctx context.Context, resourceGroupName, resourceName string, options *armmsi.UserAssignedIdentitiesClientGetOptions) (armmsi.UserAssignedIdentitiesClientGetResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", ctx, resourceGroupName, resourceName, options)
	ret0, _ := ret[0].(armmsi.UserAssignedIdentitiesClientGetResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Get indicates an expected call of Get.
func (mr *MockAzureARMMSIUserAssignedIdentitiesClientMockRecorder) Get(ctx, resourceGroupName, resourceName, options interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockAzureARMMSIUserAssignedIdentitiesClient)(nil).Get), ctx, resourceGroupName, resourceName, options)
}

// MockAzureARMMSIFederatedIdentityCredentialsClient is a mock of AzureARMMSIFederatedIdentityCredentialsClient interface.
type MockAzureARMMSIFederatedIdentityCredentialsClient struct {
	ctrl     *gomock.Controller
	recorder *MockAzureARMMSIFederatedIdentityCredentialsClientMockRecorder
}

// MockAzureARMMSIFederatedIdentityCredentialsClientMockRecorder is the mock recorder for MockAzureARMMSIFederatedIdentityCredentialsClient.
type MockAzureARMMSIFederatedIdentityCredentialsClientMockRecorder struct {
	mock *MockAzureARMMSIFederatedIdentityCredentialsClient
}

// NewMockAzureARMMSIFederatedIdentityCredentialsClient creates a new mock instance.
func NewMockAzureARMMSIFederatedIdentityCredentialsClient(ctrl *gomock.Controller) *MockAzureARMMSIFederatedIdentityCredentialsClient {
	mock := &MockAzureARMMSIFederatedIdentityCredentialsClient{ctrl: ctrl}
	mock.recorder = &MockAzureARMMSIFederatedIdentityCredentialsClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockAzureARMMSIFederatedIdentityCredentialsClient) EXPECT() *MockAzureARMMSIFederatedIdentityCredentialsClientMockRecorder {
	return m.recorder
}

// CreateOrUpdate mocks base method.
func (m *MockAzureARMMSIFederatedIdentityCredentialsClient) CreateOrUpdate(ctx context.Context, resourceGroupName, resourceName, federatedIdentityCredentialResourceName string, parameters armmsi.FederatedIdentityCredential, options *armmsi.FederatedIdentityCredentialsClientCreateOrUpdateOptions) (armmsi.FederatedIdentityCredentialsClientCreateOrUpdateResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateOrUpdate", ctx, resourceGroupName, resourceName, federatedIdentityCredentialResourceName, parameters, options)
	ret0, _ := ret[0].(armmsi.FederatedIdentityCredentialsClientCreateOrUpdateResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateOrUpdate indicates an expected call of CreateOrUpdate.
func (mr *MockAzureARMMSIFederatedIdentityCredentialsClientMockRecorder) CreateOrUpdate(ctx, resourceGroupName, resourceName, federatedIdentityCredentialResourceName, parameters, options interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateOrUpdate", reflect.TypeOf((*MockAzureARMMSIFederatedIdentityCredentialsClient)(nil).CreateOrUpdate), ctx, resourceGroupName, resourceName, federatedIdentityCredentialResourceName, parameters, options)
}

// Delete mocks base method.
func (m *MockAzureARMMSIFederatedIdentityCredentialsClient) Delete(ctx context.Context, resourceGroupName, resourceName, federatedIdentityCredentialResourceName string, options *armmsi.FederatedIdentityCredentialsClientDeleteOptions) (armmsi.FederatedIdentityCredentialsClientDeleteResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Delete", ctx, resourceGroupName, resourceName, federatedIdentityCredentialResourceName, options)
	ret0, _ := ret[0].(armmsi.FederatedIdentityCredentialsClientDeleteResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Delete indicates an expected call of Delete.
func (mr *MockAzureARMMSIFederatedIdentityCredentialsClientMockRecorder) Delete(ctx, resourceGroupName, resourceName, federatedIdentityCredentialResourceName, options interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Delete", reflect.TypeOf((*MockAzureARMMSIFederatedIdentityCredentialsClient)(nil).Delete), ctx, resourceGroupName, resourceName, federatedIdentityCredentialResourceName, options)
}

// Get mocks base method.
func (m *MockAzureARMMSIFederatedIdentityCredentialsClient) Get(ctx context.Context, resourceGroupName, resourceName, federatedIdentityCredentialResourceName string, options *armmsi.FederatedIdentityCredentialsClientGetOptions) (armmsi.FederatedIdentityCredentialsClientGetResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", ctx, resourceGroupName, resourceName, federatedIdentityCredentialResourceName, options)
	ret0, _ := ret[0].(armmsi.FederatedIdentityCredentialsClientGetResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Get indicates an expected call of Get.
func (mr *MockAzureARMMSIFederatedIdentityCredentialsClientMockRecorder) Get(ctx, resourceGroupName, resourceName, federatedIdentityCredentialResourceName, options interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockAzureARMMSIFederatedIdentityCredentialsClient)(nil).Get), ctx, resourceGroupName, resourceName, federatedIdentityCredentialResourceName, options)
}

// MockAzureARMAuthorizationRoleDefinitionsClient is a mock of AzureARMAuthorizationRoleDefinitionsClient interface.
type MockAzureARMAuthorizationRoleDefinitionsClient struct {
	ctrl     *gomock.Controller
	recorder *MockAzureARMAuthorizationRoleDefinitionsClientMockRecorder
}

// MockAzureARMAuthorizationRoleDefinitionsClientMockRecorder is the mock recorder for MockAzureARMAuthorizationRoleDefinitionsClient.
type MockAzureARMAuthorizationRoleDefinitionsClientMockRecorder struct {
	mock *MockAzureARMAuthorizationRoleDefinitionsClient
}

// NewMockAzureARMAuthorizationRoleDefinitionsClient creates a new mock instance.
func NewMockAzureARMAuthorizationRoleDefinitionsClient(ctrl *gomock.Controller) *MockAzureARMAuthorizationRoleDefinitionsClient {
	mock := &MockAzureARMAuthorizationRoleDefinitionsClient{ctrl: ctrl}
	mock.recorder = &MockAzureARMAuthorizationRoleDefinitionsClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockAzureARMAuthorizationRoleDefinitionsClient) EXPECT() *MockAzureARMAuthorizationRoleDefinitionsClientMockRecorder {
	return m.recorder
}

// CreateOrUpdate mocks base method.
func (m *MockAzureARMAuthorizationRoleDefinitionsClient) CreateOrUpdate(ctx context.Context, scope, roleDefinitionID string, roleDefinition armauthorization.RoleDefinition, options *armauthorization.RoleDefinitionsClientCreateOrUpdateOptions) (armauthorization.RoleDefinitionsClientCreateOrUpdateResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateOrUpdate", ctx, scope, roleDefinitionID, roleDefinition, options)
	ret0, _ := ret[0].(armauthorization.RoleDefinitionsClientCreateOrUpdateResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateOrUpdate indicates an expected call of CreateOrUpdate.
func (mr *MockAzureARMAuthorizationRoleDefinitionsClientMockRecorder) CreateOrUpdate(ctx, scope, roleDefinitionID, roleDefinition, options interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateOrUpdate", reflect.TypeOf((*MockAzureARMAuthorizationRoleDefinitionsClient)(nil).CreateOrUpdate), ctx, scope, roleDefinitionID, roleDefinition, options)
}

// Delete mocks base method.
func (m *MockAzureARMAuthorizationRoleDefinitionsClient) Delete(ctx context.Context, scope, roleDefinitionID string, options *armauthorization.RoleDefinitionsClientDeleteOptions) (armauthorization.RoleDefinitionsClientDeleteResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Delete", ctx, scope, roleDefinitionID, options)
	ret0, _ := ret[0].(armauthorization.RoleDefinitionsClientDeleteResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Delete indicates an expected call of Delete.
func (mr *MockAzureARMAuthorizationRoleDefinitionsClientMockRecorder) Delete(ctx, scope, roleDefinitionID, options interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Delete", reflect.TypeOf((*MockAzureARMAuthorizationRoleDefinitionsClient)(nil).Delete), ctx, scope, roleDefinitionID, options)
}

// NewListPager mocks base method.
func (m *MockAzureARMAuthorizationRoleDefinitionsClient) NewListPager(scope string, options *armauthorization.RoleDefinitionsClientListOptions) *runtime.Pager[armauthorization.RoleDefinitionsClientListResponse] {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewListPager", scope, options)
	ret0, _ := ret[0].(*runtime.Pager[armauthorization.RoleDefinitionsClientListResponse])
	return ret0
}

// NewListPager indicates an expected call of NewListPager.
func (mr *MockAzureARMAuthorizationRoleDefinitionsClientMockRecorder) NewListPager(scope, options interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewListPager", reflect.TypeOf((*MockAzureARMAuthorizationRoleDefinitionsClient)(nil).NewListPager), scope, options)
}

// MockAzureARMAuthorizationRoleAssignmentsClient is a mock of AzureARMAuthorizationRoleAssignmentsClient interface.
type MockAzureARMAuthorizationRoleAssignmentsClient struct {
	ctrl     *gomock.Controller
	recorder *MockAzureARMAuthorizationRoleAssignmentsClientMockRecorder
}

// MockAzureARMAuthorizationRoleAssignmentsClientMockRecorder is the mock recorder for MockAzureARMAuthorizationRoleAssignmentsClient.
type MockAzureARMAuthorizationRoleAssignmentsClientMockRecorder struct {
	mock *MockAzureARMAuthorizationRoleAssignmentsClient
}

// NewMockAzureARMAuthorizationRoleAssignmentsClient creates a new mock instance.
func NewMockAzureARMAuthorizationRoleAssignmentsClient(ctrl *gomock.Controller) *MockAzureARMAuthorizationRoleAssignmentsClient {
	mock := &MockAzureARMAuthorizationRoleAssignmentsClient{ctrl: ctrl}
	mock.recorder = &MockAzureARMAuthorizationRoleAssignmentsClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockAzureARMAuthorizationRoleAssignmentsClient) EXPECT() *MockAzureARMAuthorizationRoleAssignmentsClientMockRecorder {
	return m.recorder
}

// Create mocks base method.
func (m *MockAzureARMAuthorizationRoleAssignmentsClient) Create(ctx context.Context, scope, roleAssignmentName string, parameters armauthorization.RoleAssignmentCreateParameters, options *armauthorization.RoleAssignmentsClientCreateOptions) (armauthorization.RoleAssignmentsClientCreateResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Create", ctx, scope, roleAssignmentName, parameters, options)
	ret0, _ := ret[0].(armauthorization.RoleAssignmentsClientCreateResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Create indicates an expected call of Create.
func (mr *MockAzureARMAuthorizationRoleAssignmentsClientMockRecorder) Create(ctx, scope, roleAssignmentName, parameters, options interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Create", reflect.TypeOf((*MockAzureARMAuthorizationRoleAssignmentsClient)(nil).Create), ctx, scope, roleAssignmentName, parameters, options)
}

// Delete mocks base method.
func (m *MockAzureARMAuthorizationRoleAssignmentsClient) Delete(ctx context.Context, scope, roleAssignmentName string, options *armauthorization.RoleAssignmentsClientDeleteOptions) (armauthorization.RoleAssignmentsClientDeleteResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Delete", ctx, scope, roleAssignmentName, options)
	ret0, _ := ret[0].(armauthorization.RoleAssignmentsClientDeleteResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Delete indicates an expected call of Delete.
func (mr *MockAzureARMAuthorizationRoleAssignmentsClientMockRecorder) Delete(ctx, scope, roleAssignmentName, options interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Delete", reflect.TypeOf((*MockAzureARMAuthorizationRoleAssignmentsClient)(nil).Delete), ctx, scope, roleAssignmentName, options)
}

// NewListForSubscriptionPager mocks base method.
func (m *MockAzureARMAuthorizationRoleAssignmentsClient) NewListForSubscriptionPager(options *armauthorization.RoleAssignmentsClientListForSubscriptionOptions) *runtime.Pager[armauthorization.RoleAssignmentsClientListForSubscriptionResponse] {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewListForSubscriptionPager", options)
	ret0, _ := ret[0].(*runtime.Pager[armauthorization.RoleAssignmentsClientListForSubscriptionResponse])
	return ret0
}

// NewListForSubscriptionPager indicates an expected call of NewListForSubscriptionPager.
func (mr *MockAzureARMAuthorizationRoleAssignmentsClientMockRecorder) NewListForSubscriptionPager(options interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewListForSubscriptionPager", reflect.TypeOf((*MockAzureARMAuthorizationRoleAssignmentsClient)(nil).NewListForSubscriptionPager), options)
}

// MockAzureARMKeyVaultVaultsClient is a mock of AzureARMKeyVaultVaultsClient interface.
type MockAzureARMKeyVaultVaultsClient struct {
	ctrl     *gomock.Controller
	recorder *MockAzureARMKeyVaultVaultsClientMockRecorder
}

// MockAzureARMKeyVaultVaultsClientMockRecorder is the mock recorder for MockAzureARMKeyVaultVaultsClient.
type MockAzureARMKeyVaultVaultsClientMockRecorder struct {
	mock *MockAzureARMKeyVaultVaultsClient
}

// NewMockAzureARMKeyVaultVaultsClient creates a new mock instance.
func NewMockAzureARMKeyVaultVaultsClient(ctrl *gomock.Controller) *MockAzureARMKeyVaultVaultsClient {
	mock := &MockAzureARMKeyVaultVaultsClient{ctrl: ctrl}
	mock.recorder = &MockAzureARMKeyVaultVaultsClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockAzureARMKeyVaultVaultsClient) EXPECT() *MockAzureARMKeyVaultVaultsClientMockRecorder {
	return m.recorder
}

// Get mocks base method.
func (m *MockAzureARMKeyVaultVaultsClient) Get(ctx context.Context, resourceGroupName, vaultName string, options *armkeyvault.VaultsClientGetOptions) (armkeyvault.VaultsClientGetResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", ctx, resourceGroupName, vaultName, options)
	ret0, _ := ret[0].(armkeyvault.VaultsClientGetResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Get indicates an expected call of Get.
func (mr *MockAzureARMKeyVaultVaultsClientMockRecorder) Get(ctx, resourceGroupName, vaultName, options interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockAzureARMKeyVaultVaultsClient)(nil).Get), ctx, resourceGroupName, vaultName, options)
}

// NewListByResourceGroupPager mocks base method.
func (m *MockAzureARMKeyVaultVaultsClient) NewListByResourceGroupPager(resourceGroupName string, options *armkeyvault.VaultsClientListByResourceGroupOptions) *runtime.Pager[armkeyvault.VaultsClientListByResourceGroupResponse] {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewListByResourceGroupPager", resourceGroupName, options)
	ret0, _ := ret[0].(*runtime.Pager[armkeyvault.VaultsClientListByResourceGroupResponse])
	return ret0
}

// NewListByResourceGroupPager indicates an expected call of NewListByResourceGroupPager.
func (mr *MockAzureARMKeyVaultVaultsClientMockRecorder) NewListByResourceGroupPager(resourceGroupName, options interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewListByResourceGroupPager", reflect.TypeOf((*MockAzureARMKeyVaultVaultsClient)(nil).NewListByResourceGroupPager), resourceGroupName, options)
}

// UpdateAccessPolicy mocks base method.
func (m *MockAzureARMKeyVaultVaultsClient) UpdateAccessPolicy(ctx context.Context, resourceGroupName, vaultName string, operationKind armkeyvault.AccessPolicyUpdateKind, parameters armkeyvault.VaultAccessPolicyParameters, options *armkeyvault.VaultsClientUpdateAccessPolicyOptions) (armkeyvault.VaultsClientUpdateAccessPolicyResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpdateAccessPolicy", ctx, resourceGroupName, vaultName, operationKind, parameters, options)
	ret0, _ := ret[0].(armkeyvault.VaultsClientUpdateAccessPolicyResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// UpdateAccessPolicy indicates an expected call of UpdateAccessPolicy.
func (mr *MockAzureARMKeyVaultVaultsClientMockRecorder) UpdateAccessPolicy(ctx, resourceGroupName, vaultName, operationKind, parameters, options interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpdateAccessPolicy", reflect.TypeOf((*MockAzureARMKeyVaultVaultsClient)(nil).UpdateAccessPolicy), ctx, resourceGroupName, vaultName, operationKind, parameters, options)
}
