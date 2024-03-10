package apiutils

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

func RemoveFinalizerFromPod(ctx context.Context, c client.Client, pod corev1.Pod, finalizer string, legacyFinalizer string) (ctrl.Result, error) {
	updatedPod := pod.DeepCopy()
	if controllerutil.RemoveFinalizer(updatedPod, finalizer) || controllerutil.RemoveFinalizer(updatedPod, legacyFinalizer) {
		err := c.Patch(ctx, updatedPod, client.MergeFrom(&pod))
		if err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, errors.Wrap(err)
		}
	}

	return ctrl.Result{}, nil
}
