package awsagent

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	eksTypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"strings"
)

type Agent struct {
	stsClient   *sts.Client
	iamClient   *iam.Client
	accountId   string
	oidcUrl     string
	clusterName string
}

func NewAWSAgent(
	ctx context.Context,
	oidcUrlFromEnv string,
) *Agent {
	logrus.Info("AWS Intents agent - enabled")

	awsConfig, err := config.LoadDefaultConfig(context.Background())

	if err != nil {
		panic(err)
	}

	currentCluster, err := getCurrentEKSCluster(ctx, awsConfig)

	if err != nil {
		logrus.WithError(err).Fatal("failed to get current EKS cluster")
	}

	oidcUrl := ""

	if oidcUrlFromEnv == "" {
		oidcUrl = *currentCluster.Identity.Oidc.Issuer
		logrus.Infof("Retreieved OIDC URL for current EKS cluster: %s", oidcUrl)
	} else {
		logrus.Infof("OIDC URL provided from config: %s", oidcUrlFromEnv)
		oidcUrl = oidcUrlFromEnv
	}

	iamClient := iam.NewFromConfig(awsConfig)
	stsClient := sts.NewFromConfig(awsConfig)

	callerIdent, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})

	if err != nil {
		logrus.WithError(err).Panic("unable to get STS caller identity")
	}

	return &Agent{
		stsClient:   stsClient,
		iamClient:   iamClient,
		accountId:   *callerIdent.Account,
		oidcUrl:     strings.Split(oidcUrl, "://")[1],
		clusterName: *currentCluster.Name,
	}
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
