package gcpagent

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/errors"
)

const GCPPodLabel = "credentials-operator.otterize.com/create-gcp-sa"

func (a *Agent) IntentType() otterizev1alpha3.IntentType {
	return otterizev1alpha3.IntentTypeGCP
}

func (a *Agent) ApplyOnPodLabel() string {
	return GCPPodLabel
}

func (a *Agent) AddRolePolicyFromIntents(ctx context.Context, namespace string, accountName string, intentsServiceName string, intents []otterizev1alpha3.Intent) error {
	err := a.applyIAMPartialPolicy(ctx, namespace, accountName, intents)
	if err != nil {
		return errors.Wrap(err)
	}
	return nil
}

func (a *Agent) DeleteRolePolicyFromIntents(ctx context.Context, intents otterizev1alpha3.ClientIntents) error {
	// TODO: implement

	return nil
}
