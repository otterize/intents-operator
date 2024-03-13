package clusterid

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/shared/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const OtterizeClusterUIDResourceName = "otterize-cluster-uid"

// GetClusterUID Fetches cluster ID from the config map created in the Otterize namespace
func GetClusterUID(ctx context.Context, client client.Client) (string, error) {
	podNamespace := os.Getenv("POD_NAMESPACE")
	cm := v1.ConfigMap{}
	err := client.Get(ctx, types.NamespacedName{Namespace: podNamespace, Name: OtterizeClusterUIDResourceName}, &cm)
	if err != nil {
		return "", errors.Wrap(err)
	}

	clusterUID, ok := cm.Data["clusteruid"]
	if !ok || clusterUID == "" {
		return "", errors.Wrap(fmt.Errorf("invalid cluster UID found in %s config map", OtterizeClusterUIDResourceName))
	}

	return clusterUID, nil
}
