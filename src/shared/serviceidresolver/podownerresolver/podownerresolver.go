package podownerresolver

import (
	"context"
	"flag"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
)

const (
	cacheSize = 2000
	cacheTTL  = time.Hour
)

var (
	podToServiceIDCache = expirable.NewLRU[types.NamespacedName, serviceidentity.ServiceIdentity](cacheSize, nil, cacheTTL)
)

func ResolvePodToServiceIdentityUsingImageName(pod *corev1.Pod) string {
	// filter out Istio and sidecars
	images := make([]string, 0)
	for _, container := range pod.Spec.Containers {
		if container.Name == "istio-proxy" || strings.Contains(container.Name, "sidecar") {
			continue
		}
		image := container.Image
		_, after, found := strings.Cut(image, "/")
		if !found {
			images = append(images, image)
			continue
		}

		before, _, found := strings.Cut(after, ":")
		if !found {
			images = append(images, after)
			continue
		}
		images = append(images, before)
	}

	return strings.Join(images, "-")
}

func ResolvePodToServiceIdentityUsingAnnotationOnly(pod *corev1.Pod) (string, bool) {
	nameFromAnnotation, ok := pod.Annotations[viper.GetString(WorkloadNameOverrideAnnotationKey)]
	if ok {
		return nameFromAnnotation, ok
	}
	return resolvePodToServiceIdentityUsingDeprecatedAnnotationOnly(pod)
}

func resolvePodToServiceIdentityUsingDeprecatedAnnotationOnly(pod *corev1.Pod) (string, bool) {
	nameFromAnnotation, ok := pod.Annotations[ServiceNameOverrideAnnotationDeprecated]
	return nameFromAnnotation, ok
}

func ResolvePodToServiceIdentity(ctx context.Context, k8sClient client.Client, pod *corev1.Pod) (serviceidentity.ServiceIdentity, error) {
	// Skip cache in test mode
	if flag.Lookup("test.v") != nil {
		return resolvePodToServiceIdentity(ctx, k8sClient, pod)
	}

	key := types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}
	if identity, ok := podToServiceIDCache.Get(key); ok {
		return identity, nil
	}

	identity, err := resolvePodToServiceIdentity(ctx, k8sClient, pod)
	if err != nil {
		return serviceidentity.ServiceIdentity{}, errors.Wrap(err)
	}
	podToServiceIDCache.Add(key, identity)
	return identity, nil
}

// resolvePodToServiceIdentity resolves a pod object to its otterize service ID, referenced in intents objects.
// It calls GetOwnerObject to recursively iterates over the pod's owner reference hierarchy until reaching a root owner reference.
// In case the pod is annotated with an "intents.otterize.com/service-name" annotation, that annotation's value will override
// any owner reference name as the service name.
func resolvePodToServiceIdentity(ctx context.Context, k8sClient client.Client, pod *corev1.Pod) (serviceidentity.ServiceIdentity, error) {
	annotatedServiceName, ok := ResolvePodToServiceIdentityUsingAnnotationOnly(pod)
	if ok {
		return serviceidentity.ServiceIdentity{Name: annotatedServiceName, Namespace: pod.Namespace, ResolvedUsingOverrideAnnotation: lo.ToPtr(true)}, nil
	}
	ownerObj, err := GetOwnerObject(ctx, k8sClient, pod)
	if err != nil {
		return serviceidentity.ServiceIdentity{}, errors.Wrap(err)
	}

	// If the owner is a Job, then the job is often auto generated by some external tool, and has a non-predictable. Use the image name instead.
	if viper.GetBool(UseImageNameForServiceIDForJobs) && ownerObj.GetObjectKind().GroupVersionKind().Kind == "Job" {
		return serviceidentity.ServiceIdentity{Name: ResolvePodToServiceIdentityUsingImageName(pod), Namespace: pod.Namespace, ResolvedUsingOverrideAnnotation: lo.ToPtr(false)}, nil
	}

	resourceName := ownerObj.GetName()
	// Deployments and other resources with pod templates have a dot in their name since they follow RFC 1123 subdomain
	// naming convention. We use the dot as a separator between the service and the namespace. We replace the dot with
	// an underscore, which isn't a valid character in a DNS name.
	// So, for example, a deployment named "my-deployment.5.2.0" will be seen by Otterize as "my-deployment_5_2_0"
	otterizeServiceName := strings.ReplaceAll(resourceName, ".", "_")

	ownerKind := ownerObj.GetObjectKind().GroupVersionKind().Kind
	return serviceidentity.ServiceIdentity{Name: otterizeServiceName, Namespace: pod.Namespace, OwnerObject: ownerObj, Kind: ownerKind, ResolvedUsingOverrideAnnotation: lo.ToPtr(false)}, nil
}

// GetOwnerObject recursively iterates over the pod's owner reference hierarchy until reaching a root owner reference
// and returns it.
func GetOwnerObject(ctx context.Context, k8sClient client.Client, pod *corev1.Pod) (client.Object, error) {
	log := logrus.WithFields(logrus.Fields{"pod": pod.Name, "namespace": pod.Namespace})
	var obj client.Object
	obj = pod
	for len(obj.GetOwnerReferences()) > 0 {
		owner := obj.GetOwnerReferences()[0]
		ownerObj := &unstructured.Unstructured{}
		ownerObj.SetAPIVersion(owner.APIVersion)
		ownerObj.SetKind(owner.Kind)
		// If the kind is not found, we try to resolve it without the version
		// This is a workaround to handle the Kubernetes migration from CronJob at batch/v1beta1 to batch/v1, which resulted in CronJobs with v1beta1
		// version still existing in the cluster despite this kind version not existing anymore. See https://app.bugsnag.com/otterize/intents-operator/errors/660f3f15aec08200089f3c22
		if owner.Kind == "CronJob" {
			mapping, err := k8sClient.RESTMapper().RESTMapping(ownerObj.GroupVersionKind().GroupKind(), ownerObj.GroupVersionKind().Version)
			if errors.Is(err, &meta.NoKindMatchError{}) {
				mapping, err = k8sClient.RESTMapper().RESTMapping(ownerObj.GroupVersionKind().GroupKind(), "")
			}
			if err != nil {
				return nil, errors.Errorf("error getting REST mapping for owner reference: %w", err)
			}
			gvk := ownerObj.GroupVersionKind()
			gvk.Version = mapping.GroupVersionKind.Version
			ownerObj.SetGroupVersionKind(gvk)
		}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: owner.Name, Namespace: obj.GetNamespace()}, ownerObj)
		if err != nil {
			if k8serrors.IsForbidden(err) {
				// We don't have permissions for further resolving of the owner object,
				// and so we treat it as the identity.
				log.WithError(err).WithFields(logrus.Fields{"owner": owner.Name, "ownerKind": obj.GetObjectKind().GroupVersionKind()}).Warning(
					"permission error resolving owner, will use owner object as service identifier",
				)
				ownerObj.SetName(owner.Name)
				return ownerObj, nil
			} else if k8serrors.IsNotFound(err) {
				log.WithError(err).WithFields(logrus.Fields{"owner": owner.Name, "ownerKind": obj.GetObjectKind().GroupVersionKind()}).Warning(
					"resolving owner failed due to owner not found (this is fine if the owner is also being terminated), will use current owner name as service identifier",
				)
				ownerObj.SetName(owner.Name)
				return ownerObj, nil
			} else if errors.Is(err, &meta.NoKindMatchError{}) {
				log.WithError(err).WithFields(logrus.Fields{"owner": owner.Name, "ownerKind": obj.GetObjectKind().GroupVersionKind()}).Warning(
					"resolving owner failed due to owner kind not found, will use current owner name as service identifier",
				)
				ownerObj.SetName(owner.Name)
				return ownerObj, nil
			}
			return nil, errors.Errorf("error querying owner reference: %w", err)
		}

		// recurse parent owner reference
		obj = ownerObj
	}

	log.WithFields(logrus.Fields{"owner": obj.GetName(), "ownerKind": obj.GetObjectKind().GroupVersionKind()}).Debug("pod resolved to owner name")
	return obj, nil
}
