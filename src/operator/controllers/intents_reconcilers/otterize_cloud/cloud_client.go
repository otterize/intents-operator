package otterize_cloud

import (
	"context"
	"fmt"
	"github.com/Khan/genqlient/graphql"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/otterize_cloud/graphql_clients/environments"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"strings"
)

const ErrEnvNotFound = "environment not found"

// CloudClient Here we add all the auto-generate functions from graphql that we want to use
// It will allow us to mock all of them and create lightweight tests without need any client-server games
type CloudClient interface {
	GetOrCreateOtterizeEnv(ctx context.Context) (string, error)
	UpdateOtterizeEnvWithNamespace(ctx context.Context, envID, namespace string) error
	ReportKafkaServerConfig(ctx context.Context, environment string, source string, servers interface{}) error
}

type CloudClientImpl struct {
	client      graphql.Client
	tokenSource oauth2.TokenSource
}

func NewClient(otterizeClientID string, otterizeClientSecret string, apiAddress string) CloudClient {
	cfg := clientcredentials.Config{
		ClientID:     otterizeClientID,
		ClientSecret: otterizeClientSecret,
		TokenURL:     fmt.Sprintf("%s/auth/tokens/token", apiAddress),
		AuthStyle:    oauth2.AuthStyleInParams,
	}

	ctx := context.Background()
	client := graphql.NewClient(
		fmt.Sprintf("%s/api/graphql/v1", apiAddress),
		oauth2.NewClient(ctx, cfg.TokenSource(ctx)))

	tokenSource := cfg.TokenSource(context.Background())
	if tokenSource == nil {
		return nil
	}

	return &CloudClientImpl{
		client:      client,
		tokenSource: tokenSource,
	}
}

func (c *CloudClientImpl) GetOrCreateOtterizeEnv(ctx context.Context) (string, error) {
	env, err := environments.KubernetesEnvironment(ctx, c.client)
	if err != nil {
		if strings.Contains(err.Error(), ErrEnvNotFound) {
			logrus.Infof("No environment found associated with token. Creating new environment")
			env, err := environments.CreateKubernetesEnvironment(ctx, c.client)
			if err != nil {
				return "", err
			}
			return env.Me.CreateKubernetesEnvironment.GetId(), nil
		} else {
			return "", err
		}
	}
	return env.Me.KubernetesEnvironment.GetId(), nil
}

func (c *CloudClientImpl) UpdateOtterizeEnvWithNamespace(ctx context.Context, envID, namespace string) error {
	env, err := environments.AddNamespacesToEnv(ctx, c.client, envID, []string{namespace})
	if err != nil {
		return err
	}

	logrus.Infof("Added namespace %s to Otterize env %s", namespace, env.AddEnvironmentNamespaces.GetName())
	return nil
}

func (c *CloudClientImpl) ReportKafkaServerConfig(_ context.Context, _ string, _ string, _ interface{}) error {
	// TODO: Implemented once I sync the graphql, but it will just be direct call to the generated graphql function
	return nil
}
