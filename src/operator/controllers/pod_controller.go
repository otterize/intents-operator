package controllers

import (
	"context"
	"fmt"
	"github.com/asaskevich/govalidator"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/spire-integration-operator/src/operator/secrets"
	"github.com/otterize/spire-integration-operator/src/spireclient/entries"
	"github.com/sirupsen/logrus"
	"hash/fnv"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"strings"
	"time"
)

const (
	refreshSecretsLoopTick      = time.Minute
	TLSSecretNameAnnotation     = "otterize/tls-secret-name"
	SVIDFileNameAnnotation      = "otterize/svid-file-name"
	BundleFileNameAnnotation    = "otterize/bundle-file-name"
	KeyFileNameAnnotation       = "otterize/key-file-name"
	DNSNamesAnnotation          = "otterize/dns-names"
	CertTTLAnnotation           = "otterize/cert-ttl"
	CertTypeAnnotation          = "otterize/cert-type"
	KeyStoreNameAnnotation      = "otterize/keystore-file-name"
	TrustStoreNameAnnotation    = "otterize/truststore-file-name"
	JksStoresPasswordAnnotation = "otterize/jks-password"
	ServiceNameSelectorLabel    = "otterize/spire-integration-operator/service-name"
)

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	scheme            *runtime.Scheme
	entriesRegistry   entries.Registry
	secretsManager    secrets.Manager
	serviceIdResolver *serviceidresolver.Resolver
	eventRecorder     record.EventRecorder
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=create;get;list;update;patch
// +kubebuilder:rbac:groups=apps,resources=replicasets,deamonsets,statefulsets,verbs=get;list;watch

func NewPodReconciler(client client.Client, scheme *runtime.Scheme, entriesRegistry entries.Registry,
	secretsManager secrets.Manager, serviceIdResolver *serviceidresolver.Resolver, eventRecorder record.EventRecorder) *PodReconciler {
	return &PodReconciler{
		Client:            client,
		scheme:            scheme,
		entriesRegistry:   entriesRegistry,
		secretsManager:    secretsManager,
		serviceIdResolver: serviceIdResolver,
		eventRecorder:     eventRecorder,
	}
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

func (r *PodReconciler) ensurePodTLSSecret(ctx context.Context, pod *corev1.Pod, serviceName string, entryID string, entryHash string) error {
	log := logrus.WithFields(logrus.Fields{"pod": pod.Name, "namespace": pod.Namespace})
	if pod.Annotations == nil || pod.Annotations[TLSSecretNameAnnotation] == "" {
		log.WithField("annotation.key", TLSSecretNameAnnotation).Info("skipping TLS secrets creation - annotation not found")
		return nil
	}

	secretName := pod.Annotations[TLSSecretNameAnnotation]
	certConfig := certConfigFromPod(pod)
	log.WithFields(logrus.Fields{"secret_name": secretName, "cert_config": certConfig}).Info("ensuring TLS secret")
	secretConfig := secrets.NewSecretConfig(entryID, entryHash, secretName, pod.Namespace, serviceName, certConfig)
	if err := r.secretsManager.EnsureTLSSecret(ctx, secretConfig); err != nil {
		log.WithError(err).Error("failed creating TLS secret")
		return err
	}

	r.eventRecorder.Eventf(pod, corev1.EventTypeNormal, "Successfully ensured secret under name '%s'", secretName)

	return nil
}

func certConfigFromPod(pod *corev1.Pod) secrets.CertConfig {
	certType := secrets.StrToCertType(pod.Annotations[CertTypeAnnotation])
	certConfig := secrets.CertConfig{CertType: certType}
	switch certType {
	case secrets.PemCertType:
		certConfig.PemConfig = secrets.NewPemConfig(pod.Annotations[SVIDFileNameAnnotation], pod.Annotations[BundleFileNameAnnotation], pod.Annotations[KeyFileNameAnnotation])
	case secrets.JksCertType:
		certConfig.JksConfig = secrets.NewJksConfig(pod.Annotations[KeyStoreNameAnnotation], pod.Annotations[TrustStoreNameAnnotation], pod.Annotations[JksStoresPasswordAnnotation])

	}
	return certConfig
}

func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logrus.WithField("pod", req.NamespacedName)

	// Fetch the Pod from the Kubernetes API.
	pod := &corev1.Pod{}
	if err := r.Get(ctx, req.NamespacedName, pod); err != nil {
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
	serviceID, err := r.serviceIdResolver.ResolvePodToServiceIdentity(ctx, pod)
	if err != nil {
		r.eventRecorder.Eventf(pod, corev1.EventTypeWarning, "Pod owner resolving failed", "could not resolve pod to its owner: %s", err.Error())
		return ctrl.Result{}, err
	}

	// Ensure the ServiceNameSelectorLabel is set
	result, err := r.updatePodLabel(ctx, pod, ServiceNameSelectorLabel, serviceID)
	if err != nil || result.Requeue {
		r.eventRecorder.Event(pod, corev1.EventTypeWarning, "Pod label update failed", err.Error())
		return result, err
	}

	dnsNames, err := r.resolvePodToCertDNSNames(pod)
	if err != nil {
		err = fmt.Errorf("error resolving pod cert DNS names, will continue with an empty DNS names list: %w", err)
		r.eventRecorder.Event(pod, corev1.EventTypeWarning, "Resolving cert DNS names failed", err.Error())
		return ctrl.Result{}, err
	}

	ttl, err := r.resolvePodToCertTTl(pod)
	if err != nil {
		r.eventRecorder.Event(pod, corev1.EventTypeWarning, "Getting cert TTL failed", err.Error())
		return ctrl.Result{}, err
	}

	// Add spire-server entry for pod
	entryID, err := r.entriesRegistry.RegisterK8SPodEntry(ctx, pod.Namespace, ServiceNameSelectorLabel, serviceID, ttl, dnsNames)
	if err != nil {
		log.WithError(err).Error("failed registering SPIRE entry for pod")
		r.eventRecorder.Event(pod, corev1.EventTypeWarning, "Failed registering SPIRE entry", err.Error())
		return ctrl.Result{}, err
	}
	r.eventRecorder.Event(pod, corev1.EventTypeNormal, "Successfully registered pod under SPIRE with entry ID '%s'", entryID)

	hashStr, err := r.getEntryHash(pod.Namespace, serviceID, ttl, dnsNames)
	if err != nil {
		r.eventRecorder.Event(pod, corev1.EventTypeWarning, "Failed calculating SPIRE entry hash", err.Error())
		return ctrl.Result{}, err
	}

	// generate TLS secret for pod
	if err := r.ensurePodTLSSecret(ctx, pod, serviceID, entryID, hashStr); err != nil {
		r.eventRecorder.Event(pod, corev1.EventTypeWarning, "Failed creating TLS secret for pod", err.Error())
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *PodReconciler) getEntryHash(namespace string, serviceName string, ttl int32, dnsNames []string) (string, error) {
	entryPropertiesHashMaker := fnv.New32a()
	_, err := entryPropertiesHashMaker.Write([]byte(namespace + ServiceNameSelectorLabel + serviceName + string(ttl) + strings.Join(dnsNames, "")))
	if err != nil {
		return "", fmt.Errorf("failed hashing SPIRE entry properties %w", err)
	}
	hashStr := strconv.Itoa(int(entryPropertiesHashMaker.Sum32()))
	return hashStr, nil
}

func (r *PodReconciler) resolvePodToCertDNSNames(pod *corev1.Pod) ([]string, error) {
	if len(pod.Annotations[DNSNamesAnnotation]) == 0 {
		return nil, nil
	}

	dnsNames := strings.Split(pod.Annotations[DNSNamesAnnotation], ",")
	for _, name := range dnsNames {
		if !govalidator.IsDNSName(name) {
			return nil, fmt.Errorf("invalid DNS name: %s", name)
		}
	}
	return dnsNames, nil
}

func (r *PodReconciler) resolvePodToCertTTl(pod *corev1.Pod) (int32, error) {
	ttlString := pod.Annotations[CertTTLAnnotation]
	if len(ttlString) == 0 {
		return 0, nil
	}

	ttl64, err := strconv.ParseInt(ttlString, 0, 32)

	if err != nil {
		return 0, fmt.Errorf("failed converting ttl: %s str to int. %w", ttlString, err)
	}

	return int32(ttl64), nil
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
			err := r.secretsManager.RefreshTLSSecrets(ctx)
			if err != nil {
				logrus.WithError(err).Error("failed refreshing TLS secrets")
			}
		case <-ctx.Done():
			return
		}
	}
}
