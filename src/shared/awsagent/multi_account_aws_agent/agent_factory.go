package multi_account_aws_agent

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/otterize/intents-operator/src/shared/awsagent"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/spf13/viper"
	"k8s.io/client-go/util/keyutil"
	"os"
	"path"
)

func MakeAgentsFromAccountList[T any](ctx context.Context, accounts []operatorconfig.AWSAccount, agentFactory func(awsAgent *awsagent.Agent) T, awsOptionsInput []awsagent.Option) (map[string]T, error) {
	agents := make(map[string]T)

	certChain, err := os.ReadFile(path.Join(viper.GetString(operatorconfig.AWSRolesAnywhereCertDirKey), viper.GetString(operatorconfig.AWSRolesAnywhereCertChainFilenameKey)))
	if err != nil {
		return nil, errors.Errorf("failed to read cert chain file: %w", err)
	}

	cert, err := x509.ParseCertificate(certChain)
	if err != nil {
		return nil, errors.Errorf("failed to parse certificate: %w", err)
	}

	privKey, err := os.ReadFile(path.Join(viper.GetString(operatorconfig.AWSRolesAnywhereCertDirKey), viper.GetString(operatorconfig.AWSRolesAnywherePrivKeyFilenameKey)))
	if err != nil {
		return nil, errors.Errorf("failed to read private key file: %w", err)
	}
	privKeyParsed, err := keyutil.ParsePrivateKeyPEM(privKey)
	if err != nil {
		return nil, errors.Errorf("failed to parse private key: %w", err)
	}
	privKeyEcdsa, ok := privKeyParsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key must be an ECDSA key")
	}
	for _, account := range accounts {
		awsOptions := make([]awsagent.Option, len(awsOptionsInput))
		copy(awsOptions, awsOptionsInput)
		awsConfig, err := config.LoadDefaultConfig(ctx,
			config.WithCredentialsProvider(credentialsProvider(
				privKeyEcdsa,
				cert,
				account)))
		if err != nil {
			return nil, errors.Errorf("failed to initialize AWS config for account with role '%s': %w", account.RoleARN, err)
		}
		awsOptions = append(awsOptions, awsagent.WithRolesAnywhere(
			account.TrustAnchorARN,
			account.TrustDomain,
			viper.GetString(operatorconfig.AWSRolesAnywhereClusterNameKey),
		), awsagent.WithAWSConfig(awsConfig))
		awsAgent, err := awsagent.NewAWSAgent(ctx, awsOptions...)
		if err != nil {
			return nil, errors.Errorf("failed to initialize AWS agent for account with role '%s': %w", account.RoleARN, err)
		}
		specializedAgent := agentFactory(awsAgent)
		agents[awsAgent.AccountID] = specializedAgent
	}
	return agents, nil
}
