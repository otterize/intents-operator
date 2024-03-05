package azureagent

import (
	"context"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

const (
	AzurePodLabel                        = "credentials-operator.otterize.com/create-azure-role-assignment"
	ServiceManagedByAzureAgentAnnotation = "credentials-operator.otterize.com/managed-by-azure-agent"
)

type Config struct {
	SubscriptionID          string
	ResourceGroup           string
	AKSClusterName          string
	Location                string // optional, detected from ResourceGroup if not provided
	AKSClusterOIDCIssuerURL string // optional, detected from AKS cluster if not provided
}

type Agent struct {
	conf                               Config
	credentials                        *azidentity.DefaultAzureCredential
	resourceGroupsClient               *armresources.ResourceGroupsClient
	userAssignedIdentitiesClient       *armmsi.UserAssignedIdentitiesClient
	federatedIdentityCredentialsClient *armmsi.FederatedIdentityCredentialsClient
	roleDefinitionsClient              *armauthorization.RoleDefinitionsClient
	roleAssignmentsClient              *armauthorization.RoleAssignmentsClient
	managedClustersClient              *armcontainerservice.ManagedClustersClient
}

func NewAzureAgent(ctx context.Context, conf Config) (*Agent, error) {
	logrus.Info("Initializing Azure agent")

	credentials, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}

	armmsiClientFactory, err := armmsi.NewClientFactory(conf.SubscriptionID, credentials, nil)
	if err != nil {
		return nil, err
	}

	armauthorizationClientFactory, err := armauthorization.NewClientFactory(conf.SubscriptionID, credentials, nil)
	if err != nil {
		return nil, err
	}

	resourceGroupsClient, err := armresources.NewResourceGroupsClient(conf.SubscriptionID, credentials, nil)
	if err != nil {
		return nil, err
	}

	armcontainerserviceClientFactory, err := armcontainerservice.NewClientFactory(conf.SubscriptionID, credentials, nil)
	if err != nil {
		return nil, err
	}

	userAssignedIdentitiesClient := armmsiClientFactory.NewUserAssignedIdentitiesClient()
	federatedIdentityCredentialsClient := armmsiClientFactory.NewFederatedIdentityCredentialsClient()
	roleDefinitionsClient := armauthorizationClientFactory.NewRoleDefinitionsClient()
	roleAssignmentsClient := armauthorizationClientFactory.NewRoleAssignmentsClient()
	managedClustersClient := armcontainerserviceClientFactory.NewManagedClustersClient()

	if conf.Location == "" {
		resourceGroup, err := resourceGroupsClient.Get(ctx, conf.ResourceGroup, nil)
		if err != nil {
			return nil, err
		}

		conf.Location = *resourceGroup.Location
	}

	agent := &Agent{
		conf:                               conf,
		credentials:                        credentials,
		resourceGroupsClient:               resourceGroupsClient,
		userAssignedIdentitiesClient:       userAssignedIdentitiesClient,
		federatedIdentityCredentialsClient: federatedIdentityCredentialsClient,
		roleDefinitionsClient:              roleDefinitionsClient,
		roleAssignmentsClient:              roleAssignmentsClient,
		managedClustersClient:              managedClustersClient,
	}

	if conf.Location == "" {
		conf.Location, err = agent.loadLocation(ctx)
		if err != nil {
			return nil, err
		}
	}
	if conf.AKSClusterOIDCIssuerURL == "" {
		conf.AKSClusterOIDCIssuerURL, err = agent.loadClusterOIDCIssuer(ctx)
		if err != nil {
			return nil, err
		}
	}

	return agent, nil
}

func (a *Agent) loadLocation(ctx context.Context) (string, error) {
	resourceGroup, err := a.resourceGroupsClient.Get(ctx, a.conf.ResourceGroup, nil)
	if err != nil {
		return "", err
	}

	return *resourceGroup.Location, nil
}

func (a *Agent) loadClusterOIDCIssuer(ctx context.Context) (string, error) {
	cluster, err := a.managedClustersClient.Get(ctx, a.conf.ResourceGroup, a.conf.AKSClusterName, nil)
	if err != nil {
		return "", err
	}

	return *cluster.Properties.OidcIssuerProfile.IssuerURL, nil
}

func (a *Agent) AppliesOnPod(pod *corev1.Pod) bool {
	return pod.Labels != nil && pod.Labels[AzurePodLabel] == "true"
}
