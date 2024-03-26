package awspolicyagent

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/awsagent"
)

type Agent struct {
	*awsagent.Agent
}

func NewAWSPolicyAgent(awsAgent *awsagent.Agent) *Agent {
	return &Agent{awsAgent}
}

func (a *Agent) IntentType() otterizev1alpha3.IntentType {
	return otterizev1alpha3.IntentTypeAWS
}

func (a *Agent) createPolicyFromIntents(intents []otterizev1alpha3.Intent) awsagent.PolicyDocument {
	policy := awsagent.PolicyDocument{
		Version: "2012-10-17",
	}

	for _, intent := range intents {
		awsResource := intent.Name
		actions := intent.AWSActions

		policy.Statement = append(policy.Statement, awsagent.StatementEntry{
			Effect:   "Allow",
			Resource: awsResource,
			Action:   actions,
		})
	}

	return policy
}

func (a *Agent) AddRolePolicyFromIntents(ctx context.Context, namespace string, accountName string, intentsServiceName string, intents []otterizev1alpha3.Intent) error {
	policyDoc := a.createPolicyFromIntents(intents)
	return a.AddRolePolicy(ctx, namespace, accountName, intentsServiceName, policyDoc.Statement)
}

func (a *Agent) DeleteRolePolicyFromIntents(ctx context.Context, intents otterizev1alpha3.ClientIntents) error {
	return a.DeleteRolePolicyByNamespacedName(ctx, intents.Namespace, intents.Spec.Service.Name)
}
