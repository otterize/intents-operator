package awsagent

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	eksTypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
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
) (*Agent, error) {
	logrus.Info("Initializing AWS Intents agent")

	awsConfig, err := config.LoadDefaultConfig(ctx)

	if err != nil {
		return nil, fmt.Errorf("could not load AWS config")
	}

	oidcUrl := viper.GetString(operatorconfig.ClusterOIDCProviderUrlKey)
	if !viper.IsSet(operatorconfig.ClusterOIDCProviderUrlKey) {
		oidcUrl, err = tryRetrieveOIDCURL(ctx, awsConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve OIDC URL from AWS API: %w", err)
		}
	}

	iamClient := iam.NewFromConfig(awsConfig)
	stsClient := sts.NewFromConfig(awsConfig)

	callerIdent, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})

	if err != nil {
		return nil, fmt.Errorf("unable to get STS caller identity: %w", err)
	}

	return &Agent{
		stsClient: stsClient,
		iamClient: iamClient,
		accountId: *callerIdent.Account,
		oidcUrl:   strings.Split(oidcUrl, "://")[1],
	}, nil
}

func tryRetrieveOIDCURL(ctx context.Context, awsConfig aws.Config) (string, error) {
	currentCluster, err := getCurrentEKSCluster(ctx, awsConfig)

	if err != nil {
		return "", err
	}

	OIDCURL := *currentCluster.Identity.Oidc.Issuer

	logrus.Infof("Retrieved OIDC URL for current EKS cluster: %s", OIDCURL)

	return OIDCURL, nil
}

func getCurrentEKSCluster(ctx context.Context, config aws.Config) (*eksTypes.Cluster, error) {
	imdsClient := imds.NewFromConfig(config)
	output, err := imdsClient.GetInstanceIdentityDocument(ctx, &imds.GetInstanceIdentityDocumentInput{})

	if err != nil {
		return nil, err
	}

	ec2Client := ec2.NewFromConfig(config)
	describeInstancesOutput, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{output.InstanceID},
	})

	if err != nil {
		return nil, err
	}

	clusterName, found := lo.Find(describeInstancesOutput.Reservations[0].Instances[0].Tags, func(item types.Tag) bool {
		return *item.Key == "aws:eks:cluster-name"
	})

	if !found {
		return nil, errors.New("EKS cluster name tag not found")
	}

	eksClient := eks.NewFromConfig(config)

	describeClusterOutput, err := eksClient.DescribeCluster(ctx, &eks.DescribeClusterInput{Name: clusterName.Value})

	if err != nil {
		return nil, err
	}

	return describeClusterOutput.Cluster, nil
}
