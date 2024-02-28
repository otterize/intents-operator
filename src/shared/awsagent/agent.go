package awsagent

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	eksTypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/rolesanywhere"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"strings"
	"sync"
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

type RolesAnywhereClient interface {
	EnableProfile(ctx context.Context, params *rolesanywhere.EnableProfileInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.EnableProfileOutput, error)
	UpdateCrl(ctx context.Context, params *rolesanywhere.UpdateCrlInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.UpdateCrlOutput, error)
	ImportCrl(ctx context.Context, params *rolesanywhere.ImportCrlInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.ImportCrlOutput, error)
	TagResource(ctx context.Context, params *rolesanywhere.TagResourceInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.TagResourceOutput, error)
	PutNotificationSettings(ctx context.Context, params *rolesanywhere.PutNotificationSettingsInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.PutNotificationSettingsOutput, error)
	DisableTrustAnchor(ctx context.Context, params *rolesanywhere.DisableTrustAnchorInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.DisableTrustAnchorOutput, error)
	GetCrl(ctx context.Context, params *rolesanywhere.GetCrlInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.GetCrlOutput, error)
	ListTrustAnchors(ctx context.Context, params *rolesanywhere.ListTrustAnchorsInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.ListTrustAnchorsOutput, error)
	EnableCrl(ctx context.Context, params *rolesanywhere.EnableCrlInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.EnableCrlOutput, error)
	ListCrls(ctx context.Context, params *rolesanywhere.ListCrlsInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.ListCrlsOutput, error)
	ListProfiles(ctx context.Context, params *rolesanywhere.ListProfilesInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.ListProfilesOutput, error)
	ListTagsForResource(ctx context.Context, params *rolesanywhere.ListTagsForResourceInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.ListTagsForResourceOutput, error)
	UntagResource(ctx context.Context, params *rolesanywhere.UntagResourceInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.UntagResourceOutput, error)
	DisableCrl(ctx context.Context, params *rolesanywhere.DisableCrlInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.DisableCrlOutput, error)
	ResetNotificationSettings(ctx context.Context, params *rolesanywhere.ResetNotificationSettingsInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.ResetNotificationSettingsOutput, error)
	ListSubjects(ctx context.Context, params *rolesanywhere.ListSubjectsInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.ListSubjectsOutput, error)
	UpdateTrustAnchor(ctx context.Context, params *rolesanywhere.UpdateTrustAnchorInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.UpdateTrustAnchorOutput, error)
	CreateProfile(ctx context.Context, params *rolesanywhere.CreateProfileInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.CreateProfileOutput, error)
	GetSubject(ctx context.Context, params *rolesanywhere.GetSubjectInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.GetSubjectOutput, error)
	GetProfile(ctx context.Context, params *rolesanywhere.GetProfileInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.GetProfileOutput, error)
	Options() rolesanywhere.Options
	CreateTrustAnchor(ctx context.Context, params *rolesanywhere.CreateTrustAnchorInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.CreateTrustAnchorOutput, error)
	DeleteCrl(ctx context.Context, params *rolesanywhere.DeleteCrlInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.DeleteCrlOutput, error)
	EnableTrustAnchor(ctx context.Context, params *rolesanywhere.EnableTrustAnchorInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.EnableTrustAnchorOutput, error)
	DeleteTrustAnchor(ctx context.Context, params *rolesanywhere.DeleteTrustAnchorInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.DeleteTrustAnchorOutput, error)
	UpdateProfile(ctx context.Context, params *rolesanywhere.UpdateProfileInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.UpdateProfileOutput, error)
	DisableProfile(ctx context.Context, params *rolesanywhere.DisableProfileInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.DisableProfileOutput, error)
	GetTrustAnchor(ctx context.Context, params *rolesanywhere.GetTrustAnchorInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.GetTrustAnchorOutput, error)
	DeleteProfile(ctx context.Context, params *rolesanywhere.DeleteProfileInput, optFns ...func(*rolesanywhere.Options)) (*rolesanywhere.DeleteProfileOutput, error)
}

type Agent struct {
	iamClient           IAMClient
	rolesAnywhereClient RolesAnywhereClient
	accountID           string
	oidcURL             string
	clusterName         string
	profileNameToId     map[string]string
	profileCacheOnce    sync.Once
	trustAnchorArn      string
}

func NewAWSAgent(
	ctx context.Context,
	trustAnchorArn string,
) (*Agent, error) {
	logrus.Info("Initializing AWS Intents agent")

	awsConfig, err := config.LoadDefaultConfig(ctx)

	if err != nil {
		return nil, errors.Errorf("could not load AWS config")
	}

	iamClient := iam.NewFromConfig(awsConfig)
	rolesAnywhereClient := rolesanywhere.NewFromConfig(awsConfig)
	stsClient := sts.NewFromConfig(awsConfig)

	agent := &Agent{
		iamClient:           iamClient,
		rolesAnywhereClient: rolesAnywhereClient,
		profileNameToId:     make(map[string]string),
		trustAnchorArn:      trustAnchorArn,
	}

	if trustAnchorArn == "" {

		currentCluster, err := getCurrentEKSCluster(ctx, awsConfig)

		if err != nil {
			return nil, errors.Errorf("failed to get current EKS cluster: %w", err)
		}
		agent.clusterName = *currentCluster.Name

		OIDCURL := *currentCluster.Identity.Oidc.Issuer
		agent.oidcURL = strings.Split(OIDCURL, "://")[1]
	}

	callerIdent, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})

	if err != nil {
		return nil, errors.Errorf("unable to get STS caller identity: %w", err)
	}
	agent.accountID = *callerIdent.Account

	return agent, nil
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
		return nil, errors.Errorf("could not get EKS cluster name: %w", err)
	}

	eksClient := eks.NewFromConfig(config)

	describeClusterOutput, err := eksClient.DescribeCluster(ctx, &eks.DescribeClusterInput{Name: &clusterName})

	if err != nil {
		return nil, errors.Wrap(err)
	}

	return describeClusterOutput.Cluster, nil
}
