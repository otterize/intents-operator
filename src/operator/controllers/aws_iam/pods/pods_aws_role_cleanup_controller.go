package pods

import (
	"context"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type PodAWSRoleCleanupReconciler struct {
	client.Client
}

func NewPodAWSRoleCleanupReconciler(client client.Client) *PodAWSRoleCleanupReconciler {
	return &PodAWSRoleCleanupReconciler{
		Client: client,
	}
}

const podServiceAccountIndexField = "spec.serviceAccountName"

func initPodServiceAccountIndexField(mgr ctrl.Manager) error {
	err := mgr.GetCache().IndexField(
		context.Background(),
		&corev1.Pod{},
		podServiceAccountIndexField,
		func(object client.Object) []string {
			pod := object.(*corev1.Pod)
			return []string{pod.Spec.ServiceAccountName}
		})
	if err != nil {
		return err
	}

	return nil
}

func (r *PodAWSRoleCleanupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := initPodServiceAccountIndexField(mgr)
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		For(&corev1.Pod{}).
		Complete(r)
}

func (r *PodAWSRoleCleanupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pod := corev1.Pod{}

	err := r.Get(ctx, req.NamespacedName, &pod)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if pod.DeletionTimestamp == nil {
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(&pod, metadata.AWSRoleFinalizer) {
		logrus.Debugf("pod %v does not have the Otterize finalizer, skipping", pod.Name)
		return ctrl.Result{}, nil
	}

	var pods corev1.PodList
	err = r.List(ctx, &pods,
		client.MatchingFields{podServiceAccountIndexField: pod.Spec.ServiceAccountName},
		&client.ListOptions{Namespace: pod.Namespace})
	if err != nil {
		return ctrl.Result{}, err
	}
	// check if this is the last pod linked to this SA.
	if len(pods.Items) == 1 && pods.Items[0].UID == pod.UID {
		var serviceAccount corev1.ServiceAccount
		err := r.Get(ctx, types.NamespacedName{Name: pod.Spec.ServiceAccountName, Namespace: pod.Namespace}, &serviceAccount)
		if err != nil {
			// service account can be deleted before the pods go down, in which case cleanup has already occurred, so just let the pod terminate.
			if apierrors.IsNotFound(err) {
				return r.removeFinalizerFromPod(ctx, pod)
			}
			return ctrl.Result{}, err
		}

		updatedServiceAccount := serviceAccount.DeepCopy()
		if updatedServiceAccount.Labels == nil {
			updatedServiceAccount.Labels = make(map[string]string)
		}
		// Normally we would call the other reconciler, but because this is blocking the removal of a pod finalizer,
		// we instead update the ServiceAccount and let it do the hard work, so we can remove the pod finalizer ASAP.
		updatedServiceAccount.Labels[metadata.OtterizeServiceAccountLabel] = metadata.OtterizeServiceAccountHasNoPodsValue
		err = r.Client.Patch(ctx, updatedServiceAccount, client.MergeFrom(&serviceAccount))
		if err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			// service account can be deleted before the pods go down, in which case cleanup has already occurred, so just let the pod terminate.
			if apierrors.IsNotFound(err) {
				return r.removeFinalizerFromPod(ctx, pod)
			}
			return ctrl.Result{}, err
		}
	}
	// in case there's more than 1 pod, this is not the last pod so we can just let the pod terminate.
	return r.removeFinalizerFromPod(ctx, pod)
}

func (r *PodAWSRoleCleanupReconciler) removeFinalizerFromPod(ctx context.Context, pod corev1.Pod) (ctrl.Result, error) {
	updatedPod := pod.DeepCopy()
	if controllerutil.RemoveFinalizer(updatedPod, metadata.AWSRoleFinalizer) {
		err := r.Client.Patch(ctx, updatedPod, client.MergeFrom(&pod))
		if err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}
