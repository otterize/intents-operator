package linkerdmanager

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	LinkerdProxyContainerName = "linkerd-proxy"
)

func IsPodPartOfLinkerdMesh(pod corev1.Pod) bool {
	for _, container := range pod.Spec.Containers {
		if container.Name == LinkerdProxyContainerName {
			return true
		}
	}
	return false
}

func IsLinkerdServerInstalled(ctx context.Context, client client.Client) (bool, error) {
	// turn to check for all necessary crds
	linkerdServerCRDName := "servers.policy.linkerd.io"
	crd := apiextensionsv1.CustomResourceDefinition{}
	err := client.Get(ctx, types.NamespacedName{Name: linkerdServerCRDName}, &crd)
	if err != nil && !k8serrors.IsNotFound(err) {
		return false, err
	}

	if k8serrors.IsNotFound(err) {
		return false, nil
	}

	return true, nil
}
