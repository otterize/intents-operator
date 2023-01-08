package otterizecloud

import (
	"context"
	"github.com/Khan/genqlient/graphql"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/sirupsen/logrus"
)

type CloudClient interface {
	ReportKafkaServerConfig(ctx context.Context, server graphqlclient.KafkaServerConfigInput) error
	ReportAppliedIntents(ctx context.Context, namespace *string, intents []*graphqlclient.IntentInput) error
	ReportComponentStatus(ctx context.Context, component graphqlclient.ComponentType)
}

type CloudClientImpl struct {
	client graphql.Client
}

func NewClient(ctx context.Context) (CloudClient, bool, error) {
	client, ok, err := otterizecloudclient.NewClient(ctx)
	if !ok {
		return nil, false, nil
	} else if err != nil {
		return nil, true, err
	}

	return &CloudClientImpl{client: client}, true, nil
}

func (c *CloudClientImpl) ReportKafkaServerConfig(ctx context.Context, server graphqlclient.KafkaServerConfigInput) error {
	_, err := graphqlclient.ReportKafkaServerConfig(ctx, c.client, server)
	if err != nil {
		return err
	}

	logrus.Infof("Successfully reported KafkaServerConfig # %+v # to Otterize cloud", server)

	return nil
}

func (c *CloudClientImpl) ReportAppliedIntents(
	ctx context.Context,
	namespace *string,
	intents []*graphqlclient.IntentInput) error {

	_, err := graphqlclient.ReportAppliedKubernetesIntents(ctx, c.client, namespace, intents)
	if err != nil {
		return err
	}

	logrus.Infof("New intents count for namespace %s: %d", *namespace, len(intents))
	return nil
}

func (c *CloudClientImpl) ReportComponentStatus(ctx context.Context, component graphqlclient.ComponentType) {
	_, err := graphqlclient.ReportComponentStatus(ctx, c.client, component)
	if err != nil {
		logrus.WithError(err).Error("Failed to reported component status Otterize cloud")
		return
	}
}
