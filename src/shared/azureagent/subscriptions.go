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
			if *subscription.ID != "/subscriptions/0eb222dc-cf72-4123-b0f5-d0dd21804d10" {
				subscriptions = append(subscriptions, *subscription)
			}
		}
	}

	return subscriptions, nil
}
