package otterizecloud

import (
	"context"
	"github.com/Khan/genqlient/graphql"
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/sirupsen/logrus"
)

type CloudClient interface {
	ReportKafkaServerConfig(ctx context.Context, server graphqlclient.KafkaServerConfigInput) error
	ReportAppliedIntents(ctx context.Context, namespace string, clientIntentsList otterizev1alpha1.ClientIntentsList) error
	ReportIntentsOperatorConfiguration(ctx context.Context, enableEnforcement bool) error
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
	namespace string,
	clientIntentsList otterizev1alpha1.ClientIntentsList) error {

	intentsInput, err := clientIntentsList.FormatAsOtterizeIntents()
	if err != nil {
		return err
	}

	_, err = graphqlclient.ReportAppliedKubernetesIntents(ctx, c.client, namespace, intentsInput)
	if err != nil {
		return err
	}
	logrus.Infof("New intents count for namespace %s: %d", namespace, len(intentsInput))
	return nil
}

func (c *CloudClientImpl) ReportIntentsOperatorConfiguration(
	ctx context.Context,
	enableEnforcement bool) error {
	_, err := graphqlclient.ReportIntentsOperatorConfiguration(ctx, c.client, graphqlclient.IntentsOperatorConfigurationInput{
		EnableEnforcement: enableEnforcement,
	})
	if err != nil {
		return err
	}
	logrus.Infof("Intents operator configuration reported to cloud successfully")
	return nil
}
