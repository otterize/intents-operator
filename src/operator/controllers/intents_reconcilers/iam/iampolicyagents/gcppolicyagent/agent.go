package gcppolicyagent

import (
	"context"
	otterizev2 "github.com/otterize/intents-operator/src/operator/api/v2"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/gcpagent"
	corev1 "k8s.io/api/core/v1"
)

type Agent struct {
	*gcpagent.Agent
}

func NewGCPPolicyAgent(gcpAgent *gcpagent.Agent) *Agent {
	return &Agent{gcpAgent}
}

func (a *Agent) IntentType() otterizev2.IntentType {
	return otterizev2.IntentTypeGCP
}

func (a *Agent) AddRolePolicyFromIntents(ctx context.Context, namespace string, accountName string, intentsServiceName string, intents []otterizev2.Target, pod corev1.Pod) error {
	return a.ApplyIAMPartialPolicy(ctx, namespace, accountName, intentsServiceName, intents)
}

func (a *Agent) DeleteRolePolicyFromIntents(ctx context.Context, intents otterizev2.ClientIntents) error {
	err := a.DeleteIAMPartialPolicy(ctx, intents.Namespace, intents.Spec.Workload.Name)
	if err != nil {
		return errors.Wrap(err)
	}
	return nil
}
