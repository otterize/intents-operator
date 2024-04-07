package azureagent

import (
	"context"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
)

type Config struct {
	SubscriptionID          string
	ResourceGroup           string
	AKSClusterName          string
	TenantID                string // optional, detected from SubscriptionID if not provided
	Location                string // optional, detected from ResourceGroup if not provided
	AKSClusterOIDCIssuerURL string // optional, detected from AKS cluster if not provided
}

type Agent struct {
	conf                               Config
	credentials                        *azidentity.DefaultAzureCredential
	subscriptionClient                 AzureARMSubscriptionsClient
	resourceGroupsClient               AzureARMResourcesResourceGroupsClient
	managedClustersClient              AzureARMContainerServiceManagedClustersClient
	userAssignedIdentitiesClient       AzureARMMSIUserAssignedIdentitiesClient
	federatedIdentityCredentialsClient AzureARMMSIFederatedIdentityCredentialsClient
	roleDefinitionsClient              AzureARMAuthorizationRoleDefinitionsClient
	roleAssignmentsClient              AzureARMAuthorizationRoleAssignmentsClient
	vaultsClient                       AzureARMKeyVaultVaultsClient
}

func NewAzureAgent(ctx context.Context, conf Config) (*Agent, error) {
	logrus.Info("Initializing Azure agent")

	credentials, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	armsubscriptionsClientFactory, err := armsubscriptions.NewClientFactory(credentials, nil)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	armmsiClientFactory, err := armmsi.NewClientFactory(conf.SubscriptionID, credentials, nil)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	armauthorizationClientFactory, err := armauthorization.NewClientFactory(conf.SubscriptionID, credentials, nil)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	resourceGroupsClient, err := armresources.NewResourceGroupsClient(conf.SubscriptionID, credentials, nil)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	armcontainerserviceClientFactory, err := armcontainerservice.NewClientFactory(conf.SubscriptionID, credentials, nil)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	armkeyvaultClientFactory, err := armkeyvault.NewClientFactory(conf.SubscriptionID, credentials, nil)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	subscriptionClient := armsubscriptionsClientFactory.NewClient()
	userAssignedIdentitiesClient := armmsiClientFactory.NewUserAssignedIdentitiesClient()
	federatedIdentityCredentialsClient := armmsiClientFactory.NewFederatedIdentityCredentialsClient()
	roleDefinitionsClient := armauthorizationClientFactory.NewRoleDefinitionsClient()
	roleAssignmentsClient := armauthorizationClientFactory.NewRoleAssignmentsClient()
	managedClustersClient := armcontainerserviceClientFactory.NewManagedClustersClient()
	vaultsClient := armkeyvaultClientFactory.NewVaultsClient()

	agent := &Agent{
		conf:                               conf,
		credentials:                        credentials,
		subscriptionClient:                 subscriptionClient,
		resourceGroupsClient:               resourceGroupsClient,
		userAssignedIdentitiesClient:       userAssignedIdentitiesClient,
		federatedIdentityCredentialsClient: federatedIdentityCredentialsClient,
		roleDefinitionsClient:              roleDefinitionsClient,
		roleAssignmentsClient:              roleAssignmentsClient,
		managedClustersClient:              managedClustersClient,
		vaultsClient:                       vaultsClient,
	}

	if err := agent.loadConfDefaults(ctx); err != nil {
		return nil, errors.Wrap(err)
	}

	return agent, nil
}

func (a *Agent) loadConfDefaults(ctx context.Context) error {
	if a.conf.TenantID == "" {
		subscription, err := a.subscriptionClient.Get(ctx, a.conf.SubscriptionID, nil)
		if err != nil {
			return errors.Errorf("error querying for subscription: %w", err)
		}
		a.conf.TenantID = *subscription.TenantID
	}

	if a.conf.Location == "" {
		resourceGroup, err := a.resourceGroupsClient.Get(ctx, a.conf.ResourceGroup, nil)
		if err != nil {
			return errors.Errorf("error querying for resource group: %w", err)
		}

		a.conf.Location = *resourceGroup.Location
	}
	if a.conf.AKSClusterOIDCIssuerURL == "" {
		cluster, err := a.managedClustersClient.Get(ctx, a.conf.ResourceGroup, a.conf.AKSClusterName, nil)
		if err != nil {
			return errors.Errorf("error querying for managed cluster: %w", err)
		}

		a.conf.AKSClusterOIDCIssuerURL = *cluster.Properties.OidcIssuerProfile.IssuerURL
	}

	return nil
}
