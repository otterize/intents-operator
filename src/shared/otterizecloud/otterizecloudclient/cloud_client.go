package otterizecloudclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/Khan/genqlient/graphql"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"net/http"
	"os"
)

func NewClient(ctx context.Context) (graphql.Client, bool, error) {
	clientID := viper.GetString(ApiClientIdKey)
	secret := viper.GetString(ApiClientSecretKey)
	apiAddress := viper.GetString(OtterizeAPIAddressKey)
	clientTimeout := viper.GetDuration(CloudClientTimeoutKey)
	extraCAPEMPaths := viper.GetStringSlice(OtterizeAPIExtraCAPEMPathsKey)
	if clientID == "" && secret == "" {
		return nil, false, nil
	}
	if clientID == "" {
		return nil, false, errors.New("missing cloud integration client ID")
	}
	if secret == "" {
		return nil, false, errors.New("missing cloud integration secret")
	}

	cfg := clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: secret,
		TokenURL:     fmt.Sprintf("%s/auth/tokens/token", apiAddress),
		AuthStyle:    oauth2.AuthStyleInParams,
	}

	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		return nil, false, errors.Errorf("error loading root system cert pool: %w", err)
	}

	for _, path := range extraCAPEMPaths {
		logrus.Infof("Loading root CA from cert PEM file at '%s'", path)
		cert, err := os.ReadFile(path)
		if err != nil {
			return nil, false, errors.Errorf("error loading cert PEM file at '%s', trying to continue without it: %w", path, err)
		}

		if ok := rootCAs.AppendCertsFromPEM(cert); !ok {
			return nil, false, errors.Errorf("Failed appending cert PEM file at '%s', trying to continue without it", path)
		}
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: rootCAs}

	// Timeout for oauth token acquisition is set by http client passed to the token source context
	// See example 'Example (CustomHTTP)' in https://pkg.go.dev/golang.org/x/oauth2
	clientWithTimeout := &http.Client{Timeout: clientTimeout, Transport: transport}
	ctxWithClient := context.WithValue(ctx, oauth2.HTTPClient, clientWithTimeout)

	tokenSrc := cfg.TokenSource(ctxWithClient)
	graphqlUrl := fmt.Sprintf("%s/graphql/v1beta", apiAddress)
	httpClient := oauth2.NewClient(ctxWithClient, tokenSrc)

	return graphql.NewClient(graphqlUrl, httpClient), true, nil
}
