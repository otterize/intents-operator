package gcpagent

import (
	"cloud.google.com/go/compute/metadata"
	"context"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Agent struct {
	projectID   string
	clusterName string
	client      client.Client
}

func NewGCPAgent(ctx context.Context, c client.Client) (*Agent, error) {
	logrus.Info("Initializing GCP Intents agent")

	// Get the current GCP project using the metadata server
	//projectID := "otterize-gcp-integration" // TODO: cleanup
	projectID, err := metadata.ProjectID()
	if err != nil {
		return nil, errors.Errorf("failed to get current GCP project: %w", err)
	}

	// Retrieve the cluster name using the metadata server
	//clusterName := "otterize-iam-gke-tutorial" // TODO: cleanup
	clusterName, err := metadata.InstanceAttributeValue("cluster-name")
	if err != nil {
		return nil, errors.Errorf("failed to get current GKE cluster: %w", err)
	}

	return &Agent{
		client:      c,
		projectID:   projectID,
		clusterName: clusterName,
	}, nil
}
