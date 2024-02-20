package azureagent

import (
	"context"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/sirupsen/logrus"
)

type Agent struct {
	credentials *azidentity.DefaultAzureCredential
}

func NewAzureAgent(ctx context.Context) (*Agent, error) {
	logrus.Info("Initializing Azure agent")

	credentials, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}

	return &Agent{
		credentials: credentials,
	}, nil
}
