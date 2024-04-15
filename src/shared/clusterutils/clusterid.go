package clusterutils

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/shared/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
)

const OtterizeClusterUIDResourceName = "otterize-cluster-uid"
const OtterizeClusterUIDKeyName = "clusteruid"

// GetClusterUID Fetches cluster ID from the config map created in the Otterize namespace
func GetClusterUID(ctx context.Context) (string, error) {
	k8sclient, err := kubernetes.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		return "", errors.Wrap(err)
	}
	podNamespace := os.Getenv("POD_NAMESPACE")
	configMap, err := k8sclient.CoreV1().ConfigMaps(podNamespace).Get(ctx, OtterizeClusterUIDResourceName, metav1.GetOptions{})
	if err != nil {
		return "", errors.Wrap(err)
	}

	clusterUID, ok := configMap.Data[OtterizeClusterUIDKeyName]
	if !ok || clusterUID == "" {
		return "", errors.Wrap(fmt.Errorf("invalid cluster UID found in %s config map", OtterizeClusterUIDResourceName))
	}

	return clusterUID, nil
}
