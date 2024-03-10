package serviceaccounts

import (
	"context"
	"fmt"
	"github.com/otterize/credentials-operator/src/controllers/iam"
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
	agents []iam.IAMCredentialsAgent
}

func NewServiceAccountReconciler(client client.Client, agents []iam.IAMCredentialsAgent) *ServiceAccountReconciler {
	return &ServiceAccountReconciler{
		Client: client,
		agents: agents,
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

	value, ok := getLabelValue(&serviceAccount, metadata.OtterizeServiceAccountLabel)
	if !ok {
		logger.Debugf("serviceAccount not labeled with %s, skipping", metadata.OtterizeServiceAccountLabel)
		return ctrl.Result{}, nil
	}

	isReferencedByPods := value == metadata.OtterizeServiceAccountHasPodsValue

	// Perform cleanup if the service account is being deleted or no longer referenced by pods
	if serviceAccount.DeletionTimestamp != nil || !isReferencedByPods {
		return r.HandleServiceCleanup(ctx, serviceAccount)
	}

	return r.HandleServiceUpdate(ctx, serviceAccount)
}

func getLabelValue(serviceAccount *corev1.ServiceAccount, label string) (string, bool) {
	if serviceAccount.Labels == nil {
		return "", false
	}
	value, ok := serviceAccount.Labels[label]
	return value, ok
}

func (r *ServiceAccountReconciler) HandleServiceCleanup(ctx context.Context, serviceAccount corev1.ServiceAccount) (ctrl.Result, error) {
	for _, agent := range r.agents {
		if err := agent.OnServiceAccountTermination(ctx, &serviceAccount); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove service account: %w", err)
		}
	}

	if serviceAccount.DeletionTimestamp != nil {
		updatedServiceAccount := serviceAccount.DeepCopy()
		if controllerutil.RemoveFinalizer(updatedServiceAccount, metadata.IAMRoleFinalizer) {
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

func (r *ServiceAccountReconciler) HandleServiceUpdate(ctx context.Context, serviceAccount corev1.ServiceAccount) (ctrl.Result, error) {
	// Add a finalizer label to the service account to block deletion until cleanup is complete
	updatedServiceAccount := serviceAccount.DeepCopy()
	if controllerutil.AddFinalizer(updatedServiceAccount, metadata.IAMRoleFinalizer) {
		err := r.Client.Patch(ctx, updatedServiceAccount, client.MergeFrom(&serviceAccount))
		if err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, errors.Wrap(err)
		}
	}

	hasUpdates := false
	for _, agent := range r.agents {
		updated, requeue, err := agent.OnServiceAccountUpdate(ctx, updatedServiceAccount)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to reconcile service account: %w", err)
		}
		if requeue {
			return ctrl.Result{Requeue: true}, nil
		}
		hasUpdates = hasUpdates || updated
	}

	if !hasUpdates {
		return ctrl.Result{}, nil
	}

	if err := r.Client.Patch(ctx, updatedServiceAccount, client.MergeFrom(&serviceAccount)); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, errors.Wrap(err)
	}

	return ctrl.Result{}, nil
}
