package controllers

import (
	"context"
	"errors"
	"github.com/otterize/spifferize/src/operator/secrets"
	"github.com/otterize/spifferize/src/spireclient"
	"github.com/otterize/spifferize/src/spireclient/entries"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	refreshSecretsLoopTick   = time.Minute
	ServiceNameInputLabel    = "otterize/service-name"
	TLSSecretNameLabel       = "otterize/tls-secret-name"
	ServiceNameSelectorLabel = "spifferize/selector-service-name"
)

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	SpireClient     spireclient.ServerClient
	EntriesRegistry entries.Registry
	SecretsManager  secrets.Manager
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=create;get;list;update;patch
// +kubebuilder:rbac:groups=apps,resources=replicasets,deamonsets,statefulsets,verbs=get;list;watch

func (r *PodReconciler) resolvePodToOwnerName(ctx context.Context, pod *corev1.Pod) (string, error) {
	for _, owner := range pod.OwnerReferences {
		namespacedName := types.NamespacedName{Name: owner.Name, Namespace: pod.Namespace}
		switch owner.Kind {
		case "ReplicaSet":
			rs := &appsv1.ReplicaSet{}
			err := r.Get(ctx, namespacedName, rs)
			if err != nil {
				return "", err
			}
			return rs.OwnerReferences[0].Name, nil
		case "DaemonSet":
			ds := &appsv1.DaemonSet{}
			err := r.Get(ctx, namespacedName, ds)
			if err != nil {
				return "", err
			}
			return ds.Name, nil
		case "StatefulSet":
			ss := &appsv1.StatefulSet{}
			err := r.Get(ctx, namespacedName, ss)
			if err != nil {
				return "", err
			}
			return ss.Name, nil
		default:
			logrus.WithFields(logrus.Fields{"pod": pod.Name, "namespace": pod.Namespace, "owner.kind": owner.Kind}).Warn("Unknown owner kind")
		}
	}
	return "", errors.New("pod has no known owners")
}

func (r *PodReconciler) resolvePodToServiceName(ctx context.Context, pod *corev1.Pod) (string, error) {
	log := logrus.WithFields(logrus.Fields{"pod": pod.Name, "namespace": pod.Namespace})
	if pod.Labels != nil && pod.Labels[ServiceNameInputLabel] != "" {
		serviceName := pod.Labels[ServiceNameInputLabel]
		log.WithFields(logrus.Fields{"label.key": ServiceNameInputLabel, "label.value": serviceName}).Info("using service name from pod label")
		return serviceName, nil
	}

	ownerName, err := r.resolvePodToOwnerName(ctx, pod)
	if err != nil {
		log.WithError(err).Error("failed resolving pod owner")
		return "", err
	}

	log.WithField("ownerName", ownerName).Info("using service name from pod owner")
	return ownerName, nil
}

func (r *PodReconciler) updatePodLabel(ctx context.Context, pod *corev1.Pod, labelKey string, labelValue string) (ctrl.Result, error) {
	log := logrus.WithFields(logrus.Fields{"pod": pod.Name, "namespace": pod.Namespace, "label.key": labelKey, "label.value": labelValue})

	if pod.Labels != nil && pod.Labels[labelKey] == labelValue {
		log.Info("no updates required - label already exists")
		return ctrl.Result{}, nil
	}

	log.Info("updating pod label")

	if pod.Labels == nil {
		pod.Labels = map[string]string{}
	}

	pod.Labels[labelKey] = labelValue

	if err := r.Update(ctx, pod); err != nil {
		if apierrors.IsConflict(err) {
			// The Pod has been updated since we read it.
			// Requeue the Pod to try to reconciliate again.
			return ctrl.Result{Requeue: true}, nil
		}
		if apierrors.IsNotFound(err) {
			// The Pod has been deleted since we read it.
			// Requeue the Pod to try to reconciliate again.
			return ctrl.Result{Requeue: true}, nil
		}
		log.WithError(err).Error("failed updating Pod")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *PodReconciler) generatePodTLSSecret(ctx context.Context, pod *corev1.Pod, serviceName string, spiffeID spiffeid.ID) error {
	log := logrus.WithFields(logrus.Fields{"pod": pod.Name, "namespace": pod.Namespace})
	if pod.Labels == nil || pod.Labels[TLSSecretNameLabel] == "" {
		log.WithField("label.key", TLSSecretNameLabel).Info("skipping TLS secrets creation - label not found")
		return nil
	}

	secretName := pod.Labels[TLSSecretNameLabel]
	log.WithFields(logrus.Fields{"label.key": TLSSecretNameLabel, "label.value": secretName}).Info("ensuring TLS secret")
	if err := r.SecretsManager.EnsureTLSSecret(ctx, pod.Namespace, secretName, serviceName, spiffeID); err != nil {
		log.WithError(err).Error("failed creating TLS secret")
		return err
	}

	return nil
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
		log.WithError(err).Error("unable to fetch Pod")
		return ctrl.Result{}, err
	}

	log.Info("updating SPIRE entries & secrets for pod")

	// resolve pod to otterize service name
	serviceName, err := r.resolvePodToServiceName(ctx, &pod)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Ensure the ServiceNameSelectorLabel is set
	result, err := r.updatePodLabel(ctx, &pod, ServiceNameSelectorLabel, serviceName)
	if err != nil || !result.IsZero() {
		return result, err
	}

	// Add spire-server entry for pod
	spiffeID, err := r.EntriesRegistry.RegisterK8SPodEntry(ctx, pod.Namespace, ServiceNameSelectorLabel, serviceName)
	if err != nil {
		log.WithError(err).Error("failed registering SPIRE entry for pod")
		return ctrl.Result{}, err
	}

	// generate TLS secret for pod
	if err := r.generatePodTLSSecret(ctx, &pod, serviceName, spiffeID); err != nil {
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

func (r *PodReconciler) RefreshSecretsLoop(ctx context.Context) {
	for {
		select {
		case <-time.After(refreshSecretsLoopTick):
			err := r.SecretsManager.RefreshTLSSecrets(ctx)
			if err != nil {
				logrus.WithError(err).Error("failed refreshing TLS secrets")
			}
		case <-ctx.Done():
			return
		}
	}
}
