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
	stsClient   *sts.Client
	iamClient   *iam.Client
	accountID   string
	oidcURL     string
	clusterName string
}

func NewAWSAgent(
	ctx context.Context,
) (*Agent, error) {
	logrus.Info("Initializing AWS Intents agent")

	awsConfig, err := config.LoadDefaultConfig(ctx)

	if err != nil {
		return nil, fmt.Errorf("could not load AWS config")
	}

	currentCluster, err := getCurrentEKSCluster(ctx, awsConfig)

	if err != nil {
		return nil, fmt.Errorf("failed to get current EKS cluster: %w", err)
	}

	OIDCURL := *currentCluster.Identity.Oidc.Issuer

	iamClient := iam.NewFromConfig(awsConfig)
	stsClient := sts.NewFromConfig(awsConfig)

	callerIdent, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})

	if err != nil {
		return nil, fmt.Errorf("unable to get STS caller identity: %w", err)
	}

	return &Agent{
		stsClient:   stsClient,
		iamClient:   iamClient,
		accountID:   *callerIdent.Account,
		oidcURL:     strings.Split(OIDCURL, "://")[1],
		clusterName: *currentCluster.Name,
	}, nil
}

func getEKSClusterName(ctx context.Context, config aws.Config) (string, error) {
	if viper.IsSet(operatorconfig.EKSClusterNameKey) {
		return viper.GetString(operatorconfig.EKSClusterNameKey), nil
	}

	imdsClient := imds.NewFromConfig(config)
	output, err := imdsClient.GetInstanceIdentityDocument(ctx, &imds.GetInstanceIdentityDocumentInput{})

	if err != nil {
		return "", err
	}

	ec2Client := ec2.NewFromConfig(config)
	describeInstancesOutput, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{output.InstanceID},
	})

	if err != nil {
		return "", err
	}

	clusterName, found := lo.Find(describeInstancesOutput.Reservations[0].Instances[0].Tags, func(item types.Tag) bool {
		return *item.Key == "aws:eks:cluster-name"
	})

	if !found {
		return "", errors.New("EKS cluster name tag not found")
	}

	return *clusterName.Value, nil
}

func getCurrentEKSCluster(ctx context.Context, config aws.Config) (*eksTypes.Cluster, error) {
	clusterName, err := getEKSClusterName(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("could not get EKS cluster name: %w", err)
	}

	eksClient := eks.NewFromConfig(config)

	describeClusterOutput, err := eksClient.DescribeCluster(ctx, &eks.DescribeClusterInput{Name: &clusterName})

	if err != nil {
		return nil, err
	}

	return describeClusterOutput.Cluster, nil
}
