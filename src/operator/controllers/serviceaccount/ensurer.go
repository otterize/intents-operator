package serviceaccount

import (
	"context"
	"fmt"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ReasonCreateServiceAccount         = "CreateServiceAccount"
	ReasonCreatingServiceAccountFailed = "CreatingServiceAccountFailed"
	ReasonCreateServiceAccountSkipped  = "CreatingServiceAccountSkipped"
)

type Ensurer struct {
	client.Client
	recorder record.EventRecorder
}

func NewServiceAccountEnsurer(client client.Client, eventRecorder record.EventRecorder) *Ensurer {
	return &Ensurer{Client: client, recorder: eventRecorder}
}

func isServiceAccountNameValid(name string) bool {
	return len(validation.IsDNS1123Subdomain(name)) == 0
}

func (e *Ensurer) EnsureServiceAccount(ctx context.Context, pod *v1.Pod) error {
	if pod.Annotations == nil {
		return nil
	}
	serviceAccountName, annotationExists := pod.Annotations[metadata.ServiceAccountNameAnnotation]
	if !annotationExists {
		logrus.Debugf("pod %s does'nt have service account annotation, skipping ensure service account", pod)
		return nil
	}

	if !isServiceAccountNameValid(serviceAccountName) {
		err := fmt.Errorf("service account name %s is invalid according to 'RFC 1123 subdomain'. skipping service account ensure for pod %s", serviceAccountName, pod)
		logrus.Warningf(err.Error())
		e.recorder.Eventf(pod, v1.EventTypeWarning, ReasonCreatingServiceAccountFailed, err.Error())
		return err
	}

	serviceAccount := v1.ServiceAccount{}
	err := e.Client.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: serviceAccountName}, &serviceAccount)
	if err != nil && !apierrors.IsNotFound(err) {
		logrus.Errorf("failed get service accounts: %s", err.Error())
		e.recorder.Eventf(pod, v1.EventTypeWarning, ReasonCreatingServiceAccountFailed, "Failed creating service account: %s", err.Error())
		return err
	} else if err == nil {
		logrus.Debugf("service account %s already exists, skipping service account creation", serviceAccountName)
		e.recorder.Eventf(pod, v1.EventTypeNormal, ReasonCreateServiceAccountSkipped, "service account %s already exists, skipping service account creation", serviceAccountName)
		return nil
	}

	logrus.Infof("creating service account named %s for pod %s", serviceAccountName, pod)
	if err := e.createServiceAccount(ctx, serviceAccountName, pod); err != nil {
		logrus.Errorf("failed creating service account for pod %s: %s", pod, err.Error())
		e.recorder.Eventf(pod, v1.EventTypeWarning, ReasonCreatingServiceAccountFailed, "Failed creating service account: %s", err.Error())
		return err
	}
	e.recorder.Eventf(pod, v1.EventTypeNormal, ReasonCreateServiceAccount, "Successfully created service account: %s", serviceAccountName)
	logrus.Infof("successfuly created service account named %s for pod %s", serviceAccountName, pod)

	return nil
}

func (e *Ensurer) createServiceAccount(ctx context.Context, serviceAccountName string, pod *v1.Pod) error {
	serviceAccount := v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: serviceAccountName, Namespace: pod.Namespace, Labels: map[string]string{metadata.OtterizeServiceAccountLabel: serviceAccountName}},
	}
	return e.Client.Create(ctx, &serviceAccount)
}
