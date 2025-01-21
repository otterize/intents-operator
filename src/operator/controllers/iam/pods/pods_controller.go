package pods

import (
	"context"
	"github.com/otterize/credentials-operator/src/controllers/iam/iamcredentialsagents"
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
	agent iamcredentialsagents.IAMCredentialsAgent
}

func NewPodReconciler(client client.Client, agent iamcredentialsagents.IAMCredentialsAgent) *PodReconciler {
	return &PodReconciler{
		Client: client,
		agent:  agent,
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

	pod := corev1.Pod{}

	err := r.Get(ctx, req.NamespacedName, &pod)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrap(err)
	}

	if pod.DeletionTimestamp == nil {
		return r.handlePodUpdate(ctx, pod)
	}

	return r.handlePodCleanup(ctx, pod)
}

func (r *PodReconciler) handlePodUpdate(ctx context.Context, pod corev1.Pod) (ctrl.Result, error) {
	logger := logrus.WithField("name", pod.Name).WithField("namespace", pod.Namespace)

	if !r.agent.AppliesOnPod(&pod) {
		logger.Debug("pod does not have the Otterize IAM label, skipping")
		return ctrl.Result{}, nil
	}

	logger.Debug("Otterize IAM label found on pod")

	var serviceAccount corev1.ServiceAccount
	err := r.Get(ctx, types.NamespacedName{Name: pod.Spec.ServiceAccountName, Namespace: pod.Namespace}, &serviceAccount)
	if err != nil {
		return ctrl.Result{}, errors.Errorf("failed to get service account: %w", err)
	}

	logger = logger.WithField("serviceAccount", serviceAccount.Name)

	updatedServiceAccount := serviceAccount.DeepCopy()
	updatedPod := pod.DeepCopy()

	updated, requeue, err := r.agent.OnPodUpdate(ctx, updatedPod, updatedServiceAccount)
	if err != nil {
		return ctrl.Result{}, errors.Errorf("failed to reconcile pod: %w", err)
	}
	if requeue {
		return ctrl.Result{Requeue: true}, nil
	}
	if updated {
		logger.Info("updating Otterize IAM managed pod")
		controllerutil.AddFinalizer(updatedPod, r.agent.FinalizerName())
		err := r.Patch(ctx, updatedPod, client.MergeFrom(&pod))
		if err != nil {
			if apierrors.IsConflict(err) || apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, errors.Wrap(err)
		}

		logger.Info("updating Otterize IAM managed serviceAccount")
		apiutils.AddLabel(updatedServiceAccount, r.agent.ServiceAccountLabel(), metadata.OtterizeServiceAccountHasPodsValue)
		err = r.Patch(ctx, updatedServiceAccount, client.MergeFrom(&serviceAccount))
		if err != nil {
			if apierrors.IsConflict(err) || apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, errors.Wrap(err)
		}
	}

	return ctrl.Result{}, nil
}

func (r *PodReconciler) handlePodCleanup(ctx context.Context, pod corev1.Pod) (ctrl.Result, error) {
	logger := logrus.WithField("name", pod.Name).WithField("namespace", pod.Namespace)

	if !controllerutil.ContainsFinalizer(&pod, r.agent.FinalizerName()) && !controllerutil.ContainsFinalizer(&pod, metadata.DeprecatedIAMRoleFinalizer) {
		logger.Debug("pod does not have the Otterize finalizer, skipping")
		return ctrl.Result{}, nil
	}

	updatedPod := pod.DeepCopy()
	if controllerutil.RemoveFinalizer(updatedPod, r.agent.FinalizerName()) || controllerutil.RemoveFinalizer(updatedPod, metadata.DeprecatedIAMRoleFinalizer) {
		err := r.Patch(ctx, updatedPod, client.StrategicMergeFrom(&pod))
		if err != nil {
			if apierrors.IsConflict(err) || apierrors.IsNotFound(err) || apierrors.IsForbidden(err) || apierrors.IsInvalid(err) {
				// These are all errors that can happen because the pod is already being deleted, requeuing
				// should solve them all in a classy way
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, errors.Wrap(err)
		}
	}

	return ctrl.Result{}, nil
}
