package azureagent

import (
	"context"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/otterize/intents-operator/src/shared/errors"
	"strings"
)

func (a *Agent) ValidateScope(ctx context.Context, scope string) error {
	res, err := a.resourceClient.GetByID(ctx, scope, "2022-09-01", nil)
	if err != nil {
		return errors.Wrap(err)
	}
	if res.GenericResource.ID == nil {
		return errors.Errorf("scope %s not found", scope)
	}
	return nil
}

func (a *Agent) GetFullStorageResourceScope(ctx context.Context, storageAccountScope string, partialScope string) (string, error) {
	subscriptions, err := a.ListSubscriptions(ctx)
	if err != nil {
		return "", errors.Wrap(err)
	}

	// Iterate through all subscriptions to find the parent storage account
	for _, sub := range subscriptions {
		resourceClient, err := a.GetResourceClientForSubscription(*sub.SubscriptionID)
		if err != nil {
			return "", errors.Wrap(err)
		}

		// List only storage accounts to reduce the number of resources to iterate through
		pager := resourceClient.NewListPager(&armresources.ClientListOptions{
			Filter: to.Ptr("resourceType eq 'Microsoft.Storage/storageAccounts'"),
		})
		for pager.More() {
			resp, err := pager.NextPage(ctx)
			if err != nil {
				return "", errors.Wrap(err)
			}

			// Iterate through resources to find a resource matching the partial scope
			for _, resource := range resp.Value {
				if resource.ID != nil && strings.HasSuffix(*resource.ID, storageAccountScope) {
					// Rebuild the full scope:
					// partialScope: <optional-prefix>/providers/Microsoft.Storage/storageAccounts/<storage-account-name>/<child-resource>
					// parts[1]: /<child-resource> || ""
					parts := strings.Split(partialScope, storageAccountScope)
					fullScope := fmt.Sprintf("%s%s", *resource.ID, parts[1])
					return fullScope, nil
				}
			}
		}
	}

	return "", errors.Errorf("resource not found for partial scope %s", partialScope)
}
