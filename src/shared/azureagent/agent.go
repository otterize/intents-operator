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
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"time"
)

const (
	// AzureApplyOnPodLabel is used to mark pods that should be processed by the Azure agent to create an associated Azure identity & role assignment
	AzureApplyOnPodLabel = "credentials-operator.otterize.com/create-azure-workload-identity"
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
	Conf                                Config
	credentials                         *azidentity.DefaultAzureCredential
	resourceClient                      AzureARMResourcesClient
	providerResourceTypesClient         AzureARMResourcesProviderResourceTypesClient
	subscriptionClient                  AzureARMSubscriptionsClient
	resourceGroupsClient                AzureARMResourcesResourceGroupsClient
	managedClustersClient               AzureARMContainerServiceManagedClustersClient
	userAssignedIdentitiesClient        AzureARMMSIUserAssignedIdentitiesClient
	federatedIdentityCredentialsClient  AzureARMMSIFederatedIdentityCredentialsClient
	roleDefinitionsClient               AzureARMAuthorizationRoleDefinitionsClient
	roleAssignmentsClient               AzureARMAuthorizationRoleAssignmentsClient
	vaultsClient                        AzureARMKeyVaultVaultsClient
	subscriptionToResourceClient        map[string]AzureARMResourcesClient
	subscriptionToRoleAssignmentsClient map[string]AzureARMAuthorizationRoleAssignmentsClient

	providerResourceTypesCache *expirable.LRU[string, map[string]armresources.ProviderResourceType]
}

func NewAzureAgent(ctx context.Context, conf Config) (*Agent, error) {
	logrus.Info("Initializing Azure agent")

	credentials, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	armResourcesClientFactory, err := armresources.NewClientFactory(conf.SubscriptionID, credentials, nil)
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

	resourceClient := armResourcesClientFactory.NewClient()
	providerResourceTypesClient := armResourcesClientFactory.NewProviderResourceTypesClient()
	subscriptionClient := armsubscriptionsClientFactory.NewClient()
	userAssignedIdentitiesClient := armmsiClientFactory.NewUserAssignedIdentitiesClient()
	federatedIdentityCredentialsClient := armmsiClientFactory.NewFederatedIdentityCredentialsClient()
	roleDefinitionsClient := armauthorizationClientFactory.NewRoleDefinitionsClient() // Not using SubscriptionID
	roleAssignmentsClient := armauthorizationClientFactory.NewRoleAssignmentsClient() // Using SubscriptionID
	managedClustersClient := armcontainerserviceClientFactory.NewManagedClustersClient()
	vaultsClient := armkeyvaultClientFactory.NewVaultsClient()

	// Per-subscription clients
	subscriptionToResourceClient := make(map[string]AzureARMResourcesClient)
	subscriptionToResourceClient[conf.SubscriptionID] = resourceClient

	subscriptionToRoleAssignmentsClient := make(map[string]AzureARMAuthorizationRoleAssignmentsClient)
	subscriptionToRoleAssignmentsClient[conf.SubscriptionID] = roleAssignmentsClient

	agent := NewAzureAgentFromClients(
		conf,
		credentials,
		resourceClient,
		providerResourceTypesClient,
		subscriptionClient,
		resourceGroupsClient,
		managedClustersClient,
		userAssignedIdentitiesClient,
		federatedIdentityCredentialsClient,
		roleDefinitionsClient,
		roleAssignmentsClient,
		vaultsClient,
		subscriptionToResourceClient,
		subscriptionToRoleAssignmentsClient,
	)

	if err := agent.loadConfDefaults(ctx); err != nil {
		return nil, errors.Wrap(err)
	}

	// Start periodic tasks goroutine
	go wait.Forever(agent.PeriodicTasks, 5*time.Hour)

	return agent, nil
}

func NewAzureAgentFromClients(
	conf Config,
	credentials *azidentity.DefaultAzureCredential,
	resourceClient AzureARMResourcesClient,
	providerResourceTypesClient AzureARMResourcesProviderResourceTypesClient,
	subscriptionClient AzureARMSubscriptionsClient,
	resourceGroupsClient AzureARMResourcesResourceGroupsClient,
	managedClustersClient AzureARMContainerServiceManagedClustersClient,
	userAssignedIdentitiesClient AzureARMMSIUserAssignedIdentitiesClient,
	federatedIdentityCredentialsClient AzureARMMSIFederatedIdentityCredentialsClient,
	roleDefinitionsClient AzureARMAuthorizationRoleDefinitionsClient,
	roleAssignmentsClient AzureARMAuthorizationRoleAssignmentsClient,
	vaultsClient AzureARMKeyVaultVaultsClient,
	subscriptionToResourceClient map[string]AzureARMResourcesClient,
	subscriptionToRoleAssignmentsClient map[string]AzureARMAuthorizationRoleAssignmentsClient,
) *Agent {
	return &Agent{
		Conf:                                conf,
		credentials:                         credentials,
		resourceClient:                      resourceClient,
		providerResourceTypesClient:         providerResourceTypesClient,
		subscriptionClient:                  subscriptionClient,
		resourceGroupsClient:                resourceGroupsClient,
		managedClustersClient:               managedClustersClient,
		userAssignedIdentitiesClient:        userAssignedIdentitiesClient,
		federatedIdentityCredentialsClient:  federatedIdentityCredentialsClient,
		roleDefinitionsClient:               roleDefinitionsClient,
		roleAssignmentsClient:               roleAssignmentsClient,
		vaultsClient:                        vaultsClient,
		subscriptionToResourceClient:        subscriptionToResourceClient,
		subscriptionToRoleAssignmentsClient: subscriptionToRoleAssignmentsClient,

		providerResourceTypesCache: expirable.NewLRU[string, map[string]armresources.ProviderResourceType](100, nil, time.Hour),
	}
}

func (a *Agent) loadConfDefaults(ctx context.Context) error {
	if a.Conf.TenantID == "" {
		subscription, err := a.subscriptionClient.Get(ctx, a.Conf.SubscriptionID, nil)
		if err != nil {
			return errors.Errorf("error querying for subscription: %w", err)
		}
		a.Conf.TenantID = *subscription.TenantID
	}

	if a.Conf.Location == "" {
		resourceGroup, err := a.resourceGroupsClient.Get(ctx, a.Conf.ResourceGroup, nil)
		if err != nil {
			return errors.Errorf("error querying for resource group: %w", err)
		}

		a.Conf.Location = *resourceGroup.Location
	}
	if a.Conf.AKSClusterOIDCIssuerURL == "" {
		cluster, err := a.managedClustersClient.Get(ctx, a.Conf.ResourceGroup, a.Conf.AKSClusterName, nil)
		if err != nil {
			return errors.Errorf("error querying for managed cluster: %w", err)
		}

		a.Conf.AKSClusterOIDCIssuerURL = *cluster.Properties.OidcIssuerProfile.IssuerURL
	}

	return nil
}

func (a *Agent) AppliesOnPod(pod *corev1.Pod) bool {
	return pod.Labels != nil && pod.Labels[AzureApplyOnPodLabel] == "true"
}

// GetRoleAssignmentClientForSubscription returns a role assignments client for the given subscription ID.
// In order to support multiple subscriptions, we need to create a role assignments client for each subscription.
func (a *Agent) GetRoleAssignmentClientForSubscription(subId string) (client AzureARMAuthorizationRoleAssignmentsClient, err error) {
	client, ok := a.subscriptionToRoleAssignmentsClient[subId]
	if !ok {
		// Create a role assignments client for the subscription
		client, err = armauthorization.NewRoleAssignmentsClient(subId, a.credentials, nil)
		if err != nil {
			return nil, errors.Wrap(err)
		}

		a.subscriptionToRoleAssignmentsClient[subId] = client
	}

	return client, nil
}

func (a *Agent) GetResourceClientForSubscription(subId string) (client AzureARMResourcesClient, err error) {
	client, ok := a.subscriptionToResourceClient[subId]
	if !ok {
		// Create a resource client for the subscription
		client, err = armresources.NewClient(subId, a.credentials, nil)
		if err != nil {
			return nil, errors.Wrap(err)
		}

		a.subscriptionToResourceClient[subId] = client
	}

	return client, nil
}
