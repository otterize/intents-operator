package gcpagent

import (
	"cloud.google.com/go/compute/metadata"
	"context"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	EnvGcpProjectId = "gcp-project-id"
	EnvGcpGkeName   = "gcp-gke-name"

	// GCPApplyOnPodLabel is used to mark pods that should be processed by the GCP agent to create an associated GCP service account
	GCPApplyOnPodLabel = "credentials-operator.otterize.com/create-gcp-sa"
)

type Agent struct {
	projectID   string
	clusterName string
	client      client.Client
}

func NewGCPAgent(ctx context.Context, c client.Client) (*Agent, error) {
	logrus.Info("Initializing GCP Intents agent")

	// Get the current GCP project using the metadata server or local env
	projectID, err := getGCPAttribute(EnvGcpProjectId)
	if err != nil {
		return nil, errors.Errorf("failed to get current GCP project: %w", err)
	}

	// Retrieve the cluster name using the metadata server or local env
	clusterName, err := getGCPAttribute(EnvGcpGkeName)
	if err != nil {
		return nil, errors.Errorf("failed to get current GKE cluster: %w", err)
	}

	return &Agent{
		client:      c,
		projectID:   projectID,
		clusterName: clusterName,
	}, nil
}

func getGCPAttribute(attribute string) (res string, err error) {
	switch attribute {
	case EnvGcpProjectId:
		res, err = metadata.ProjectID()
		if err == nil {
			return res, nil
		}
	case EnvGcpGkeName:
		res, err = metadata.InstanceAttributeValue("cluster-name")
		if err == nil {
			return res, nil
		}
	}

	res = viper.GetString(attribute)
	if res == "" {
		return "", errors.Errorf("%s environment variable is required", attribute)
	}
	return res, nil
}

func (a *Agent) AppliesOnPod(pod *corev1.Pod) bool {
	return pod.Labels != nil && pod.Labels[GCPApplyOnPodLabel] == "true"
}
