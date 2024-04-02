package operator_cloud_client

import (
	"context"
	"github.com/Khan/genqlient/graphql"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/sirupsen/logrus"
)

type CloudClient interface {
	ReportKafkaServerConfig(ctx context.Context, namespace string, servers []graphqlclient.KafkaServerConfigInput) error
	ReportAppliedIntents(ctx context.Context, namespace *string, intents []*graphqlclient.IntentInput) error
	ReportIntentsOperatorConfiguration(ctx context.Context, config graphqlclient.IntentsOperatorConfigurationInput) error
	ReportComponentStatus(ctx context.Context, component graphqlclient.ComponentType)
	ReportNetworkPolicies(ctx context.Context, namespace string, policies []graphqlclient.NetworkPolicyInput) error
	ReportProtectedServices(ctx context.Context, namespace string, protectedServices []graphqlclient.ProtectedServiceInput) error
	ReportOSSClusterID(ctx context.Context, clusterID string) error
}

type CloudClientImpl struct {
	client graphql.Client
}

func NewClient(ctx context.Context) (CloudClient, bool, error) {
	client, ok, err := otterizecloudclient.NewClient(ctx)
	if !ok {
		return nil, false, nil
	} else if err != nil {
		return nil, true, errors.Wrap(err)
	}

	return &CloudClientImpl{client: client}, true, nil
}

func (c *CloudClientImpl) ReportKafkaServerConfig(ctx context.Context, namespace string, servers []graphqlclient.KafkaServerConfigInput) error {
	_, err := graphqlclient.ReportKafkaServerConfig(ctx, c.client, namespace, servers)
	if err != nil {
		return errors.Wrap(err)
	}

	logrus.Infof("Successfully reported KafkaServerConfig, count: %d", len(servers))

	return nil
}

func (c *CloudClientImpl) ReportAppliedIntents(
	ctx context.Context,
	namespace *string,
	intents []*graphqlclient.IntentInput) error {

	_, err := graphqlclient.ReportAppliedKubernetesIntents(ctx, c.client, namespace, intents)
	if err != nil {
		return errors.Wrap(err)
	}

	logrus.Infof("New intents count for namespace %s: %d", *namespace, len(intents))
	return nil
}

func (c *CloudClientImpl) ReportIntentsOperatorConfiguration(
	ctx context.Context,
	config graphqlclient.IntentsOperatorConfigurationInput) error {
	_, err := graphqlclient.ReportIntentsOperatorConfiguration(ctx, c.client, config)
	if err != nil {
		return errors.Wrap(err)
	}
	logrus.Info("Intents operator configuration reported to cloud successfully")
	return nil
}

func (c *CloudClientImpl) ReportComponentStatus(ctx context.Context, component graphqlclient.ComponentType) {
	_, err := graphqlclient.ReportComponentStatus(ctx, c.client, component)
	if err != nil {
		logrus.WithError(err).Error("failed to reported component status Otterize cloud")
		return
	}
}

func (c *CloudClientImpl) ReportNetworkPolicies(ctx context.Context, namespace string, policies []graphqlclient.NetworkPolicyInput) error {
	logrus.WithField("namespace", namespace).
		WithField("count", len(policies)).
		Infof("Reporting network policies")

	_, err := graphqlclient.ReportNetworkPolicies(ctx, c.client, namespace, policies)
	return errors.Wrap(err)
}

func (c *CloudClientImpl) ReportProtectedServices(ctx context.Context, namespace string, protectedServices []graphqlclient.ProtectedServiceInput) error {
	logrus.WithField("namespace", namespace).
		WithField("count", len(protectedServices)).
		Infof("Reporting network policies")

	_, err := graphqlclient.ReportProtectedServicesSnapshot(ctx, c.client, namespace, protectedServices)
	return errors.Wrap(err)
}

func (c *CloudClientImpl) ReportOSSClusterID(ctx context.Context, clusterID string) error {
	logrus.Info("Reporting ClusterID to Otterize cloud")
	res, err := graphqlclient.ReportOSSClusterID(ctx, c.client, clusterID)
	if err != nil {
		return errors.Wrap(err)
	}
	if res.ReportOSSClusterId {
		logrus.Info("OSS ClusterID set for integration")
	}
	return nil
}
