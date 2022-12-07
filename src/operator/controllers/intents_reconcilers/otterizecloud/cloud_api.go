package otterizecloud

import (
	"context"
	"github.com/Khan/genqlient/graphql"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloudclient"
	"github.com/sirupsen/logrus"
)

type CloudApi interface {
	ReportKubernetesNamespace(ctx context.Context, namespace string) error
}

type CloudApiImpl struct {
	client graphql.Client
}

func NewClient(ctx context.Context) (CloudApi, bool, error) {
	client, ok, err := otterizecloudclient.NewClient(ctx)
	if !ok {
		return nil, false, nil
	} else if err != nil {
		return nil, true, err
	}

	return &CloudApiImpl{client: client}, true, nil
}

func (c *CloudApiImpl) ReportKubernetesNamespace(ctx context.Context, namespace string) error {
	res, err := graphqlclient.ReportKubernetesNamespace(ctx, c.client, namespace)
	if err != nil {
		return err
	}
	// Return value is bool - returns true if anything was updated in the DB
	if res.ReportKubernetesNamespace {
		logrus.Infof("Successfully reported namespace # %s # to Otterize cloud", namespace)
	}
	return nil
}
