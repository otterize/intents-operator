package service_account_pod

import (
	"context"
	"fmt"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/otterize/intents-operator/src/shared/awsagent"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

const (
	ReasonServiceAccountCreated        = "ServiceAccountCreated"
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

func isServiceAccountNameValid(name string) bool {
	return len(validation.IsDNS1123Subdomain(name)) == 0
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
	serviceAccountName, annotationExists := pod.Annotations[metadata.ServiceAccountNameAnnotation]
	if !annotationExists {
		logrus.Debugf("pod %v doesn't have service account annotation, skipping ensure service account", pod)
		return ctrl.Result{}, nil
	}

	if !isServiceAccountNameValid(serviceAccountName) {
		err := fmt.Errorf("service account name %s is invalid according to 'RFC 1123 subdomain'. skipping service account ensure for pod %v", serviceAccountName, pod)
		e.recorder.Eventf(&pod, v1.EventTypeWarning, ReasonCreatingServiceAccountFailed, err.Error())
		return ctrl.Result{}, err
	}

	serviceAccount := v1.ServiceAccount{}
	err = e.client.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: serviceAccountName}, &serviceAccount)

	if apierrors.IsNotFound(err) {
		logrus.Debugf("creating service account named %s for pod/%s/%s", serviceAccountName, pod.Namespace, pod.Name)
		if err := e.createServiceAccount(ctx, serviceAccountName, &pod); err != nil {
			e.recorder.Eventf(&pod, v1.EventTypeWarning, ReasonCreatingServiceAccountFailed, "Failed creating service account: %s", err.Error())
			return ctrl.Result{}, err
		}
		e.recorder.Eventf(&pod, v1.EventTypeNormal, ReasonServiceAccountCreated, "Successfully created service account: %s", serviceAccountName)
		logrus.Debugf("successfuly created service account named %s for pod/%s/%s", serviceAccountName, pod.Namespace, pod.Name)
		return ctrl.Result{}, nil
	}

	if err != nil {
		e.recorder.Eventf(&pod, v1.EventTypeWarning, ReasonCreatingServiceAccountFailed, "Failed creating service account: %s", err.Error())
		return ctrl.Result{}, err
	}

	logrus.Debugf("service account %s already exists, Updating it", serviceAccountName)
	updatedServiceAccount := serviceAccount.DeepCopy()
	if updatedServiceAccount.Labels == nil {
		updatedServiceAccount.Labels = make(map[string]string)
	}
	previousServiceAccountValue, ok := serviceAccount.Labels[metadata.OtterizeServiceAccountLabel]
	if !ok || previousServiceAccountValue != serviceAccountName {
		updatedServiceAccount.Labels[metadata.OtterizeServiceAccountLabel] = serviceAccountName
		err := e.client.Patch(ctx, updatedServiceAccount, client.MergeFrom(&serviceAccount))
		if err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			e.recorder.Eventf(&pod, v1.EventTypeWarning, ReasonServiceAccountUpdateFailed, "failed to update pre-existing service account with Otterize label: %s", err.Error())
			return ctrl.Result{}, err
		}
		e.recorder.Eventf(&pod, v1.EventTypeNormal, ReasonServiceAccountUpdated, "service account %s already exists but is missing Otterize label, labeling", serviceAccountName)
	}

	return ctrl.Result{}, nil
}

func (e *PodServiceAccountReconciler) createServiceAccount(ctx context.Context, serviceAccountName string, pod *v1.Pod) error {
	serviceAccount := v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: pod.Namespace,
			Labels: map[string]string{
				metadata.OtterizeServiceAccountLabel: serviceAccountName,
			},
		},
	}

	return e.client.Create(ctx, &serviceAccount)
}
