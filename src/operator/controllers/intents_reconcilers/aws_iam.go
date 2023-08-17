package intents_reconcilers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AWSIAMReconciler struct {
	client               client.Client
	scheme               *runtime.Scheme
	operatorPodName      string
	operatorPodNamespace string
	injectablerecorder.InjectableRecorder
}

func NewAWSIAMReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	operatorPodName string,
	operatorPodNamespace string,
) *AWSIAMReconciler {
	return &AWSIAMReconciler{
		client:               client,
		scheme:               scheme,
		operatorPodName:      operatorPodName,
		operatorPodNamespace: operatorPodNamespace,
	}
}

// policyDocument is our definition of our policies to be uploaded to IAM.
type policyDocument struct {
	Version   string
	Statement []statementEntry
}

// statementEntry will dictate what this policy will allow or not allow.
type statementEntry struct {
	Effect    string            `json:"Effect,omitempty"`
	Action    []string          `json:"Action,omitempty"`
	Resource  string            `json:"Resource,omitempty"`
	Principal map[string]string `json:"Principal,omitempty"`
	Sid       string            `json:"Sid,omitempty"`
}

func (r *AWSIAMReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	sdkConfig, err := awsConfig.LoadDefaultConfig(context.Background())

	if err != nil {
		panic(err)
	}

	iamClient := iam.NewFromConfig(sdkConfig)

	logger := logrus.WithField("namespacedName", req.String())

	intents := &otterizev1alpha2.ClientIntents{}
	err = r.client.Get(ctx, req.NamespacedName, intents)

	if err != nil && k8serrors.IsNotFound(err) {
		logger.WithError(err).Info("No intents found")
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	if intents.Spec == nil {
		logger.Info("No specs found")
		return ctrl.Result{}, nil
	}

	awsCalls := lo.Filter(intents.Spec.Calls, func(item otterizev1alpha2.Intent, index int) bool {
		return item.Type == otterizev1alpha2.IntentTypeAWS
	})

	if len(awsCalls) == 0 {
		return ctrl.Result{}, nil
	}

	roleName := fmt.Sprintf("otterize-%s-%s", req.Namespace, req.Name)

	trust := policyDocument{
		Version: "2012-10-17",
		Statement: []statementEntry{
			{
				Effect:    "Allow",
				Principal: map[string]string{"AWS": "353146681200"},
				Action:    []string{"sts:AssumeRole"},
			},
		},
	}

	policyBytes, err := json.Marshal(trust)

	if err != nil {
		panic(err)
	}

	_, err = iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	})

	if err != nil {
		var e *types.NoSuchEntityException
		if errors.As(err, &e) {
			createRoleOutput, err := iamClient.CreateRole(
				ctx, &iam.CreateRoleInput{
					RoleName:                 &roleName,
					AssumeRolePolicyDocument: aws.String(string(policyBytes)),
					Tags: []types.Tag{
						{
							Key:   aws.String("otterize/service"),
							Value: aws.String(intents.Spec.Service.Name),
						},
						{
							Key:   aws.String("otterize/namespace"),
							Value: aws.String(intents.Namespace),
						},
					},
				},
			)

			if err != nil {
				panic(err)
			}

			logger.Infof("Created role %s", *createRoleOutput.Role.RoleName)
		}
	}

	policy := policyDocument{
		Version: "2012-10-17",
	}

	for _, intent := range awsCalls {
		for _, call := range intent.AWSResources {
			policy.Statement = append(policy.Statement,
				statementEntry{
					Effect:   "Allow",
					Resource: call.Resource,
					Action:   call.Actions,
				},
			)
		}
	}

	policyBytes, err = json.Marshal(policy)

	if err != nil {
		panic(err)
	}

	createPolicyOutput, err := iamClient.CreatePolicy(ctx, &iam.CreatePolicyInput{
		PolicyDocument: aws.String(string(policyBytes)),
		PolicyName:     aws.String(fmt.Sprintf("otterize-policy-%s-%s", intents.Namespace, intents.Name)),
	})

	if err != nil {
		panic(err)
	}

	_, err = iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
		PolicyArn: createPolicyOutput.Policy.Arn,
		RoleName:  aws.String(roleName),
	})

	if err != nil {
		panic(err)
	}

	return ctrl.Result{}, nil
}
