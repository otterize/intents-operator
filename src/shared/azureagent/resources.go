package azureagent

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/shared/errors"
	"regexp"
	"strings"
)

const StorageAccountPattern = `providers/Microsoft.Storage/storageAccounts/([^/]+)`

func (a *Agent) ValidateScope(ctx context.Context, scope string) error {
	res, err := a.resourceClient.GetByID(ctx, scope, "2022-09-01", nil)
	if err != nil {
		return err
	}
	if res.GenericResource.ID == nil {
		return errors.Errorf("scope %s not found", scope)
	}
	return nil
}

func (a *Agent) GetFullResourceScope(ctx context.Context, partialScope string) (string, error) {
	subscriptions, err := a.ListSubscriptions(ctx)
	if err != nil {
		return "", errors.Wrap(err)
	}

	// Storage accounts are unique across azure, so we should look up the storage account directly
	regex := regexp.MustCompile(StorageAccountPattern)
	matchStorageAccount := regex.FindStringSubmatch(partialScope)
	scopeToCheck := partialScope
	if len(matchStorageAccount) > 1 {
		scopeToCheck = fmt.Sprintf("/providers/Microsoft.Storage/storageAccounts/%s", matchStorageAccount[1])
	}

	for _, sub := range subscriptions {
		resourceClient, err := a.GetResourceClientForSubscription(*sub.SubscriptionID)
		if err != nil {
			return "", errors.Wrap(err)
		}

		pager := resourceClient.NewListPager(nil)
		for pager.More() {
			resp, err := pager.NextPage(ctx)
			if err != nil {
				return "", errors.Wrap(err)
			}

			// Iterate through resources to find a resource matching the partial scope
			for _, resource := range resp.Value {
				if resource.ID != nil && strings.HasSuffix(*resource.ID, scopeToCheck) {
					if len(matchStorageAccount) > 1 {
						// Rebuild the full scope:
						// partialScope: <optional-prefix>/providers/Microsoft.Storage/storageAccounts/<storage-account-name>/<child-resource>
						// scopeToCheck: /providers/Microsoft.Storage/storageAccounts/<storage-account-name>
						// parts[1]: /<child-resource> || ""
						parts := strings.Split(partialScope, scopeToCheck)
						fullScope := fmt.Sprintf("%s%s", *resource.ID, parts[1])
						return fullScope, nil
					}
					return *resource.ID, nil
				}
			}
		}
	}

	return "", errors.Errorf("resource not found for partial scope %s", partialScope)
}
