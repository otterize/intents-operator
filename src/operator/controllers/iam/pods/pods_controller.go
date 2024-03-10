package pods

import (
	"context"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/otterize/credentials-operator/src/shared/apiutils"
	"github.com/otterize/intents-operator/src/shared/errors"
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

type PodReconciler struct {
	client.Client
}

func NewPodReconciler(client client.Client) *PodReconciler {
	return &PodReconciler{
		Client: client,
	}
}

func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := apiutils.InitPodServiceAccountIndexField(mgr)
	if err != nil {
		return errors.Wrap(err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		For(&corev1.Pod{}).
		Complete(r)
}

func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logrus.WithField("name", req.Name).WithField("namespace", req.Namespace)
	pod := corev1.Pod{}

	err := r.Get(ctx, req.NamespacedName, &pod)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrap(err)
	}

	if pod.DeletionTimestamp == nil {
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(&pod, metadata.IAMRoleFinalizer) && !controllerutil.ContainsFinalizer(&pod, metadata.AWSRoleFinalizer) {
		logger.Debug("pod does not have the Otterize finalizer, skipping")
		return ctrl.Result{}, nil
	}

	// Find all pods that have the same service account
	saConsumers, err := apiutils.GetPodServiceAccountConsumers(ctx, r, pod)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	// Get only the pods that are IAM consumers - also handles case where label was removed from the pod.
	iamSAConsumers := lo.Filter(saConsumers, func(filteredPod corev1.Pod, _ int) bool {
		return controllerutil.ContainsFinalizer(&pod, metadata.IAMRoleFinalizer) || controllerutil.ContainsFinalizer(&pod, metadata.AWSRoleFinalizer) || pod.UID == filteredPod.UID
	})

	// check if this is the last pod linked to this SA.
	if len(iamSAConsumers) == 1 && iamSAConsumers[0].UID == pod.UID {
		var serviceAccount corev1.ServiceAccount
		err := r.Get(ctx, types.NamespacedName{Name: pod.Spec.ServiceAccountName, Namespace: pod.Namespace}, &serviceAccount)
		if err != nil {
			// service account can be deleted before the pods go down, in which case cleanup has already occurred, so just let the pod terminate.
			if apierrors.IsNotFound(err) {
				return apiutils.RemoveFinalizerFromPod(ctx, r, pod, metadata.IAMRoleFinalizer, metadata.AWSRoleFinalizer)
			}
			return ctrl.Result{}, errors.Wrap(err)
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
				return apiutils.RemoveFinalizerFromPod(ctx, r, pod, metadata.IAMRoleFinalizer, metadata.AWSRoleFinalizer)
			}
			return ctrl.Result{}, errors.Wrap(err)
		}
	}

	// in case there's more than 1 pod, this is not the last pod, so we can just let the pod terminate.
	return apiutils.RemoveFinalizerFromPod(ctx, r, pod, metadata.IAMRoleFinalizer, metadata.AWSRoleFinalizer)
}
