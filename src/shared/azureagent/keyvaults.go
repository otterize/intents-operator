package azureagent

import (
	"context"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
)

func (a *Agent) listKeyVaults(ctx context.Context) ([]string, error) {
	var keyVaults []string
	pager := a.vaultsClient.NewListByResourceGroupPager(a.conf.ResourceGroup, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, errors.Wrap(err)
		}
		for _, keyVault := range page.Value {
			keyVaults = append(keyVaults, *keyVault.Name)
		}
	}

	return keyVaults, nil
}

func (a *Agent) getKeyVaultAccessPolicies(ctx context.Context, keyVaultName string, userAssignedIdentity armmsi.Identity) ([]*armkeyvault.AccessPolicyEntry, error) {
	getResponse, err := a.vaultsClient.Get(ctx, a.conf.ResourceGroup, keyVaultName, nil)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	accessPoliciesForIdentity := lo.Filter(getResponse.Vault.Properties.AccessPolicies, func(policy *armkeyvault.AccessPolicyEntry, _ int) bool {
		return *policy.ObjectID == *userAssignedIdentity.Properties.ClientID
	})

	return accessPoliciesForIdentity, nil
}

func (a *Agent) getExistingKeyVaultAccessPolicies(ctx context.Context, userAssignedIdentity armmsi.Identity) (map[string][]*armkeyvault.AccessPolicyEntry, error) {
	keyVaults, err := a.listKeyVaults(ctx)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	accessPoliciesByName := make(map[string][]*armkeyvault.AccessPolicyEntry)

	for _, keyVaultName := range keyVaults {
		accessPolicies, err := a.getKeyVaultAccessPolicies(ctx, keyVaultName, userAssignedIdentity)
		if err != nil {
			return nil, errors.Wrap(err)
		}

		accessPoliciesByName[keyVaultName] = accessPolicies
	}

	return accessPoliciesByName, nil
}

// PtrSlicesEqual checks if two slices of pointers are equal, by comparing the dereferenced pointers
func PtrSlicesEqual[T comparable](a, b []*T) bool {
	return slices.EqualFunc(a, b, func(x, y *T) bool {
		return lo.FromPtr(x) == lo.FromPtr(y)
	})
}

func AccessPoliciesEqual(a, b *armkeyvault.AccessPolicyEntry) bool {
	return lo.FromPtr(a.ObjectID) == lo.FromPtr(b.ObjectID) &&
		lo.FromPtr(a.TenantID) == lo.FromPtr(b.TenantID) &&
		PtrSlicesEqual(a.Permissions.Certificates, b.Permissions.Certificates) &&
		PtrSlicesEqual(a.Permissions.Keys, b.Permissions.Keys) &&
		PtrSlicesEqual(a.Permissions.Secrets, b.Permissions.Secrets) &&
		PtrSlicesEqual(a.Permissions.Storage, b.Permissions.Storage)
}

func (a *Agent) updateKeyVaultPolicy(ctx context.Context, keyVaultName string,
	operationKind armkeyvault.AccessPolicyUpdateKind, policy armkeyvault.AccessPolicyEntry) error {
	_, err := a.vaultsClient.UpdateAccessPolicy(ctx,
		a.conf.ResourceGroup,
		keyVaultName,
		operationKind,
		armkeyvault.VaultAccessPolicyParameters{
			Properties: &armkeyvault.VaultAccessPolicyProperties{
				AccessPolicies: []*armkeyvault.AccessPolicyEntry{&policy},
			},
		},
		nil,
	)
	if err != nil {
		return errors.Wrap(err)
	}
	return nil
}

func (a *Agent) addKeyVaultAccessPolicy(ctx context.Context, keyVaultName string, policy armkeyvault.AccessPolicyEntry) error {
	logger := logrus.WithField("name", keyVaultName).WithField("objectId", policy.ObjectID)
	logger.Debug("Adding key vault access policy")
	return a.updateKeyVaultPolicy(ctx, keyVaultName, armkeyvault.AccessPolicyUpdateKindAdd, policy)
}

func (a *Agent) replaceKeyVaultAccessPolicy(ctx context.Context, keyVaultName string, policy armkeyvault.AccessPolicyEntry) error {
	logger := logrus.WithField("name", keyVaultName).WithField("objectId", policy.ObjectID)
	logger.Debug("Replacing key vault access policy")
	return a.updateKeyVaultPolicy(ctx, keyVaultName, armkeyvault.AccessPolicyUpdateKindReplace, policy)
}

func (a *Agent) removeKeyVaultAccessPolicy(ctx context.Context, keyVaultName string, userAssignedIdentity armmsi.Identity) error {
	logger := logrus.WithField("name", keyVaultName).WithField("objectId", userAssignedIdentity.Properties.ClientID)
	logger.Debug("Removing key vault access policy")
	return a.updateKeyVaultPolicy(ctx, keyVaultName, armkeyvault.AccessPolicyUpdateKindRemove, armkeyvault.AccessPolicyEntry{ObjectID: userAssignedIdentity.Properties.ClientID})
}
