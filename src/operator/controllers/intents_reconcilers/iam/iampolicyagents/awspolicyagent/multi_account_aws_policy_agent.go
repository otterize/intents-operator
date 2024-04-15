package awspolicyagent

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/awsagent"
	"github.com/otterize/intents-operator/src/shared/awsagent/multi_account_aws_agent"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	corev1 "k8s.io/api/core/v1"
)

type MultiaccountAWSPolicyAgent struct {
	agents map[string]*Agent
}

func NewMultiaccountAWSPolicyAgent(ctx context.Context, accounts []operatorconfig.AWSAccount) (*MultiaccountAWSPolicyAgent, error) {
	agents, err := multi_account_aws_agent.MakeAgentsFromAccountList(ctx, accounts, NewAWSPolicyAgent, nil)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	return &MultiaccountAWSPolicyAgent{agents: agents}, nil
}

func (m *MultiaccountAWSPolicyAgent) IntentType() otterizev1alpha3.IntentType {
	return otterizev1alpha3.IntentTypeAWS
}

func (m *MultiaccountAWSPolicyAgent) AppliesOnPod(pod *corev1.Pod) bool {
	return awsagent.AppliesOnPod(pod)
}

func (m *MultiaccountAWSPolicyAgent) AddRolePolicyFromIntents(ctx context.Context, namespace string, accountName string, intentsServiceName string, intents []otterizev1alpha3.Intent, pod corev1.Pod) error {
	accountId, found := multi_account_aws_agent.AccountFromPod(&pod)
	if !found {
		return errors.New("AWS account must be specified on pod")
	}
	return m.agents[accountId].AddRolePolicyFromIntents(ctx, namespace, accountName, intentsServiceName, intents, pod)
}

func (m *MultiaccountAWSPolicyAgent) DeleteRolePolicyFromIntents(ctx context.Context, intents otterizev1alpha3.ClientIntents) error {
	// Go over all accounts and delete policies
	for _, agent := range m.agents {
		err := agent.DeleteRolePolicyFromIntents(ctx, intents)
		if err != nil {
			return errors.Wrap(err)
		}
	}
	return nil
}
