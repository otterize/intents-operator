package rolesanywhere_creds

import (
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
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
	"k8s.io/client-go/util/keyutil"
	"net/http"
	"os"
	"runtime"
	"time"
)

func CredentialsProvider(keyPath string, certPath string, account operatorconfig.AWSAccount) awsv2.CredentialsProviderFunc {
	return func(context.Context) (awsv2.Credentials, error) {
		return getCredentials(keyPath, certPath, account.RoleARN, account)
	}
}

// Convert certificate to string, so that it can be present in the HTTP request header
func certificateChainToString(certificateChainPEM []byte) string {
	return base64.StdEncoding.EncodeToString(certificateChainPEM)
}

func getCredentials(keyPath string, certPath string, roleARN string, account operatorconfig.AWSAccount) (awsv2.Credentials, error) {
	chainData, err := os.ReadFile(certPath)
	if err != nil {
		return awsv2.Credentials{}, errors.Errorf("failed to read cert chain file: %w", err)
	}

	certs := make([]x509.Certificate, 0)
	for block, rest := pem.Decode(chainData); block != nil; block, rest = pem.Decode(rest) {
		crt, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return awsv2.Credentials{}, errors.Errorf("parsing issued certificate: %w", err)
		}

		certs = append(certs, *crt)
	}

	if len(certs) == 0 {
		return awsv2.Credentials{}, errors.Errorf("no certificates found in chain")
	}

	leaf := certs[0]
	intermediates := certs[1:]
	if len(intermediates) == 0 {
		// awssh.CreateSignFunction compares the intermediates slice to nil to check if the 'X-Amz-X509-Chain' header should be added to the request,
		// and starting at some point, the AWS API started rejecting requests with an existing-yet-empty 'X-Amz-X509-Chain' header.
		// To avoid this, we set the intermediates slice to nil if it's empty.
		intermediates = nil
	}

	privKey, err := os.ReadFile(keyPath)
	if err != nil {
		return awsv2.Credentials{}, errors.Errorf("failed to read private key file: %w", err)
	}
	privKeyParsed, err := keyutil.ParsePrivateKeyPEM(privKey)
	if err != nil {
		return awsv2.Credentials{}, errors.Errorf("failed to parse private key: %w", err)
	}
	privKeyEcdsa, ok := privKeyParsed.(*ecdsa.PrivateKey)
	if !ok {
		return awsv2.Credentials{}, errors.New("private key must be an ECDSA key")
	}

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

	rolesAnywhereClient.Handlers.Sign.PushBackNamed(request.NamedHandler{Name: "v4x509.SignRequestHandler", Fn: awssh.CreateSignFunction(*privKeyEcdsa, leaf, intermediates)})

	createSessionRequest := rolesanywhere.CreateSessionInput{
		Cert:               lo.ToPtr(certificateChainToString(chainData)),
		ProfileArn:         &account.ProfileARN,
		TrustAnchorArn:     &account.TrustAnchorARN,
		DurationSeconds:    lo.ToPtr(int64(43200)),
		InstanceProperties: nil,
		RoleArn:            &roleARN,
		SessionName:        nil,
	}
	output, err := rolesAnywhereClient.CreateSession(&createSessionRequest)
	if err != nil {
		return awsv2.Credentials{}, errors.Errorf("failed to create session: %w", err)
	}

	if len(output.CredentialSet) == 0 {
		return awsv2.Credentials{}, errors.New("unable to obtain temporary security credentials from CreateSession")
	}

	credentials := output.CredentialSet[0].Credentials
	expirationAsTime, err := time.Parse(time.RFC3339, *credentials.Expiration)
	if err != nil {
		return awsv2.Credentials{}, errors.Errorf("failed to parse expiration time: %w", err)
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
