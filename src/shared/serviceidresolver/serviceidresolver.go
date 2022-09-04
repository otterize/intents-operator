package serviceidresolver

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ServiceNameAnnotation = "otterize/service-name"
)

type Resolver struct {
	client.Client
}

func NewResolver(c client.Client) *Resolver {
	return &Resolver{c}
}

// ResolvePodToServiceIdentity resolves a pod object to its otterize service ID, referenced in intents objects.
// This is done by recursion over the pod's owner reference hierarchy until reaching a root owner reference.
// In case the pod is annotated with an "otterize/service-name" annotation, that annotation's value will override
// any owner reference name as the service name.
func (r *Resolver) ResolvePodToServiceIdentity(ctx context.Context, pod *corev1.Pod) (string, error) {
	log := logrus.WithFields(logrus.Fields{"pod": pod.Name, "namespace": pod.Namespace})

	annotatedServiceName, ok := pod.Annotations[ServiceNameAnnotation]
	if ok {
		return annotatedServiceName, nil
	}

	var obj client.Object
	obj = pod
	for len(obj.GetOwnerReferences()) > 0 {
		owner := obj.GetOwnerReferences()[0]
		ownerObj := &unstructured.Unstructured{}
		ownerObj.SetAPIVersion(owner.APIVersion)
		ownerObj.SetKind(owner.Kind)
		err := r.Get(ctx, types.NamespacedName{Name: owner.Name, Namespace: obj.GetNamespace()}, ownerObj)
		if err != nil && errors.IsForbidden(err) {
			// We don't have permissions for further resolving of the owner object,
			// and so we treat it as the identity.
			log.WithFields(logrus.Fields{"owner": owner.Name, "ownerKind": obj.GetObjectKind().GroupVersionKind()}).Warning(
				"permission error resolving owner, will use owner object as service identifier",
			)
			return owner.Name, nil
		} else if err != nil {
			return "", fmt.Errorf("error querying owner reference: %w", err)
		}

		// recurse parent owner reference
		obj = ownerObj
	}

	log.WithFields(logrus.Fields{"owner": obj.GetName(), "ownerKind": obj.GetObjectKind().GroupVersionKind()}).Info("pod resolved to owner name")
	return obj.GetName(), nil
}
