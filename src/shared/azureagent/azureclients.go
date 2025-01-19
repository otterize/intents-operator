package azureagent

import (
	"context"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
)

type AzureARMResourcesClient interface {
	GetByID(ctx context.Context, resourceID string, apiVersion string, options *armresources.ClientGetByIDOptions) (armresources.ClientGetByIDResponse, error)
	NewListPager(options *armresources.ClientListOptions) *runtime.Pager[armresources.ClientListResponse]
}

type AzureARMResourcesProviderResourceTypesClient interface {
	List(ctx context.Context, resourceProviderNamespace string, options *armresources.ProviderResourceTypesClientListOptions) (armresources.ProviderResourceTypesClientListResponse, error)
}

type AzureARMSubscriptionsClient interface {
	Get(ctx context.Context, subscriptionID string, options *armsubscriptions.ClientGetOptions) (armsubscriptions.ClientGetResponse, error)
	NewListPager(options *armsubscriptions.ClientListOptions) *runtime.Pager[armsubscriptions.ClientListResponse]
}

type AzureARMResourcesResourceGroupsClient interface {
	Get(ctx context.Context, resourceGroupName string, options *armresources.ResourceGroupsClientGetOptions) (armresources.ResourceGroupsClientGetResponse, error)
}

type AzureARMContainerServiceManagedClustersClient interface {
	Get(ctx context.Context, resourceGroupName string, resourceName string, options *armcontainerservice.ManagedClustersClientGetOptions) (armcontainerservice.ManagedClustersClientGetResponse, error)
}

type AzureARMMSIUserAssignedIdentitiesClient interface {
	CreateOrUpdate(ctx context.Context, resourceGroupName string, resourceName string, parameters armmsi.Identity, options *armmsi.UserAssignedIdentitiesClientCreateOrUpdateOptions) (armmsi.UserAssignedIdentitiesClientCreateOrUpdateResponse, error)
	Delete(ctx context.Context, resourceGroupName string, resourceName string, options *armmsi.UserAssignedIdentitiesClientDeleteOptions) (armmsi.UserAssignedIdentitiesClientDeleteResponse, error)
	Get(ctx context.Context, resourceGroupName string, resourceName string, options *armmsi.UserAssignedIdentitiesClientGetOptions) (armmsi.UserAssignedIdentitiesClientGetResponse, error)
}

type AzureARMMSIFederatedIdentityCredentialsClient interface {
	CreateOrUpdate(ctx context.Context, resourceGroupName string, resourceName string, federatedIdentityCredentialResourceName string, parameters armmsi.FederatedIdentityCredential, options *armmsi.FederatedIdentityCredentialsClientCreateOrUpdateOptions) (armmsi.FederatedIdentityCredentialsClientCreateOrUpdateResponse, error)
	Delete(ctx context.Context, resourceGroupName string, resourceName string, federatedIdentityCredentialResourceName string, options *armmsi.FederatedIdentityCredentialsClientDeleteOptions) (armmsi.FederatedIdentityCredentialsClientDeleteResponse, error)
	Get(ctx context.Context, resourceGroupName string, resourceName string, federatedIdentityCredentialResourceName string, options *armmsi.FederatedIdentityCredentialsClientGetOptions) (armmsi.FederatedIdentityCredentialsClientGetResponse, error)
}

type AzureARMAuthorizationRoleDefinitionsClient interface {
	Delete(ctx context.Context, scope string, roleDefinitionID string, options *armauthorization.RoleDefinitionsClientDeleteOptions) (armauthorization.RoleDefinitionsClientDeleteResponse, error)
	CreateOrUpdate(ctx context.Context, scope string, roleDefinitionID string, roleDefinition armauthorization.RoleDefinition, options *armauthorization.RoleDefinitionsClientCreateOrUpdateOptions) (armauthorization.RoleDefinitionsClientCreateOrUpdateResponse, error)
	NewListPager(scope string, options *armauthorization.RoleDefinitionsClientListOptions) *runtime.Pager[armauthorization.RoleDefinitionsClientListResponse]
}

type AzureARMAuthorizationRoleAssignmentsClient interface {
	Create(ctx context.Context, scope string, roleAssignmentName string, parameters armauthorization.RoleAssignmentCreateParameters, options *armauthorization.RoleAssignmentsClientCreateOptions) (armauthorization.RoleAssignmentsClientCreateResponse, error)
	Delete(ctx context.Context, scope string, roleAssignmentName string, options *armauthorization.RoleAssignmentsClientDeleteOptions) (armauthorization.RoleAssignmentsClientDeleteResponse, error)
	NewListForSubscriptionPager(options *armauthorization.RoleAssignmentsClientListForSubscriptionOptions) *runtime.Pager[armauthorization.RoleAssignmentsClientListForSubscriptionResponse]
}

type AzureARMKeyVaultVaultsClient interface {
	Get(ctx context.Context, resourceGroupName string, vaultName string, options *armkeyvault.VaultsClientGetOptions) (armkeyvault.VaultsClientGetResponse, error)
	NewListByResourceGroupPager(resourceGroupName string, options *armkeyvault.VaultsClientListByResourceGroupOptions) *runtime.Pager[armkeyvault.VaultsClientListByResourceGroupResponse]
	UpdateAccessPolicy(ctx context.Context, resourceGroupName string, vaultName string, operationKind armkeyvault.AccessPolicyUpdateKind, parameters armkeyvault.VaultAccessPolicyParameters, options *armkeyvault.VaultsClientUpdateAccessPolicyOptions) (armkeyvault.VaultsClientUpdateAccessPolicyResponse, error)
}
