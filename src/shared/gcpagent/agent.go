package gcpagent

import (
	"context"
	"github.com/sirupsen/logrus"
)

type Agent struct {
	projectID   string
	clusterName string
}

func NewGCPAgent(ctx context.Context) (*Agent, error) {
	logrus.Info("Initializing GCP Intents agent")

	// Get the current GCP project using the metadata server
	projectID := "otterize-gcp-integration"
	//projectID, err := metadata.ProjectID()
	//if err != nil {
	//	return nil, errors.Errorf("failed to get current GCP project: %w", err)
	//}

	// Retrieve the cluster name using the metadata server
	clusterName := "otterize-iam-gke-tutorial"
	//clusterName, err := metadata.InstanceAttributeValue("cluster-name")
	//if err != nil {
	//	return nil, errors.Errorf("failed to get current GKE cluster: %w", err)
	//}

	return &Agent{
		projectID:   projectID,
		clusterName: clusterName,
	}, nil
}
