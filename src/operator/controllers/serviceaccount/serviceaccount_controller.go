package serviceaccount

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/otterize/intents-operator/src/shared/awsagent"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

type ServiceAccountReconciler struct {
	client.Client
	scheme   *runtime.Scheme
	awsAgent *awsagent.Agent
}

func NewServiceAccountReconciler(client client.Client, scheme *runtime.Scheme, awsAgent *awsagent.Agent) *ServiceAccountReconciler {
	return &ServiceAccountReconciler{
		Client:   client,
		scheme:   scheme,
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

	if err := r.Get(ctx, req.NamespacedName, &serviceAccount); err != nil {
		if apierrors.IsNotFound(err) {
			err = r.awsAgent.DeleteOtterizeIAMRole(ctx, req.Namespace, req.Name)

			if err != nil {
				logger.WithError(err).Errorf("failed to remove service account")
			}

			return ctrl.Result{}, err
		}

		logger.WithError(err).Error("unable to fetch ServiceAccount")
		return ctrl.Result{}, err
	}

	shouldUpdateAnnotation, role, err := r.reconcileAWSRole(ctx, &serviceAccount)

	if err != nil {
		return ctrl.Result{}, err
	}

	if shouldUpdateAnnotation {
		updatedServiceAccount := serviceAccount.DeepCopy()
		if updatedServiceAccount.Annotations == nil {
			updatedServiceAccount.Annotations = make(map[string]string)
		}

		updatedServiceAccount.Annotations[metadata.ServiceAccountAWSRoleARNAnnotation] = *role.Arn
		err = r.Client.Patch(ctx, updatedServiceAccount, client.MergeFrom(&serviceAccount))
		if err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
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

	if !hasOtterizeServiceAccountLabel(serviceAccount) {
		logger.Debug("serviceAccount not labeled with credentials-operator.otterize.com/service-account, skipping")
		return false, nil, nil
	}

	if roleARN, ok := hasAWSAnnotation(serviceAccount); ok {
		generatedRoleARN := r.awsAgent.GenerateRoleARN(serviceAccount.Namespace, serviceAccount.Name)
		found, role, err := r.awsAgent.GetOtterizeRole(ctx, serviceAccount.Namespace, serviceAccount.Name)

		if err != nil {
			return false, nil, fmt.Errorf("failed getting AWS role: %w", err)
		}

		if found {
			if generatedRoleARN != roleARN {
				logger.WithField("arn", role.Arn).Debug("ServiceAccount AWS role exists, but annotation is misconfigured, should be updated")
				return true, role, nil
			}
			logger.WithField("arn", role.Arn).Debug("ServiceAccount has matching AWS role")
			return false, role, nil
		}
	}

	role, err = r.awsAgent.CreateOtterizeIAMRole(ctx, serviceAccount.Namespace, serviceAccount.Name)
	if err != nil {
		return true, nil, fmt.Errorf("failed creating AWS role for service account: %w", err)
	}

	logger.WithField("arn", role.Arn).Info("created AWS role for ServiceAccount")
	return true, role, nil
}

func hasAWSAnnotation(serviceAccount *corev1.ServiceAccount) (string, bool) {
	if serviceAccount.Annotations == nil {
		return "", false
	}

	roleARN, ok := serviceAccount.Annotations[metadata.ServiceAccountAWSRoleARNAnnotation]
	return roleARN, ok
}

func hasOtterizeServiceAccountLabel(serviceAccount *corev1.ServiceAccount) bool {
	if serviceAccount.Labels == nil {
		return false
	}

	_, ok := serviceAccount.Labels[metadata.OtterizeServiceAccountLabel]
	return ok
}
