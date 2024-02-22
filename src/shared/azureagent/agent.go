package azureagent

import (
	"context"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/sirupsen/logrus"
)

const (
	// TODO: read from environment
	subscriptionID    = "ef54c90c-5351-4c8f-a126-16a6d789104f"
	location          = "eastus"
	resourceGroupName = "myResourceGroup"
	clusterName       = "myCluster"
)

type Agent struct {
	credentials                        *azidentity.DefaultAzureCredential
	userAssignedIdentitiesClient       *armmsi.UserAssignedIdentitiesClient
	federatedIdentityCredentialsClient *armmsi.FederatedIdentityCredentialsClient
	roleDefinitionsClient              *armauthorization.RoleDefinitionsClient
	roleAssignmentsClient              *armauthorization.RoleAssignmentsClient
}

func NewAzureAgent(ctx context.Context) (*Agent, error) {
	logrus.Info("Initializing Azure agent")

	credentials, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}

	armmsiClientFactory, err := armmsi.NewClientFactory(subscriptionID, credentials, nil)
	if err != nil {
		return nil, err
	}

	armauthorizationClientFactory, err := armauthorization.NewClientFactory(subscriptionID, credentials, nil)
	if err != nil {
		return nil, err
	}

	userAssignedIdentitiesClient := armmsiClientFactory.NewUserAssignedIdentitiesClient()
	federatedIdentityCredentialsClient := armmsiClientFactory.NewFederatedIdentityCredentialsClient()
	roleDefinitionsClient := armauthorizationClientFactory.NewRoleDefinitionsClient()
	roleAssignmentsClient := armauthorizationClientFactory.NewRoleAssignmentsClient()

	return &Agent{
		credentials:                        credentials,
		userAssignedIdentitiesClient:       userAssignedIdentitiesClient,
		federatedIdentityCredentialsClient: federatedIdentityCredentialsClient,
		roleDefinitionsClient:              roleDefinitionsClient,
		roleAssignmentsClient:              roleAssignmentsClient,
	}, nil
}
