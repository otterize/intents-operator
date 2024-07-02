package gcppolicyagent

import (
	"context"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
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

func (a *Agent) IntentType() otterizev2alpha1.IntentType {
	return otterizev2alpha1.IntentTypeGCP
}

func (a *Agent) AddRolePolicyFromIntents(ctx context.Context, namespace string, accountName string, intentsServiceName string, intents []otterizev2alpha1.Target, pod corev1.Pod) error {
	return a.ApplyIAMPartialPolicy(ctx, namespace, accountName, intentsServiceName, intents)
}

func (a *Agent) DeleteRolePolicyFromIntents(ctx context.Context, intents otterizev2alpha1.ClientIntents) error {
	err := a.DeleteIAMPartialPolicy(ctx, intents.Namespace, intents.Spec.Workload.Name)
	if err != nil {
		return errors.Wrap(err)
	}
	return nil
}
