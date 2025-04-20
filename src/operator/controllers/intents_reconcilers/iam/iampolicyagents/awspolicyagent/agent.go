package awspolicyagent

import (
	"context"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/awsagent"
	corev1 "k8s.io/api/core/v1"
	"regexp"
)

var (
	awsRegionRegex    = regexp.MustCompile(`\$\((AWS_REGION)\)`)
	awsAccountIdRegex = regexp.MustCompile(`\$\((AWS_ACCOUNT_ID)\)`)
)

type Agent struct {
	agent *awsagent.Agent
}

func NewAWSPolicyAgent(awsAgent *awsagent.Agent) *Agent {
	return &Agent{awsAgent}
}

func (a *Agent) AppliesOnPod(pod *corev1.Pod) bool {
	return a.agent.AppliesOnPod(pod)
}

func (a *Agent) IntentType() otterizev2alpha1.IntentType {
	return otterizev2alpha1.IntentTypeAWS
}

func (a *Agent) templateResourceName(resource string) string {
	// replace template variables $(AWS_REGION) and $(AWS_ACCOUNT_ID) with a.region and a.accountID
	resource = awsRegionRegex.ReplaceAllString(resource, a.agent.Region)
	resource = awsAccountIdRegex.ReplaceAllString(resource, a.agent.AccountID)
	return resource
}

func (a *Agent) createPolicyFromIntents(intents []otterizev2alpha1.Target) awsagent.PolicyDocument {
	policy := awsagent.PolicyDocument{
		Version: "2012-10-17",
	}

	if len(intents) == 0 {
		// This is the equivalent of a "null" policy (allowing nothing) for AWS IAM
		policy.Statement = append(policy.Statement, awsagent.StatementEntry{
			Effect:   "Allow",
			Resource: "*",
			Action:   []string{"none:null"},
		})
	}

	for _, intent := range intents {
		awsResource := a.templateResourceName(intent.AWS.ARN)
		actions := intent.AWS.Actions

		policy.Statement = append(policy.Statement, awsagent.StatementEntry{
			Effect:   "Allow",
			Resource: awsResource,
			Action:   actions,
		})
	}

	return policy
}

func (a *Agent) AddRolePolicyFromIntents(ctx context.Context, namespace string, accountName string, intentsServiceName string, _ otterizev2alpha1.ClientIntents, filteredTargets []otterizev2alpha1.Target, _ corev1.Pod) error {
	policyDoc := a.createPolicyFromIntents(filteredTargets)
	return a.agent.AddRolePolicy(ctx, namespace, accountName, intentsServiceName, policyDoc.Statement)
}

func (a *Agent) DeleteRolePolicyFromIntents(ctx context.Context, intents otterizev2alpha1.ClientIntents) error {
	return a.agent.DeleteRolePolicyByNamespacedName(ctx, intents.Namespace, intents.Spec.Workload.Name)
}
