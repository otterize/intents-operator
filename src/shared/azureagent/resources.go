package azureagent

import (
	"context"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"regexp"
	"strings"
)

var ResourceProviderNamespaceRegex = regexp.MustCompile(`^/subscriptions/[^/]+/resourceGroups/[^/]+/providers/([^/]+)/([^/]+).*$`)

const defaultAPIVersion = "2022-09-01"

func (a *Agent) loadProviderResourceTypes(ctx context.Context, provider string) (map[string]armresources.ProviderResourceType, error) {
	if providerResourceTypes, ok := a.providerResourceTypesCache.Get(provider); ok {
		return providerResourceTypes, nil
	}

	res, err := a.providerResourceTypesClient.List(ctx, provider, nil)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	providerResourceTypesList := lo.FromSlicePtr(res.ProviderResourceTypeListResult.Value)
	providerResourceTypes := lo.SliceToMap(providerResourceTypesList, func(r armresources.ProviderResourceType) (string, armresources.ProviderResourceType) {
		return lo.FromPtr(r.ResourceType), r
	})
	a.providerResourceTypesCache.Add(provider, providerResourceTypes)
	return providerResourceTypes, nil
}

func (a *Agent) getResourceAPIVersion(ctx context.Context, provider string, resourceName string) (string, error) {
	providerResourceTypes, err := a.loadProviderResourceTypes(ctx, provider)
	if err != nil {
		return "", errors.Wrap(err)
	}

	resourceType, found := providerResourceTypes[resourceName]
	if !found {
		return "", errors.Errorf("resource type %s not found for provider %s", resourceName, provider)
	}

	return lo.FromPtr(resourceType.DefaultAPIVersion), nil
}

func (a *Agent) ValidateScope(ctx context.Context, scope string) error {
	apiVersion := defaultAPIVersion
	if match := ResourceProviderNamespaceRegex.FindStringSubmatch(scope); len(match) > 1 {
		provider := match[1]
		resourceName := match[2]
		resourceAPIVersion, err := a.getResourceAPIVersion(ctx, provider, resourceName)
		if err != nil {
			return errors.Wrap(err)
		}
		apiVersion = resourceAPIVersion
	}

	res, err := a.resourceClient.GetByID(ctx, scope, apiVersion, nil)
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
