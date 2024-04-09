package apiutils

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

const PodServiceAccountIndexField = "spec.serviceAccountName"

func InitPodServiceAccountIndexField(mgr ctrl.Manager) error {
	err := mgr.GetCache().IndexField(
		context.Background(),
		&corev1.Pod{},
		PodServiceAccountIndexField,
		func(object client.Object) []string {
			pod := object.(*corev1.Pod)
			return []string{pod.Spec.ServiceAccountName}
		})
	if err != nil {
		if strings.Contains(err.Error(), "indexer conflict") {
			return nil
		}
		return errors.Wrap(err)
	}

	return nil
}

func GetPodServiceAccountConsumers(ctx context.Context, c client.Client, pod corev1.Pod) ([]corev1.Pod, error) {
	pods := corev1.PodList{}
	err := c.List(ctx, &pods,
		client.MatchingFields{PodServiceAccountIndexField: pod.Spec.ServiceAccountName},
		&client.ListOptions{Namespace: pod.Namespace},
	)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	thisPodAndNonTerminatingPods := lo.Filter(pods.Items, func(filteredPod corev1.Pod, _ int) bool {
		if pod.UID == filteredPod.UID || filteredPod.DeletionTimestamp == nil {
			return true
		}
		return false
	})

	return thisPodAndNonTerminatingPods, nil
}

func AddLabel(o client.Object, key, value string) {
	if o.GetLabels() == nil {
		o.SetLabels(map[string]string{})
	}
	labels := o.GetLabels()
	labels[key] = value
	o.SetLabels(labels)
}

func RemoveLabel(o client.Object, key string) {
	if o.GetLabels() == nil {
		return
	}
	labels := o.GetLabels()
	delete(labels, key)
	o.SetLabels(labels)
}

func AddAnnotation(o client.Object, key, value string) {
	if o.GetAnnotations() == nil {
		o.SetAnnotations(map[string]string{})
	}

	annotations := o.GetAnnotations()
	annotations[key] = value
	o.SetAnnotations(annotations)
}

func RemoveAnnotation(o client.Object, key string) {
	if o.GetAnnotations() == nil {
		return
	}

	annotations := o.GetAnnotations()
	delete(annotations, key)
	o.SetAnnotations(annotations)
}
