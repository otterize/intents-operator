package otterizecloudclient

import (
	"context"
	"errors"
	"fmt"
	"github.com/Khan/genqlient/graphql"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"net/http"
)

func NewClient(ctx context.Context) (graphql.Client, bool, error) {
	clientID := viper.GetString(ApiClientIdKey)
	secret := viper.GetString(ApiClientSecretKey)
	apiAddress := viper.GetString(OtterizeAPIAddressKey)
	clientTimeout := viper.GetDuration(CloudClientTimeoutKey)
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

	// Timeout for oauth token acquisition is set by http client passed to the token source context
	// See example 'Example (CustomHTTP)' in https://pkg.go.dev/golang.org/x/oauth2
	clientWithTimeout := &http.Client{Timeout: clientTimeout}
	ctxWithClient := context.WithValue(ctx, oauth2.HTTPClient, clientWithTimeout)

	tokenSrc := cfg.TokenSource(ctxWithClient)
	graphqlUrl := fmt.Sprintf("%s/graphql/v1beta", apiAddress)
	httpClient := oauth2.NewClient(ctxWithClient, tokenSrc)

	return graphql.NewClient(graphqlUrl, httpClient), true, nil
}
