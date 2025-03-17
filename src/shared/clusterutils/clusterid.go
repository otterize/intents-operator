package clusterutils

import (
	"context"
	"github.com/google/uuid"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/k8sconf"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"os"
)

const OtterizeClusterUIDResourceName = "otterize-cluster-uid"
const OtterizeClusterUIDKeyName = "clusteruid"

// GetClusterUID Fetches cluster ID from the config map created in the Otterize namespace
func GetClusterUID(ctx context.Context) (string, error) {
	k8sclient, err := kubernetes.NewForConfig(k8sconf.KubernetesConfigOrDie())
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
		// Should never reach this part since we set config map as immutable when we create it
		return "", errors.Errorf("invalid cluster UID found in %s config map", OtterizeClusterUIDResourceName)
	}

	return clusterUID, nil
}

func SetClusterUID(ctx context.Context) (string, error) {
	k8sclient, err := kubernetes.NewForConfig(k8sconf.KubernetesConfigOrDie())
	if err != nil {
		return "", errors.Wrap(err)
	}
	podNamespace := os.Getenv("POD_NAMESPACE")
	clusterUID := ""
	kubeSystemNs, err := k8sclient.CoreV1().Namespaces().Get(ctx, "kube-system", metav1.GetOptions{})
	if err != nil || kubeSystemNs == nil {
		logrus.Warningf("failed getting kubesystem UID: %s", err)
		clusterUID = uuid.New().String()
	} else {
		clusterUID = string(kubeSystemNs.UID)
	}

	_, err = k8sclient.CoreV1().ConfigMaps(podNamespace).Create(ctx, &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OtterizeClusterUIDResourceName,
			Namespace: podNamespace,
		},
		Immutable: lo.ToPtr(true),
		Data:      map[string]string{OtterizeClusterUIDKeyName: clusterUID},
	}, metav1.CreateOptions{})

	if err != nil {
		if k8serrors.IsAlreadyExists(err) || k8serrors.IsConflict(err) {
			// Already created by a different component, try to run GetClusterUID again
			return GetClusterUID(ctx)
		}
		return "", errors.Wrap(err)
	}

	return clusterUID, nil
}

func getOrCreateClusterUID(ctx context.Context) (string, error) {
	clusterUID, err := GetClusterUID(ctx)
	if err != nil {
		if k8sErr := &(k8serrors.StatusError{}); errors.As(err, &k8sErr) {
			if k8serrors.IsNotFound(k8sErr) {
				clusterUID, err = SetClusterUID(ctx)
				if err != nil {
					return "", errors.Wrap(err)
				}
				return clusterUID, nil
			}
		}
		return "", errors.Wrap(err)
	}
	return clusterUID, nil
}

func GetOrCreateClusterUID(ctx context.Context) string {
	clusterUID, err := getOrCreateClusterUID(ctx)
	if err != nil {
		logrus.WithError(err).Error("failed to create cluster UID, falling back to randomized UID")
		return uuid.NewString()
	}
	return clusterUID
}
