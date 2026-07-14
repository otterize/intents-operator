package istiopolicy

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/shared/errors"
	"golang.org/x/exp/slices"
	"istio.io/client-go/pkg/apis/security/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	IstioCRDName                 = "authorizationpolicies.security.istio.io"
	IstioProxyContainerName      = "istio-proxy"
	IstioSidecarStatusAnnotation = "sidecar.istio.io/status"
)

func IsPodPartOfIstioMesh(pod corev1.Pod) bool {
	if _, injected := pod.Annotations[IstioSidecarStatusAnnotation]; injected {
		return true
	}

	isProxy := func(c corev1.Container) bool { return c.Name == IstioProxyContainerName }
	return slices.ContainsFunc(pod.Spec.Containers, isProxy) ||
		slices.ContainsFunc(pod.Spec.InitContainers, isProxy)
}

func IsIstioAuthorizationPoliciesInstalled(ctx context.Context, client client.Client) (bool, error) {
	groupVersionKinds, _, err := client.Scheme().ObjectKinds(&v1beta1.AuthorizationPolicy{})
	if err != nil {
		return false, errors.Wrap(err)
	}

	istioCRDName := fmt.Sprintf("authorizationpolicies.%s", groupVersionKinds[0].Group)
	crd := apiextensionsv1.CustomResourceDefinition{}
	err = client.Get(ctx, types.NamespacedName{Name: istioCRDName}, &crd)
	if err != nil && !k8serrors.IsNotFound(err) {
		return false, errors.Wrap(err)
	}

	if k8serrors.IsNotFound(err) {
		return false, nil
	}

	return true, nil
}
