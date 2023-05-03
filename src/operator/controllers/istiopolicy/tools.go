package istiopolicy

import (
	"context"
	"fmt"
	"istio.io/client-go/pkg/apis/security/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	IstioCRDName            = "authorizationpolicies.security.istio.io"
	IstioProxyContainerName = "istio-proxy"
)

func IsPodPartOfIstioMesh(pod corev1.Pod) bool {
	for _, container := range pod.Spec.Containers {
		if container.Name == IstioProxyContainerName {
			return true
		}
	}
	return false
}

func IsIstioAuthorizationPoliciesInstalled(ctx context.Context, client client.Client) (bool, error) {
	groupVersionKinds, _, err := client.Scheme().ObjectKinds(&v1beta1.AuthorizationPolicy{})
	if err != nil {
		return false, err
	}

	istioCRDName := fmt.Sprintf("authorizationpolicies.%s", groupVersionKinds[0].Group)
	crd := apiextensionsv1.CustomResourceDefinition{}
	err = client.Get(ctx, types.NamespacedName{Name: istioCRDName}, &crd)
	if err != nil && !k8serrors.IsNotFound(err) {
		return false, err
	}

	if k8serrors.IsNotFound(err) {
		return false, nil
	}

	return true, nil
}
