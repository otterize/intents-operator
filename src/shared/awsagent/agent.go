package awsagent

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/sirupsen/logrus"
	"strings"
)

type Agent struct {
	stsClient *sts.Client
	iamClient *iam.Client
	accountId string
	oidcUrl   string
}

func NewAWSAgent(
	ctx context.Context,
	oidcUrl string, // TODO: use eks.DescribeCluster to get the OIDC URL.
) *Agent {
	logrus.Info("AWS Intents agent - enabled")

	if oidcUrl == "" {
		logrus.Fatal("EKS cluster OIDC url not provided")
	}

	awsConfig, err := config.LoadDefaultConfig(context.Background())

	if err != nil {
		panic(err)
	}

	iamClient := iam.NewFromConfig(awsConfig)
	stsClient := sts.NewFromConfig(awsConfig)

	callerIdent, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})

	if err != nil {
		logrus.WithError(err).Panic("unable to get STS caller identity")
	}

	return &Agent{
		stsClient: stsClient,
		iamClient: iamClient,
		accountId: *callerIdent.Account,
		oidcUrl:   strings.Split(oidcUrl, "://")[1],
	}
}
