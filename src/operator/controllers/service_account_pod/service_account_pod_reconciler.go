package service_account_pod

import (
	"context"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/otterize/intents-operator/src/shared/awsagent"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

const (
	ReasonGetServiceAccountFailed      = "GetServiceAccountFailed"
	ReasonCreatingServiceAccountFailed = "ServiceAccountCreationFailed"
	ReasonServiceAccountUpdated        = "ServiceAccountUpdated"
	ReasonServiceAccountUpdateFailed   = "ServiceAccountUpdateFailed"
)

type PodServiceAccountReconciler struct {
	client   client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	awsAgent *awsagent.Agent
}

func NewPodServiceAccountReconciler(client client.Client, scheme *runtime.Scheme, eventRecorder record.EventRecorder, awsAgent *awsagent.Agent) *PodServiceAccountReconciler {
	return &PodServiceAccountReconciler{
		client:   client,
		scheme:   scheme,
		awsAgent: awsAgent,
		recorder: eventRecorder,
	}
}

func (r *PodServiceAccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		For(&v1.Pod{}).
		Complete(r)
}

func (e *PodServiceAccountReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var pod v1.Pod
	err := e.client.Get(ctx, req.NamespacedName, &pod)
	if err != nil {
		return ctrl.Result{}, err
	}

	if pod.Annotations == nil {
		return ctrl.Result{}, nil
	}
	_, annotationExists := pod.Annotations[metadata.CreateAWSRoleAnnotation]
	if !annotationExists {
		logrus.Debugf("pod %v doesn't have create AWS IAM role annotation, skipping", pod)
		return ctrl.Result{}, nil
	}

	serviceAccount := v1.ServiceAccount{}
	err = e.client.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: pod.Spec.ServiceAccountName}, &serviceAccount)

	if err != nil {
		e.recorder.Eventf(&pod, v1.EventTypeWarning, ReasonGetServiceAccountFailed, "Failed getting service account: %s for pod: %v", pod.Spec.ServiceAccountName, pod)
		return ctrl.Result{}, err
	}

	logrus.Debugf("service account %s exists, Updating it", pod.Spec.ServiceAccountName)
	updatedServiceAccount := serviceAccount.DeepCopy()
	if updatedServiceAccount.Labels == nil {
		updatedServiceAccount.Labels = make(map[string]string)
	}
	previousServiceAccountValue, ok := serviceAccount.Labels[metadata.OtterizeServiceAccountLabel]
	if !ok || previousServiceAccountValue != pod.Spec.ServiceAccountName {
		updatedServiceAccount.Labels[metadata.OtterizeServiceAccountLabel] = pod.Spec.ServiceAccountName
		err := e.client.Patch(ctx, updatedServiceAccount, client.MergeFrom(&serviceAccount))
		if err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			e.recorder.Eventf(&pod, v1.EventTypeWarning, ReasonServiceAccountUpdateFailed, "failed to update pre-existing service account with Otterize label: %s", err.Error())
			return ctrl.Result{}, err
		}
		e.recorder.Eventf(&pod, v1.EventTypeNormal, ReasonServiceAccountUpdated, "service account %s already exists updated labels, labeling", pod.Spec.ServiceAccountName)
	}

	return ctrl.Result{}, nil
}
