package serviceidresolver

import (
	"context"
	"errors"
	"fmt"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

var PodNotFound = errors.New("pod not found")

type ServiceResolver interface {
	GetPodAnnotatedName(ctx context.Context, podName string, podNamespace string) (string, bool, error)
	ResolveClientIntentToPod(ctx context.Context, intent v1alpha2.ClientIntents) (corev1.Pod, error)
	ResolveIntentServerToPod(ctx context.Context, intent v1alpha2.Intent, namespace string) (corev1.Pod, error)
}

type Resolver struct {
	client client.Client
}

func NewResolver(c client.Client) *Resolver {
	return &Resolver{client: c}
}

func ResolvePodToServiceIdentityUsingAnnotationOnly(pod *corev1.Pod) (string, bool) {
	annotation, ok := pod.Annotations[viper.GetString(serviceNameOverrideAnnotationKey)]
	return annotation, ok
}

func (r *Resolver) GetPodAnnotatedName(ctx context.Context, podName string, podNamespace string) (string, bool, error) {
	var pod corev1.Pod
	err := r.client.Get(ctx, types.NamespacedName{Name: podName, Namespace: podNamespace}, &pod)
	if err != nil {
		return "", false, err
	}

	annotation, ok := ResolvePodToServiceIdentityUsingAnnotationOnly(&pod)
	return annotation, ok, nil
}

type ServiceIdentity struct {
	Name string
	// OwnerObject used to resolve the service name. May be nil if service name was resolved using annotation.
	OwnerObject client.Object
}

// ResolvePodToServiceIdentity resolves a pod object to its otterize service ID, referenced in intents objects.
// It calls GetOwnerObject to recursively iterates over the pod's owner reference hierarchy until reaching a root owner reference.
// In case the pod is annotated with an "intents.otterize.com/service-name" annotation, that annotation's value will override
// any owner reference name as the service name.
func (r *Resolver) ResolvePodToServiceIdentity(ctx context.Context, pod *corev1.Pod) (ServiceIdentity, error) {
	annotatedServiceName, ok := ResolvePodToServiceIdentityUsingAnnotationOnly(pod)
	if ok {
		return ServiceIdentity{Name: annotatedServiceName}, nil
	}
	ownerObj, err := r.GetOwnerObject(ctx, pod)
	if err != nil {
		return ServiceIdentity{}, err
	}

	resourceName := ownerObj.GetName()
	// Deployments and other resources with pod templates have a dot in their name since they follow RFC 1123 subdomain
	// naming convention. We use the dot as a separator between the service and the namespace. We replace the dot with
	// an underscore, which isn't a valid character in a DNS name.
	// So, for example, a deployment named "my-deployment.5.2.0" will be seen by Otterize as "my-deployment_5_2_0"
	otterizeServiceName := strings.ReplaceAll(resourceName, ".", "_")

	return ServiceIdentity{Name: otterizeServiceName, OwnerObject: ownerObj}, nil
}

// GetOwnerObject recursively iterates over the pod's owner reference hierarchy until reaching a root owner reference
// and returns it.
func (r *Resolver) GetOwnerObject(ctx context.Context, pod *corev1.Pod) (client.Object, error) {
	log := logrus.WithFields(logrus.Fields{"pod": pod.Name, "namespace": pod.Namespace})
	var obj client.Object
	obj = pod
	for len(obj.GetOwnerReferences()) > 0 {
		owner := obj.GetOwnerReferences()[0]
		ownerObj := &unstructured.Unstructured{}
		ownerObj.SetAPIVersion(owner.APIVersion)
		ownerObj.SetKind(owner.Kind)
		err := r.client.Get(ctx, types.NamespacedName{Name: owner.Name, Namespace: obj.GetNamespace()}, ownerObj)
		if err != nil && k8serrors.IsForbidden(err) {
			// We don't have permissions for further resolving of the owner object,
			// and so we treat it as the identity.
			log.WithError(err).WithFields(logrus.Fields{"owner": owner.Name, "ownerKind": obj.GetObjectKind().GroupVersionKind()}).Warning(
				"permission error resolving owner, will use owner object as service identifier",
			)
			ownerObj.SetName(owner.Name)
			return ownerObj, nil
		} else if err != nil {
			return nil, fmt.Errorf("error querying owner reference: %w", err)
		}

		// recurse parent owner reference
		obj = ownerObj
	}

	log.WithFields(logrus.Fields{"owner": obj.GetName(), "ownerKind": obj.GetObjectKind().GroupVersionKind()}).Debug("pod resolved to owner name")
	return obj, nil
}

func (r *Resolver) ResolveClientIntentToPod(ctx context.Context, intent v1alpha2.ClientIntents) (corev1.Pod, error) {
	podsList := &corev1.PodList{}
	labelSelector, err := intent.BuildPodLabelSelector()
	if err != nil {
		return corev1.Pod{}, err
	}
	err = r.client.List(ctx, podsList, client.MatchingLabelsSelector{Selector: labelSelector})
	if err != nil {
		return corev1.Pod{}, err
	}
	if len(podsList.Items) == 0 {
		return corev1.Pod{}, PodNotFound
	}

	for _, pod := range podsList.Items {
		if pod.DeletionTimestamp != nil {
			continue
		}

		return pod, nil
	}

	return corev1.Pod{}, PodNotFound
}

func (r *Resolver) ResolveIntentServerToPod(ctx context.Context, intent v1alpha2.Intent, namespace string) (corev1.Pod, error) {
	podsList := &corev1.PodList{}

	formattedTargetServer := v1alpha2.GetFormattedOtterizeIdentity(intent.GetServerName(), namespace)
	err := r.client.List(
		ctx,
		podsList,
		client.MatchingLabels{v1alpha2.OtterizeServerLabelKey: formattedTargetServer},
		client.InNamespace(namespace),
	)
	if err != nil {
		return corev1.Pod{}, err
	}
	if len(podsList.Items) == 0 {
		return corev1.Pod{}, PodNotFound
	}

	for _, pod := range podsList.Items {
		if pod.DeletionTimestamp != nil {
			continue
		}

		return pod, nil
	}

	return corev1.Pod{}, PodNotFound
}
