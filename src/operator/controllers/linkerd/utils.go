package linkerdmanager

import (
	"context"
	"math/rand"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	LinkerdProxyKey = "linkerd.io/inject"
)

func IsPodPartOfLinkerdMesh(pod corev1.Pod) bool {
	linkerdEnabled, ok := pod.Annotations[LinkerdProxyKey]
	if ok && linkerdEnabled == "enabled" {
		return true
	}
	return false
}

func IsLinkerdInstalled(ctx context.Context, client client.Client) (bool, error) {
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

func generateRandomString(length int, seed ...int64) string {
	charset := "abcdefghijklmnopqrstuvwxyz0123456789"
	if len(seed) > 0 {
		rand.Seed(seed[0])
	} else {
		rand.Seed(time.Now().UnixNano())
	}

	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}

	return string(b)
}
