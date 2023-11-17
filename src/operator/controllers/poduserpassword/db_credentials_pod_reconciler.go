package poduserpassword

import (
	"context"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/otterize/credentials-operator/src/controllers/otterizeclient/otterizegraphql"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

const (
	ReasonEnsuredPodUserAndPassword        = "EnsuredPodUserAndPassword"
	ReasonEnsuringPodUserAndPasswordFailed = "EnsuringPodUserAndPasswordFailed"
	ReasonPodOwnerResolutionFailed         = "PodOwnerResolutionFailed"
)

type CloudUserAndPasswordAcquirer interface {
	AcquireServiceUserAndPassword(ctx context.Context, serviceName, namespace string) (*otterizegraphql.UserPasswordCredentials, error)
}

type Reconciler struct {
	client                  client.Client
	scheme                  *runtime.Scheme
	recorder                record.EventRecorder
	serviceIdResolver       *serviceidresolver.Resolver
	userAndPasswordAcquirer CloudUserAndPasswordAcquirer
}

func NewReconciler(client client.Client, scheme *runtime.Scheme, eventRecorder record.EventRecorder, serviceIdResolver *serviceidresolver.Resolver, acquirer CloudUserAndPasswordAcquirer) *Reconciler {
	return &Reconciler{
		client:                  client,
		scheme:                  scheme,
		serviceIdResolver:       serviceIdResolver,
		recorder:                eventRecorder,
		userAndPasswordAcquirer: acquirer,
	}
}

func (e *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		For(&v1.Pod{}).
		Complete(e)
}

func (e *Reconciler) shouldCreateUserAndPasswordSecretForPod(pod v1.Pod) bool {
	return pod.Annotations != nil && hasUserAndPasswordSecretAnnotation(pod)
}

func hasUserAndPasswordSecretAnnotation(pod v1.Pod) bool {
	_, ok := pod.Annotations[metadata.UserAndPasswordSecretNameAnnotation]
	return ok
}

func (e *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var pod v1.Pod
	err := e.client.Get(ctx, req.NamespacedName, &pod)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !e.shouldCreateUserAndPasswordSecretForPod(pod) {
		return ctrl.Result{}, nil
	}

	logrus.Debug("Ensuring user-password credentials secrets for pod")
	// resolve pod to otterize service name
	serviceID, err := e.serviceIdResolver.ResolvePodToServiceIdentity(ctx, &pod)
	if err != nil {
		e.recorder.Eventf(&pod, v1.EventTypeWarning, ReasonPodOwnerResolutionFailed, "Could not resolve pod to its owner: %s", err.Error())
		return ctrl.Result{}, err
	}

	err = e.ensurePodUserAndPasswordSecret(ctx, &pod, serviceID.Name, pod.Annotations[metadata.UserAndPasswordSecretNameAnnotation])
	if err != nil {
		e.recorder.Eventf(&pod, v1.EventTypeWarning, ReasonEnsuringPodUserAndPasswordFailed, "Failed to ensure user-password credentials secret: %s", err.Error())
		return ctrl.Result{}, err
	}

	e.recorder.Event(&pod, v1.EventTypeNormal, ReasonEnsuredPodUserAndPassword, "Ensured user-password credentials in specified secret")
	return ctrl.Result{}, nil
}

func (e *Reconciler) ensurePodUserAndPasswordSecret(ctx context.Context, pod *v1.Pod, serviceName string, secretName string) error {
	log := logrus.WithFields(logrus.Fields{"pod": pod.Name, "namespace": pod.Namespace})
	err := e.client.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: secretName}, &v1.Secret{})
	if apierrors.IsNotFound(err) {
		log.Debug("Creating user-password credentials secret for pod")
		creds, err := e.userAndPasswordAcquirer.AcquireServiceUserAndPassword(ctx, serviceName, pod.Namespace)
		if err != nil {
			return err
		}

		secret := buildUserAndPasswordCredentialsSecret(secretName, pod.Namespace, creds)
		log.WithField("secret", secretName).Debug("Creating new secret with user-password credentials")
		if err := e.client.Create(ctx, secret); err != nil {
			return err
		}
		return nil
	}

	if err != nil {
		return err
	}
	log.Debug("Secret exists, nothing to do")
	return nil
}

func buildUserAndPasswordCredentialsSecret(name, namespace string, creds *otterizegraphql.UserPasswordCredentials) *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"username": []byte(creds.Username),
			"password": []byte(creds.Password),
		},
		Type: v1.SecretTypeOpaque,
	}
}
