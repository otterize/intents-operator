package azureagent

import (
	"context"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/otterize/intents-operator/src/shared/errors"
)

func (a *Agent) ListSubscriptions(ctx context.Context) ([]armsubscriptions.Subscription, error) {
	var subscriptions []armsubscriptions.Subscription
	pager := a.subscriptionClient.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, errors.Wrap(err)
		}

		for _, subscription := range page.Value {
			subscriptions = append(subscriptions, *subscription)
		}
	}

	return subscriptions, nil
}
