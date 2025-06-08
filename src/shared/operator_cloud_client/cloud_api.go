package operator_cloud_client

import (
	"context"
	"github.com/Khan/genqlient/graphql"
	"github.com/otterize/intents-operator/src/shared/clusterutils"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
)

type CloudClient interface {
	ReportKafkaServerConfig(ctx context.Context, namespace string, servers []graphqlclient.KafkaServerConfigInput) error
	ReportAppliedIntents(ctx context.Context, namespace *string, intents []*graphqlclient.IntentInput) error
	ReportIntentsOperatorConfiguration(ctx context.Context, config graphqlclient.IntentsOperatorConfigurationInput)
	ReportComponentStatus(ctx context.Context, component graphqlclient.ComponentType)
	ReportNetworkPolicies(ctx context.Context, namespace string, policies []graphqlclient.NetworkPolicyInput) error
	ReportExternallyAccessibleServices(ctx context.Context, namespace string, services []graphqlclient.ExternallyAccessibleServiceInput) error
	ReportProtectedServices(ctx context.Context, namespace string, protectedServices []graphqlclient.ProtectedServiceInput) error
	ReportIntentEvents(ctx context.Context, events []graphqlclient.ClientIntentEventInput) error
	ReportClientIntentStatuses(ctx context.Context, statuses []graphqlclient.ClientIntentStatusInput) error
	ReportAppliedIntentsRequest(ctx context.Context, intents []*graphqlclient.IntentRequestInput) error
	GetAppliedIntentsRequestsStatus(ctx context.Context, resourceGenerations []graphqlclient.IntentRequestResourceGeneration) ([]AppliedIntentsRequestStatus, error)
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

	clusterID, err := clusterutils.GetClusterUID(ctx)
	if err != nil {
		return errors.Wrap(err)
	}

	_, err = graphqlclient.ReportAppliedKubernetesIntents(ctx, c.client, namespace, intents, &clusterID)
	if err != nil {
		return errors.Wrap(err)
	}

	logrus.Debugf("New intents count for namespace %s: %d", *namespace, len(intents))
	return nil
}

func (c *CloudClientImpl) ReportIntentsOperatorConfiguration(
	ctx context.Context,
	config graphqlclient.IntentsOperatorConfigurationInput) {
	_, err := graphqlclient.ReportIntentsOperatorConfiguration(ctx, c.client, config)
	if err != nil {
		logrus.WithError(err).Error("failed to reported component status Otterize cloud")
		return
	}
	logrus.Info("Intents operator configuration reported to cloud successfully")
	return
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

func (c *CloudClientImpl) ReportExternallyAccessibleServices(ctx context.Context, namespace string, services []graphqlclient.ExternallyAccessibleServiceInput) error {
	_, err := graphqlclient.ReportExternallyAccessibleServices(ctx, c.client, namespace, services)
	if err != nil {
		return errors.Wrap(err)
	}

	logrus.Debugf("Successfully reported ExternallyAccessibleServices, count: %d", len(services))

	return nil
}

func (c *CloudClientImpl) ReportProtectedServices(ctx context.Context, namespace string, protectedServices []graphqlclient.ProtectedServiceInput) error {
	logrus.WithField("namespace", namespace).
		WithField("count", len(protectedServices)).
		Info("Reporting protected services")

	_, err := graphqlclient.ReportProtectedServicesSnapshot(ctx, c.client, namespace, protectedServices)
	return errors.Wrap(err)
}

func (c *CloudClientImpl) ReportIntentEvents(ctx context.Context, events []graphqlclient.ClientIntentEventInput) error {
	_, err := graphqlclient.ReportClientIntentEvents(ctx, c.client, events)
	return errors.Wrap(err)
}

func (c *CloudClientImpl) ReportClientIntentStatuses(ctx context.Context, statuses []graphqlclient.ClientIntentStatusInput) error {
	_, err := graphqlclient.ReportClientIntentStatuses(ctx, c.client, statuses)
	return errors.Wrap(err)
}

func (c *CloudClientImpl) GetAppliedIntentsRequestsStatus(ctx context.Context, resourceGenerations []graphqlclient.IntentRequestResourceGeneration) ([]AppliedIntentsRequestStatus, error) {
	result, err := graphqlclient.GetAppliedIntentsRequestStatus(ctx, c.client, resourceGenerations)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	return translateAppliedIntentsRequestsStatusModel(result.GetSyncPendingRequestStatuses()), nil
}

func (c *CloudClientImpl) ReportAppliedIntentsRequest(ctx context.Context, intents []*graphqlclient.IntentRequestInput) error {
	_, err := graphqlclient.ReportAppliedIntentsRequest(ctx, c.client, lo.Map(intents, func(intent *graphqlclient.IntentRequestInput, _ int) graphqlclient.IntentRequestInput {
		return *intent
	}))
	return errors.Wrap(err)
}
