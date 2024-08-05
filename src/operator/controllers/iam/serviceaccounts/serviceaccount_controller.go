package serviceaccounts

import (
	"context"
	"github.com/otterize/credentials-operator/src/controllers/iam/iamcredentialsagents"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type ServiceAccountReconciler struct {
	client.Client
	agent iamcredentialsagents.IAMCredentialsAgent
}

func NewServiceAccountReconciler(client client.Client, agent iamcredentialsagents.IAMCredentialsAgent) *ServiceAccountReconciler {
	return &ServiceAccountReconciler{
		Client: client,
		agent:  agent,
	}
}

func (r *ServiceAccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		For(&corev1.ServiceAccount{}).
		Complete(r)
}

func (r *ServiceAccountReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logrus.WithFields(logrus.Fields{"serviceAccount": req.Name, "namespace": req.Namespace})

	serviceAccount := corev1.ServiceAccount{}

	err := r.Get(ctx, req.NamespacedName, &serviceAccount)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrap(err)
	}

	value, ok := serviceAccount.Labels[r.agent.ServiceAccountLabel()]
	if !ok {
		logger.Debugf("serviceAccount not labeled with %s, skipping", r.agent.ServiceAccountLabel())
		return ctrl.Result{}, nil
	}

	isReferencedByPods := value == metadata.OtterizeServiceAccountHasPodsValue

	// Perform cleanup if the service account is being deleted or no longer referenced by pods
	if serviceAccount.DeletionTimestamp == nil && isReferencedByPods {
		return r.handleServiceAccountUpdate(ctx, serviceAccount)
	}

	return r.handleServiceAccountCleanup(ctx, serviceAccount)
}

func (r *ServiceAccountReconciler) handleServiceAccountUpdate(ctx context.Context, serviceAccount corev1.ServiceAccount) (ctrl.Result, error) {
	updatedServiceAccount := serviceAccount.DeepCopy()
	updated, requeue, err := r.agent.OnServiceAccountUpdate(ctx, updatedServiceAccount)
	if err != nil {
		return ctrl.Result{}, errors.Errorf("failed to reconcile service account: %w", err)
	}
	if requeue {
		return ctrl.Result{Requeue: true}, nil
	}
	if updated {
		controllerutil.AddFinalizer(updatedServiceAccount, r.agent.FinalizerName())
		err := r.Client.Patch(ctx, updatedServiceAccount, client.MergeFrom(&serviceAccount))
		if err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, errors.Wrap(err)
		}
	}

	return ctrl.Result{}, nil
}

func (r *ServiceAccountReconciler) handleServiceAccountCleanup(ctx context.Context, serviceAccount corev1.ServiceAccount) (ctrl.Result, error) {
	logger := logrus.WithField("name", serviceAccount.Name).WithField("namespace", serviceAccount.Namespace)
	if !controllerutil.ContainsFinalizer(&serviceAccount, r.agent.FinalizerName()) && !controllerutil.ContainsFinalizer(&serviceAccount, metadata.DeprecatedIAMRoleFinalizer) {
		logger.Debug("service account does not have the Otterize finalizer, skipping")
		return ctrl.Result{}, nil
	}

	if err := r.agent.OnServiceAccountTermination(ctx, &serviceAccount); err != nil {
		return ctrl.Result{}, errors.Errorf("failed to remove service account: %w", err)
	}

	// remove finalizer to unblock deletion
	if serviceAccount.DeletionTimestamp != nil {
		updatedServiceAccount := serviceAccount.DeepCopy()
		if controllerutil.RemoveFinalizer(updatedServiceAccount, r.agent.FinalizerName()) || controllerutil.RemoveFinalizer(updatedServiceAccount, metadata.DeprecatedIAMRoleFinalizer) {
			err := r.Client.Patch(ctx, updatedServiceAccount, client.MergeFrom(&serviceAccount))
			if err != nil {
				if apierrors.IsConflict(err) {
					return ctrl.Result{Requeue: true}, nil
				}
				return ctrl.Result{}, errors.Wrap(err)
			}
		}
	}

	return ctrl.Result{}, nil
}
