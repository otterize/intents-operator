package serviceaccount_old

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	rolesanywhereTypes "github.com/aws/aws-sdk-go-v2/service/rolesanywhere/types"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
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
	CreateOtterizeIAMRole(ctx context.Context, namespace string, name string, useSoftDeleteStrategy bool) (*types.Role, error)
	CreateRolesAnywhereProfileForRole(ctx context.Context, role types.Role, namespace string, serviceAccountName string) (*rolesanywhereTypes.ProfileDetail, error)
	DeleteRolesAnywhereProfileForServiceAccount(ctx context.Context, namespace string, serviceAccountName string) (bool, error)
}

type ServiceAccountReconciler struct {
	client.Client
	awsAgent                         AWSRolePolicyManager
	markRolesAsUnusedInsteadOfDelete bool
}

func NewServiceAccountReconciler(client client.Client, awsAgent AWSRolePolicyManager, markRolesAsUnusedInsteadOfDelete bool) *ServiceAccountReconciler {
	return &ServiceAccountReconciler{
		Client:                           client,
		awsAgent:                         awsAgent,
		markRolesAsUnusedInsteadOfDelete: markRolesAsUnusedInsteadOfDelete,
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

		if viper.GetBool(operatorconfig.EnableAWSRolesAnywhereKey) {
			deleted, err := r.awsAgent.DeleteRolesAnywhereProfileForServiceAccount(ctx, req.Namespace, req.Name)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to remove rolesanywhere profile for service account: %w", err)
			}

			if !deleted {
				logrus.Debugf("rolesanywhere profile for service account %s/%s did not exist when deletion was attempted", req.Namespace, req.Name)
			}

			logrus.WithFields(logrus.Fields{"serviceAccount": req.Name, "namespace": req.Namespace}).Debug("deleted rolesanywhere profile for service account")
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

	if viper.GetBool(operatorconfig.EnableAWSRolesAnywhereKey) {
		// In RolesAnywhere mode, the SPIFFE pod webhook, and not the reconciler, handles the role creation
		return ctrl.Result{}, nil
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
	roleARN, ok := hasAWSAnnotation(serviceAccount)

	// calling create in any case because this way we validate it is not soft-deleted and it is configured with the correct soft-delete strategy
	role, err = r.awsAgent.CreateOtterizeIAMRole(ctx, serviceAccount.Namespace, serviceAccount.Name, r.shouldUseSoftDeleteStrategy(serviceAccount))
	if err != nil {
		return false, nil, fmt.Errorf("failed creating AWS role for service account: %w", err)
	}
	logger.WithField("arn", *role.Arn).Info("created AWS role for ServiceAccount")

	// update annotation if it doesn't exist or if it is misconfigured
	shouldUpdate := !ok || roleARN != *role.Arn

	return shouldUpdate, role, nil
}

func (r *ServiceAccountReconciler) shouldUseSoftDeleteStrategy(serviceAccount *corev1.ServiceAccount) bool {
	if r.markRolesAsUnusedInsteadOfDelete {
		return true
	}
	if serviceAccount.Labels == nil {
		return false
	}

	softDeleteValue, shouldSoftDelete := serviceAccount.Labels[metadata.OtterizeAWSUseSoftDeleteKey]
	return shouldSoftDelete && softDeleteValue == metadata.OtterizeAWSUseSoftDeleteValue
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
