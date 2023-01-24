package otterizecloudclient

import (
	"context"
	"errors"
	"fmt"
	"github.com/Khan/genqlient/graphql"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

func NewClient(ctx context.Context) (graphql.Client, bool, error) {
	clientID := viper.GetString(ApiClientIdKey)
	secret := viper.GetString(ApiClientSecretKey)
	apiAddress := viper.GetString(OtterizeAPIAddressKey)
	if clientID == "" && secret == "" {
		return nil, false, nil
	}
	if clientID == "" {
		return nil, true, errors.New("missing cloud integration client ID")
	}
	if secret == "" {
		return nil, true, errors.New("missing cloud integration secret")
	}

	cfg := clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: secret,
		TokenURL:     fmt.Sprintf("%s/auth/tokens/token", apiAddress),
		AuthStyle:    oauth2.AuthStyleInParams,
	}

	tokenSrc := cfg.TokenSource(ctx)
	graphqlUrl := fmt.Sprintf("%s/graphql/v1beta", apiAddress)
	httpClient := oauth2.NewClient(ctx, tokenSrc)

	return graphql.NewClient(graphqlUrl, httpClient), true, nil
}
