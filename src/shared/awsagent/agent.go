package awsagent

import (
	"context"
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
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"strings"
)

type IAMClient interface {
	ListAttachedRolePolicies(ctx context.Context, i *iam.ListAttachedRolePoliciesInput, opts ...func(*iam.Options)) (*iam.ListAttachedRolePoliciesOutput, error)
	DeleteRole(ctx context.Context, i *iam.DeleteRoleInput, opts ...func(*iam.Options)) (*iam.DeleteRoleOutput, error)
	CreateRole(ctx context.Context, i *iam.CreateRoleInput, opts ...func(*iam.Options)) (*iam.CreateRoleOutput, error)
	GetRole(ctx context.Context, i *iam.GetRoleInput, opts ...func(*iam.Options)) (*iam.GetRoleOutput, error)
	GetPolicy(ctx context.Context, i *iam.GetPolicyInput, opts ...func(*iam.Options)) (*iam.GetPolicyOutput, error)
	ListEntitiesForPolicy(ctx context.Context, i *iam.ListEntitiesForPolicyInput, opts ...func(*iam.Options)) (*iam.ListEntitiesForPolicyOutput, error)
	DetachRolePolicy(ctx context.Context, i *iam.DetachRolePolicyInput, opts ...func(*iam.Options)) (*iam.DetachRolePolicyOutput, error)
	ListPolicyVersions(ctx context.Context, i *iam.ListPolicyVersionsInput, opts ...func(*iam.Options)) (*iam.ListPolicyVersionsOutput, error)
	DeletePolicyVersion(ctx context.Context, i *iam.DeletePolicyVersionInput, opts ...func(*iam.Options)) (*iam.DeletePolicyVersionOutput, error)
	DeletePolicy(ctx context.Context, i *iam.DeletePolicyInput, opts ...func(*iam.Options)) (*iam.DeletePolicyOutput, error)
	PutRolePolicy(ctx context.Context, i *iam.PutRolePolicyInput, opts ...func(*iam.Options)) (*iam.PutRolePolicyOutput, error)
	CreatePolicy(ctx context.Context, i *iam.CreatePolicyInput, opts ...func(*iam.Options)) (*iam.CreatePolicyOutput, error)
	CreatePolicyVersion(ctx context.Context, i *iam.CreatePolicyVersionInput, opts ...func(*iam.Options)) (*iam.CreatePolicyVersionOutput, error)
	TagPolicy(ctx context.Context, i *iam.TagPolicyInput, opts ...func(*iam.Options)) (*iam.TagPolicyOutput, error)
	AttachRolePolicy(ctx context.Context, i *iam.AttachRolePolicyInput, opts ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error)
}

type Agent struct {
	iamClient   IAMClient
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
		iamClient:   iamClient,
		accountID:   *callerIdent.Account,
		oidcURL:     strings.Split(OIDCURL, "://")[1],
		clusterName: *currentCluster.Name,
	}, nil
}

func getEKSClusterName(ctx context.Context, config aws.Config) (string, error) {
	if viper.IsSet(operatorconfig.EKSClusterNameOverrideKey) {
		return viper.GetString(operatorconfig.EKSClusterNameOverrideKey), nil
	}

	imdsClient := imds.NewFromConfig(config)
	output, err := imdsClient.GetInstanceIdentityDocument(ctx, &imds.GetInstanceIdentityDocumentInput{})

	if err != nil {
		return "", errors.Wrap(err)
	}

	ec2Client := ec2.NewFromConfig(config)
	describeInstancesOutput, err := ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{output.InstanceID},
	})

	if err != nil {
		return "", errors.Wrap(err)
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
		return nil, errors.Wrap(err)
	}

	return describeClusterOutput.Cluster, nil
}
