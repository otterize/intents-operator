package poduserpassword

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/aidarkhanov/nanoid"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/otterize/intents-operator/src/shared/clusterutils"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/pbkdf2"
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

const (
	DefaultCredentialsAlphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	DefaultCredentialsLen      = 16
)

type Reconciler struct {
	client            client.Client
	scheme            *runtime.Scheme
	recorder          record.EventRecorder
	serviceIdResolver *serviceidresolver.Resolver
}

func NewReconciler(client client.Client, scheme *runtime.Scheme, eventRecorder record.EventRecorder, serviceIdResolver *serviceidresolver.Resolver) *Reconciler {
	return &Reconciler{
		client:            client,
		scheme:            scheme,
		serviceIdResolver: serviceIdResolver,
		recorder:          eventRecorder,
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
		return ctrl.Result{}, errors.Wrap(err)
	}
	if !pod.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	if !e.shouldCreateUserAndPasswordSecretForPod(pod) {
		return ctrl.Result{}, nil
	}

	logrus.Debug("Ensuring user-password credentials secrets for pod")
	// resolve pod to otterize service name
	serviceID, err := e.serviceIdResolver.ResolvePodToServiceIdentity(ctx, &pod)
	if err != nil {
		e.recorder.Eventf(&pod, v1.EventTypeWarning, ReasonPodOwnerResolutionFailed, "Could not resolve pod to its owner: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	err = e.ensurePodUserAndPasswordPostgresSecret(ctx, &pod, serviceID.Name, pod.Annotations[metadata.UserAndPasswordSecretNameAnnotation])
	if err != nil {
		e.recorder.Eventf(&pod, v1.EventTypeWarning, ReasonEnsuringPodUserAndPasswordFailed, "Failed to ensure user-password credentials secret: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	e.recorder.Event(&pod, v1.EventTypeNormal, ReasonEnsuredPodUserAndPassword, "Ensured user-password credentials in specified secret")
	return ctrl.Result{}, nil
}

func (e *Reconciler) ensurePodUserAndPasswordPostgresSecret(ctx context.Context, pod *v1.Pod, serviceName string, secretName string) error {
	log := logrus.WithFields(logrus.Fields{"pod": pod.Name, "namespace": pod.Namespace})
	err := e.client.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: secretName}, &v1.Secret{})
	if apierrors.IsNotFound(err) {
		log.Debug("Creating user-password credentials secret for pod")
		password, err := createServicePassword()
		if err != nil {
			return errors.Wrap(err)
		}
		clusterID, err := clusterutils.GetClusterUID(ctx)
		if err != nil {
			return errors.Wrap(err)
		}

		username := clusterutils.BuildHashedUsername(serviceName, pod.Namespace, clusterID)
		pgUsername := clusterutils.KubernetesToPostgresName(username)

		secret := buildUserAndPasswordCredentialsSecret(secretName, pod.Namespace, pgUsername, password)
		log.WithField("secret", secretName).Debug("Creating new secret with user-password credentials")
		if err := e.client.Create(ctx, secret); err != nil {
			return errors.Wrap(err)
		}
		return nil
	}

	if err != nil {
		return errors.Wrap(err)
	}
	log.Debug("Secret exists, nothing to do")
	return nil
}

func buildUserAndPasswordCredentialsSecret(name, namespace, pgUsername, password string) *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"username": []byte(pgUsername),
			"password": []byte(password),
		},
		Type: v1.SecretTypeOpaque,
	}
}

func createServicePassword() (string, error) {
	password, err := nanoid.Generate(DefaultCredentialsAlphabet, DefaultCredentialsLen)
	if err != nil {
		return "", errors.Wrap(err)
	}
	salt, err := nanoid.Generate(DefaultCredentialsAlphabet, 8)
	if err != nil {
		return "", errors.Wrap(err)
	}

	dk := pbkdf2.Key([]byte(password), []byte(salt), 2048, 16, sha256.New)
	return hex.EncodeToString(dk), nil
}
