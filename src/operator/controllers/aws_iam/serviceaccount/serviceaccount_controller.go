package serviceaccount

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
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

type AWSRolePolicyManager interface {
	DeleteOtterizeIAMRole(ctx context.Context, namespace string, name string) error
	GenerateRoleARN(namespace string, name string) string
	GetOtterizeRole(ctx context.Context, namespaceName, accountName string) (bool, *types.Role, error)
	CreateOtterizeIAMRole(ctx context.Context, namespace string, name string) (*types.Role, error)
}

type ServiceAccountReconciler struct {
	client.Client
	awsAgent AWSRolePolicyManager
}

func NewServiceAccountReconciler(client client.Client, awsAgent AWSRolePolicyManager) *ServiceAccountReconciler {
	return &ServiceAccountReconciler{
		Client:   client,
		awsAgent: awsAgent,
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

	isReferencedByPods, exists := getServiceAccountLabelValue(&serviceAccount)
	if !exists {
		logger.Debug("serviceAccount not labeled with credentials-operator.otterize.com/service-account, skipping")
		return ctrl.Result{}, nil
	}

	isNoLongerReferencedByPodsOrIsBeingDeleted := serviceAccount.DeletionTimestamp != nil || !isReferencedByPods

	if isNoLongerReferencedByPodsOrIsBeingDeleted {
		err = r.awsAgent.DeleteOtterizeIAMRole(ctx, req.Namespace, req.Name)

		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove service account: %w", err)
		}

		if serviceAccount.DeletionTimestamp != nil {
			updatedServiceAccount := serviceAccount.DeepCopy()
			if controllerutil.RemoveFinalizer(updatedServiceAccount, metadata.AWSRoleFinalizer) {
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

	updatedServiceAccount := serviceAccount.DeepCopy()
	if controllerutil.AddFinalizer(updatedServiceAccount, metadata.AWSRoleFinalizer) {
		err := r.Client.Patch(ctx, updatedServiceAccount, client.MergeFrom(&serviceAccount))
		if err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, errors.Wrap(err)
		}
	}
	shouldUpdateAnnotation, role, err := r.reconcileAWSRole(ctx, &serviceAccount)

	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	if shouldUpdateAnnotation {
		if updatedServiceAccount.Annotations == nil {
			updatedServiceAccount.Annotations = make(map[string]string)
		}

		updatedServiceAccount.Annotations[metadata.ServiceAccountAWSRoleARNAnnotation] = *role.Arn
		err = r.Client.Patch(ctx, updatedServiceAccount, client.MergeFrom(&serviceAccount))
		if err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, errors.Wrap(err)
		}
	}

	return ctrl.Result{}, nil
}

// reconcileAWSRole ensures the AWS role for the service account exists
// return values:
// - modified: if AWS state was modified (and the ServiceAccount should be re-queued)
// - role: the AWS IAM role
// - err
func (r *ServiceAccountReconciler) reconcileAWSRole(ctx context.Context, serviceAccount *corev1.ServiceAccount) (updateAnnotation bool, role *types.Role, err error) {
	logger := logrus.WithFields(logrus.Fields{"serviceAccount": serviceAccount.Name, "namespace": serviceAccount.Namespace})

	if roleARN, ok := hasAWSAnnotation(serviceAccount); ok {
		generatedRoleARN := r.awsAgent.GenerateRoleARN(serviceAccount.Namespace, serviceAccount.Name)
		found, role, err := r.awsAgent.GetOtterizeRole(ctx, serviceAccount.Namespace, serviceAccount.Name)

		if err != nil {
			return false, nil, fmt.Errorf("failed getting AWS role: %w", err)
		}

		if found {
			if generatedRoleARN != roleARN {
				logger.WithField("arn", *role.Arn).Debug("ServiceAccount AWS role exists, but annotation is misconfigured, should be updated")
				return true, role, nil
			}
			logger.WithField("arn", *role.Arn).Debug("ServiceAccount has matching AWS role")
			return false, role, nil
		}
	}

	role, err = r.awsAgent.CreateOtterizeIAMRole(ctx, serviceAccount.Namespace, serviceAccount.Name)
	if err != nil {
		return true, nil, fmt.Errorf("failed creating AWS role for service account: %w", err)
	}

	logger.WithField("arn", *role.Arn).Info("created AWS role for ServiceAccount")
	return true, role, nil
}

func hasAWSAnnotation(serviceAccount *corev1.ServiceAccount) (string, bool) {
	if serviceAccount.Annotations == nil {
		return "", false
	}

	roleARN, ok := serviceAccount.Annotations[metadata.ServiceAccountAWSRoleARNAnnotation]
	return roleARN, ok
}

func getServiceAccountLabelValue(serviceAccount *corev1.ServiceAccount) (hasPods bool, exists bool) {
	if serviceAccount.Labels == nil {
		return false, false
	}

	value, ok := serviceAccount.Labels[metadata.OtterizeServiceAccountLabel]
	return value == "true", ok
}
