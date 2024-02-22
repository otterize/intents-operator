package azureagent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/otterize/intents-operator/src/shared/errors"
)

func (a *Agent) findUserAssignedIdentity(ctx context.Context, namespaceName, accountName string) (armmsi.Identity, error) {
	userAssignedIdentityName := a.generateUserAssignedIdentityName(namespaceName, accountName)
	userAssignedIdentity, err := a.userAssignedIdentitiesClient.Get(ctx, resourceGroupName, userAssignedIdentityName, nil)
	if err != nil {
		return armmsi.Identity{}, errors.Wrap(err)
	}

	return userAssignedIdentity.Identity, nil
}

func (a *Agent) generateUserAssignedIdentityName(namespace string, accountName string) string {
	// UserAssignedIdentities rules:
	// 3-128 characters
	// Alphanumerics, hyphens, and underscores
	// Start with letter or number.
	fullName := fmt.Sprintf("otr-%s-%s-%s", namespace, accountName, clusterName)

	var truncatedName string
	if len(fullName) >= (maxManagedIdentityTruncatedLength) {
		truncatedName = fullName[:maxManagedIdentityTruncatedLength]
	} else {
		truncatedName = fullName
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(fullName)))
	hash = hash[:truncatedHashLength]

	return fmt.Sprintf("%s-%s", truncatedName, hash)
}
