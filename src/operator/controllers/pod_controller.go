package controllers

import (
	"context"
	"fmt"
	spire_client "github.com/otterize/spifferize/src/spire-client"
	"github.com/otterize/spifferize/src/spire-client/bundles"
	"github.com/otterize/spifferize/src/spire-client/entries"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	SpireClient    spire_client.ServerClient
	EntriesManager *entries.Manager
	BundlesManager *bundles.Manager
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=create;get;list;update;patch

const (
	tlsSecretPrefix = "spifferize-tls"
)

func (r *PodReconciler) ensureSecret(ctx context.Context, secret *corev1.Secret) error {
	log := logrus.WithFields(logrus.Fields{"secret.namespace": secret.Namespace, "secret.name": secret.Name})

	found := corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, &found); err != nil && apierrors.IsNotFound(err) {
		log.Info("Creating a new secret")
		if err := r.Create(ctx, secret); err != nil {
			return err
		}
		log.Info("Secret created")
		return nil
	} else if err != nil {
		return err
	}

	log.Info("Updating existing secret")
	if err := r.Update(ctx, secret); err != nil {
		return err
	}
	return nil
}

func (r *PodReconciler) ensureTLSSecret(ctx context.Context, namespace string, secretName string, spiffeID spiffeid.ID) error {
	trustBundle, err := r.BundlesManager.GetTrustBundle(ctx)
	if err != nil {
		return err
	}

	svid, err := r.BundlesManager.GetX509SVID(ctx, spiffeID)
	if err != nil {
		return err
	}

	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Annotations: map[string]string{
				"svid-expires-at": fmt.Sprintf("%d", svid.ExpiresAt),
			},
		},
		Data: map[string][]byte{
			"bundle.pem": trustBundle.BundlePEM,
			"key.pem":    svid.KeyPEM,
			"svid.pem":   svid.SVIDPEM,
		},
	}

	return r.ensureSecret(ctx, &secret)
}

func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logrus.WithField("pod", req.NamespacedName)

	// Fetch the Pod from the Kubernetes API.
	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			// we'll ignore not-found errors, since they can't be fixed by an immediate
			// requeue (we'll need to wait for a new notification), and we can get them
			// on deleted requests.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch Pod")
		return ctrl.Result{}, err
	}

	// Add spire-server entry for pod
	if pod.Labels == nil || pod.Labels[entries.ServiceNamePodLabel] == "" {
		log.Info("no update required - service name label not found")
		return ctrl.Result{}, nil
	}

	serviceName := pod.Labels[entries.ServiceNamePodLabel]
	spiffeID, err := r.EntriesManager.RegisterK8SPodEntry(ctx, pod.Namespace, serviceName)
	if err != nil {
		log.Error(err, "failed registering SPIRE entry for pod")
		return ctrl.Result{}, err
	}

	secretName := fmt.Sprintf("%s-%s", tlsSecretPrefix, serviceName)
	if err := r.ensureTLSSecret(ctx, pod.Namespace, secretName, spiffeID); err != nil {
		log.Error(err, "failed to create trust bundle & svid secret")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(r)
}
