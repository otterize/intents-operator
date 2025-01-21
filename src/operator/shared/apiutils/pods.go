package apiutils

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/errors"
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
