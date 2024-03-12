package azureagent

import (
	"context"
	"fmt"
	azureerrors "github.com/Azure/azure-sdk-for-go-extensions/pkg/errors"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/otterize/intents-operator/src/shared/agentutils"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
)

const (
	federatedIdentityAudience = "api://AzureADTokenExchange"

	// UserAssignedIdentities rules:
	// 3-128 characters
	// Alphanumerics, hyphens, and underscores
	// Start with letter or number.
	maxManagedIdentityLength = 128

	// 3-120 characters
	// Alphanumeric, dash, or underscore characters are supported
	// the first character must be alphanumeric only
	maxFederatedIdentityLength = 120
)

func (a *Agent) generateUserAssignedIdentityName(namespace string, accountName string) string {
	fullName := fmt.Sprintf("ottr-uai-%s-%s-%s", namespace, accountName, a.conf.AKSClusterName)
	return agentutils.TruncateHashName(fullName, maxManagedIdentityLength)
}

func (a *Agent) generateFederatedIdentityCredentialsName(namespace string, accountName string) string {
	fullName := fmt.Sprintf("ottr-fic-%s-%s-%s", namespace, accountName, a.conf.AKSClusterName)
	return agentutils.TruncateHashName(fullName, maxFederatedIdentityLength)
}

var ErrUserIdentityNotFound = errors.New("user assigned identity not found")

func (a *Agent) findUserAssignedIdentity(ctx context.Context, namespace string, accountName string) (armmsi.Identity, error) {
	userAssignedIdentityName := a.generateUserAssignedIdentityName(namespace, accountName)
	userAssignedIdentity, err := a.userAssignedIdentitiesClient.Get(ctx, a.conf.ResourceGroup, userAssignedIdentityName, nil)
	if err != nil {
		if azureerrors.IsNotFoundErr(err) {
			return armmsi.Identity{}, errors.Wrap(ErrUserIdentityNotFound)
		}
		return armmsi.Identity{}, errors.Wrap(err)
	}

	return userAssignedIdentity.Identity, nil
}

func (a *Agent) getOrCreateUserAssignedIdentity(ctx context.Context, namespace string, accountName string) (armmsi.Identity, error) {
	logger := logrus.WithField("namespace", namespace).WithField("account", accountName)
	userAssignedIdentityName := a.generateUserAssignedIdentityName(namespace, accountName)
	logger.WithField("identity", userAssignedIdentityName).Debug("getting or creating user assigned identity")
	userAssignedIdentity, err := a.userAssignedIdentitiesClient.CreateOrUpdate(
		ctx,
		a.conf.ResourceGroup,
		userAssignedIdentityName,
		armmsi.Identity{
			Location: &a.conf.Location,
		},
		nil,
	)
	if err != nil {
		return armmsi.Identity{}, errors.Wrap(err)
	}

	federatedIdentityCredentialsName := a.generateFederatedIdentityCredentialsName(namespace, accountName)
	logger.WithField("federatedIdentity", federatedIdentityCredentialsName).Debug("getting or creating federated identity credentials")
	_, err = a.federatedIdentityCredentialsClient.CreateOrUpdate(
		ctx,
		a.conf.ResourceGroup,
		userAssignedIdentityName,
		federatedIdentityCredentialsName,
		armmsi.FederatedIdentityCredential{
			Properties: &armmsi.FederatedIdentityCredentialProperties{
				Issuer: lo.ToPtr(a.conf.AKSClusterOIDCIssuerURL),
				Subject: lo.ToPtr(
					fmt.Sprintf("system:serviceaccount:%s:%s",
						namespace,
						accountName)),
				Audiences: []*string{lo.ToPtr(federatedIdentityAudience)},
			},
		},
		nil,
	)
	if err != nil {
		return armmsi.Identity{}, errors.Wrap(err)
	}

	return userAssignedIdentity.Identity, nil
}

func (a *Agent) deleteUserAssignedIdentity(ctx context.Context, namespace string, accountName string) error {
	logger := logrus.WithField("namespace", namespace).WithField("account", accountName)
	userAssignedIdentityName := a.generateUserAssignedIdentityName(namespace, accountName)
	federatedIdentityCredentialsName := a.generateFederatedIdentityCredentialsName(namespace, accountName)

	logger.WithField("federatedIdentity", federatedIdentityCredentialsName).Debug("deleting federated identity credentials")
	_, err := a.federatedIdentityCredentialsClient.Delete(ctx, a.conf.ResourceGroup, userAssignedIdentityName, federatedIdentityCredentialsName, nil)
	if err != nil {
		return errors.Wrap(err)
	}

	logger.WithField("identity", userAssignedIdentityName).Debug("deleting user assigned identity")
	_, err = a.userAssignedIdentitiesClient.Delete(ctx, a.conf.ResourceGroup, userAssignedIdentityName, nil)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}
