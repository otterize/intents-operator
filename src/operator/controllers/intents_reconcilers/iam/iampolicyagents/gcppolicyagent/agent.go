package gcppolicyagent

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/gcpagent"
)

type Agent struct {
	*gcpagent.Agent
}

func NewGCPPolicyAgent(gcpAgent *gcpagent.Agent) *Agent {
	return &Agent{gcpAgent}
}

func (a *Agent) IntentType() otterizev1alpha3.IntentType {
	return otterizev1alpha3.IntentTypeGCP
}

func (a *Agent) AddRolePolicyFromIntents(ctx context.Context, namespace string, accountName string, intentsServiceName string, intents []otterizev1alpha3.Intent) error {
	return a.ApplyIAMPartialPolicy(ctx, namespace, accountName, intentsServiceName, intents)
}

func (a *Agent) DeleteRolePolicyFromIntents(ctx context.Context, intents otterizev1alpha3.ClientIntents) error {
	err := a.DeleteIAMPartialPolicy(ctx, intents.Namespace, intents.Spec.Service.Name)
	if err != nil {
		return errors.Wrap(err)
	}
	return nil
}
