package controllers

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/otterize/intents-operator/src/shared/awsagent"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	if r.awsAgent != nil {
		modified, role, err := r.reconcileAWSRole(ctx, &serviceAccount)

		if err != nil {
			return ctrl.Result{}, err
		}

		if modified {
			if serviceAccount.Annotations == nil {
				serviceAccount.Annotations = make(map[string]string)
			}

			serviceAccount.Annotations[metadata.ServiceAccountAWSRoleARNAnnotation] = *role.Arn
			err = r.Client.Update(ctx, &serviceAccount)

			if err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true}, nil
		}
	}

	return ctrl.Result{}, nil
}

// reconcileAWSRole ensures the AWS role for the service account exists
// return values:
// - modified: if AWS state was modified (and the ServiceAccount should be re-queued)
// - role: the AWS IAM role
// - err
func (r *ServiceAccountReconciler) reconcileAWSRole(ctx context.Context, serviceAccount *corev1.ServiceAccount) (bool, *types.Role, error) {
	logger := logrus.WithFields(logrus.Fields{"serviceAccount": serviceAccount.Name, "namespace": serviceAccount.Namespace})

	if !hasOtterizeServiceAccountLabel(serviceAccount) {
		logger.Debug("serviceAccount not labelled with credentials-operator.otterize.com/service-account, skipping")
		return false, nil, nil
	}

	if hasAWSAnnotation(serviceAccount) {
		found, role, err := r.awsAgent.GetOtterizeRole(ctx, serviceAccount.Namespace, serviceAccount.Name)

		if err != nil {
			logger.WithError(err).Error("failed checking for AWS role")
			return true, nil, err
		}

		if found {
			logger.WithField("arn", role.Arn).Debug("ServiceAccount has matching AWS role")
			return false, role, nil
		}
	}

	role, err := r.awsAgent.CreateOtterizeIAMRole(ctx, serviceAccount.Namespace, serviceAccount.Name)

	if err != nil {
		logger.WithError(err).Error("failed creating AWS role for ServiceAccount")
		return true, nil, err
	}

	logger.WithField("arn", role.Arn).Info("created AWS role for ServiceAccount")
	return true, role, nil
}

func hasAWSAnnotation(serviceAccount *corev1.ServiceAccount) bool {
	if serviceAccount.Annotations == nil {
		return false
	}

	_, ok := serviceAccount.Annotations[metadata.ServiceAccountAWSRoleARNAnnotation]
	return ok
}

func hasOtterizeServiceAccountLabel(serviceAccount *corev1.ServiceAccount) bool {
	if serviceAccount.Labels == nil {
		return false
	}

	_, ok := serviceAccount.Labels[metadata.OtterizeServiceAccountLabel]
	return ok
}
