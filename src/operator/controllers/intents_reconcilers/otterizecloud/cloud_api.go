package otterizecloud

import (
	"context"
	"github.com/Khan/genqlient/graphql"
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/otterizecloud/graphql_clients/kubernetes"
	"github.com/otterize/intents-operator/src/shared/otterizecloudclient"
	"github.com/sirupsen/logrus"
)

type CloudApi interface {
	ReportKubernetesNamespace(ctx context.Context, namespace string) error
	ReportAppliedIntents(ctx context.Context, namespace string, clientIntentsList otterizev1alpha1.ClientIntentsList) error
}

type CloudApiImpl struct {
	client graphql.Client
}

func NewClient(ctx context.Context) (*CloudApiImpl, bool, error) {
	client, ok, err := otterizecloudclient.NewClient(ctx)
	if !ok {
		return nil, false, nil
	} else if err != nil {
		return nil, true, err
	}

	return &CloudApiImpl{client: client}, true, nil
}

func (c *CloudApiImpl) ReportKubernetesNamespace(ctx context.Context, namespace string) error {
	res, err := kubernetes.ReportKubernetesNamespace(ctx, c.client, namespace)
	if err != nil {
		return err
	}
	// Return value is bool - returns true if anything was updated in the DB
	if res.ReportKubernetesNamespace {
		logrus.Infof("Successfully reported namespace # %s # to Otterize cloud", namespace)
	}
	return nil
}

func (c *CloudApiImpl) ReportAppliedIntents(
	ctx context.Context,
	namespace string,
	clientIntentsList otterizev1alpha1.ClientIntentsList) error {

	intentsInput, err := clientIntentsList.FormatAsOtterizeIntents()
	if err != nil {
		return err
	}

	logrus.Infoln("Updating new intents counts", "Namespace", namespace, "Count", len(intentsInput))
	_, err = kubernetes.ReportAppliedKubernetesIntents(ctx, c.client, namespace, intentsInput)
	if err != nil {
		return err
	}
	return nil
}
