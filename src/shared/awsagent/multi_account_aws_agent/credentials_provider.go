package multi_account_aws_agent

import (
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	awssh "github.com/aws/rolesanywhere-credential-helper/aws_signing_helper"
	"github.com/aws/rolesanywhere-credential-helper/rolesanywhere"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/samber/lo"
	"net/http"
	"runtime"
	"strings"
	"time"
)

func credentialsProvider(key *ecdsa.PrivateKey, cert *x509.Certificate, account operatorconfig.AWSAccount) awsv2.CredentialsProviderFunc {
	return func(context.Context) (awsv2.Credentials, error) {
		return getCredentials(key, cert, account.RoleARN, account)
	}
}

// Convert certificate to string, so that it can be present in the HTTP request header
func certificateToString(certificate *x509.Certificate) string {
	return base64.StdEncoding.EncodeToString(certificate.Raw)
}

// Convert certificate chain to string, so that it can be pressent in the HTTP request header
func certificateChainToString(certificateChain []*x509.Certificate) string {
	var x509ChainString strings.Builder
	for i, certificate := range certificateChain {
		x509ChainString.WriteString(certificateToString(certificate))
		if i != len(certificateChain)-1 {
			x509ChainString.WriteString(",")
		}
	}
	return x509ChainString.String()
}

func getCredentials(key *ecdsa.PrivateKey, cert *x509.Certificate, roleARN string, account operatorconfig.AWSAccount) (awsv2.Credentials, error) {
	// assign values to region and endpoint if they haven't already been assigned
	trustAnchorArn, err := arn.Parse(account.TrustAnchorARN)
	if err != nil {
		return awsv2.Credentials{}, err
	}

	profileArn, err := arn.Parse(account.ProfileARN)
	if err != nil {
		return awsv2.Credentials{}, err

	}

	if trustAnchorArn.Region != profileArn.Region {
		return awsv2.Credentials{}, errors.New("trust anchor and profile must be in the same region")
	}

	session, err := session.NewSession()
	if err != nil {
		return awsv2.Credentials{}, err
	}

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}}
	config := aws.NewConfig().WithRegion(trustAnchorArn.Region).WithHTTPClient(client).WithLogLevel(aws.LogOff)
	rolesAnywhereClient := rolesanywhere.New(session, config)

	rolesAnywhereClient.Handlers.Build.RemoveByName("core.SDKVersionUserAgentHandler")
	rolesAnywhereClient.Handlers.Build.PushBackNamed(request.NamedHandler{Name: "v4x509.CredHelperUserAgentHandler", Fn: request.MakeAddToUserAgentHandler("otterize", runtime.Version(), runtime.GOOS, runtime.GOARCH)})
	rolesAnywhereClient.Handlers.Sign.Clear()
	// FIXME is the cert chain needed here?
	rolesAnywhereClient.Handlers.Sign.PushBackNamed(request.NamedHandler{Name: "v4x509.SignRequestHandler", Fn: awssh.CreateSignFunction(*key, *cert, nil)})

	certChainPEM := certificateToString(cert)
	createSessionRequest := rolesanywhere.CreateSessionInput{
		Cert:               &certChainPEM,
		ProfileArn:         &account.ProfileARN,
		TrustAnchorArn:     &account.TrustAnchorARN,
		DurationSeconds:    lo.ToPtr(int64(43200)),
		InstanceProperties: nil,
		RoleArn:            &roleARN,
		SessionName:        nil,
	}
	output, err := rolesAnywhereClient.CreateSession(&createSessionRequest)
	if err != nil {
		return awsv2.Credentials{}, fmt.Errorf("failed to create session: %w", err)
	}

	if len(output.CredentialSet) == 0 {
		return awsv2.Credentials{}, errors.New("unable to obtain temporary security credentials from CreateSession")
	}

	credentials := output.CredentialSet[0].Credentials
	expirationAsTime, err := time.Parse(time.RFC3339, *credentials.Expiration)
	if err != nil {
		return awsv2.Credentials{}, fmt.Errorf("failed to parse expiration time: %w", err)
	}
	finalCredentials := awsv2.Credentials{
		AccessKeyID:     *credentials.AccessKeyId,
		SecretAccessKey: *credentials.SecretAccessKey,
		SessionToken:    *credentials.SessionToken,
		CanExpire:       true,
		Expires:         expirationAsTime,
	}
	return finalCredentials, nil
}
