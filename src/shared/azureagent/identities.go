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

func (a *Agent) GenerateUserAssignedIdentityName(namespace string, accountName string) string {
	fullName := fmt.Sprintf("ottr-uai-%s-%s-%s", namespace, accountName, a.Conf.AKSClusterName)
	return agentutils.TruncateHashName(fullName, maxManagedIdentityLength)
}

func (a *Agent) generateFederatedIdentityCredentialsName(namespace string, accountName string) string {
	fullName := fmt.Sprintf("ottr-fic-%s-%s-%s", namespace, accountName, a.Conf.AKSClusterName)
	return agentutils.TruncateHashName(fullName, maxFederatedIdentityLength)
}

var ErrUserIdentityNotFound = errors.NewSentinelError("user assigned identity not found")

func (a *Agent) FindUserAssignedIdentity(ctx context.Context, namespace string, accountName string) (armmsi.Identity, error) {
	userAssignedIdentityName := a.GenerateUserAssignedIdentityName(namespace, accountName)
	userAssignedIdentity, err := a.userAssignedIdentitiesClient.Get(ctx, a.Conf.ResourceGroup, userAssignedIdentityName, nil)
	if err != nil {
		if azureerrors.IsNotFoundErr(err) {
			return armmsi.Identity{}, errors.Wrap(ErrUserIdentityNotFound)
		}
		return armmsi.Identity{}, errors.Wrap(err)
	}

	return userAssignedIdentity.Identity, nil
}

func (a *Agent) GetOrCreateUserAssignedIdentity(ctx context.Context, namespace string, accountName string) (armmsi.Identity, error) {
	logger := logrus.WithField("namespace", namespace).WithField("account", accountName)
	userAssignedIdentityName := a.GenerateUserAssignedIdentityName(namespace, accountName)
	logger.WithField("identity", userAssignedIdentityName).Debug("getting or creating user assigned identity")
	userAssignedIdentity, err := a.userAssignedIdentitiesClient.CreateOrUpdate(
		ctx,
		a.Conf.ResourceGroup,
		userAssignedIdentityName,
		armmsi.Identity{
			Location: &a.Conf.Location,
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
		a.Conf.ResourceGroup,
		userAssignedIdentityName,
		federatedIdentityCredentialsName,
		armmsi.FederatedIdentityCredential{
			Properties: &armmsi.FederatedIdentityCredentialProperties{
				Issuer: lo.ToPtr(a.Conf.AKSClusterOIDCIssuerURL),
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

func (a *Agent) DeleteUserAssignedIdentity(ctx context.Context, namespace string, accountName string) error {
	logger := logrus.WithField("namespace", namespace).WithField("account", accountName)
	userAssignedIdentityName := a.GenerateUserAssignedIdentityName(namespace, accountName)
	federatedIdentityCredentialsName := a.generateFederatedIdentityCredentialsName(namespace, accountName)

	// Delete roles assigned to the identity
	identity, err := a.FindUserAssignedIdentity(ctx, namespace, accountName)
	if err != nil {
		if errors.Is(err, ErrUserIdentityNotFound) {
			return nil
		}
		return errors.Wrap(err)
	}

	roleAssignments, err := a.ListRoleAssignmentsAcrossSubscriptions(ctx, &identity)
	if err != nil {
		return errors.Wrap(err)
	}

	for _, roleAssignment := range roleAssignments {
		if err := a.DeleteRoleAssignment(ctx, roleAssignment); err != nil {
			if azureerrors.IsNotFoundErr(err) {
				continue
			}
			return errors.Wrap(err)
		}
	}

	// Delete the federated identity credentials
	logger.WithField("federatedIdentity", federatedIdentityCredentialsName).Info("deleting federated identity credentials")
	_, err = a.federatedIdentityCredentialsClient.Delete(ctx, a.Conf.ResourceGroup, userAssignedIdentityName, federatedIdentityCredentialsName, nil)
	if err != nil && !azureerrors.IsNotFoundErr(err) && !IsParentResourceNotFoundErr(err) {
		return errors.Wrap(err)
	}

	logger.WithField("identity", userAssignedIdentityName).Info("deleting user assigned identity")
	_, err = a.userAssignedIdentitiesClient.Delete(ctx, a.Conf.ResourceGroup, userAssignedIdentityName, nil)
	if err != nil && !azureerrors.IsNotFoundErr(err) {
		return errors.Wrap(err)
	}

	return nil
}
