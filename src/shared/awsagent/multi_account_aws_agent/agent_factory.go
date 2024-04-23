package multi_account_aws_agent

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/awsagent"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
)

func MakeAgentsFromAccountList[T any](ctx context.Context, accounts []operatorconfig.AWSAccount, agentFactory func(awsAgent *awsagent.Agent) T, awsOptionsInput []awsagent.Option, clusterName string, keyPath string, certPath string) (map[string]T, error) {
	agents := make(map[string]T)

	for _, account := range accounts {
		awsOptions := make([]awsagent.Option, len(awsOptionsInput))
		copy(awsOptions, awsOptionsInput)

		awsOptions = append(awsOptions, awsagent.WithRolesAnywhere(
			account,
			clusterName,
			keyPath,
			certPath,
		))
		awsAgent, err := awsagent.NewAWSAgent(ctx, awsOptions...)
		if err != nil {
			return nil, errors.Errorf("failed to initialize AWS agent for account with role '%s': %w", account.RoleARN, err)
		}
		specializedAgent := agentFactory(awsAgent)
		agents[awsAgent.AccountID] = specializedAgent
	}
	return agents, nil
}
