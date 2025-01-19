package tls_pod

import (
	"context"
	"github.com/amit7itz/goset"
	"github.com/asaskevich/govalidator"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	secretstypes "github.com/otterize/credentials-operator/src/controllers/secrets/types"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"hash/fnv"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"strconv"
	"strings"
	"time"
)

const (
	refreshSecretsLoopTick           = time.Minute
	cleanupOrphanEntriesLoopTick     = 10 * time.Minute
	ReasonEnsuredPodTLS              = "EnsuredPodTLS"
	ReasonEnsuringPodTLSFailed       = "EnsuringPodTLSFailed"
	ReasonPodOwnerResolutionFailed   = "PodOwnerResolutionFailed"
	ReasonPodLabelUpdateFailed       = "PodLabelUpdateFailed"
	ReasonCertDNSResolutionFailed    = "CertDNSResolutionFailed"
	ReasonCertTTLError               = "CertTTLError"
	ReasonEntryRegistrationFailed    = "EntryRegistrationFailed"
	ReasonPodRegistered              = "PodRegistered"
	ReasonEntryHashCalculationFailed = "EntryHashCalculationFailed"
	ReasonUsingDeprecatedAnnotations = "UsingDeprecatedAnnotations"
)

type WorkloadRegistry interface {
	RegisterK8SPod(ctx context.Context, namespace string, serviceNameLabel string, serviceName string, ttl int32, dnsNames []string) (string, error)
	CleanupOrphanK8SPodEntries(ctx context.Context, serviceNameLabel string, existingServicesByNamespace map[string]*goset.Set[string]) error
}

type SecretsManager interface {
	EnsureTLSSecret(ctx context.Context, config secretstypes.SecretConfig, pod *corev1.Pod) error
	RefreshTLSSecrets(ctx context.Context) error
}

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	scheme                               *runtime.Scheme
	workloadRegistry                     WorkloadRegistry
	secretsManager                       SecretsManager
	serviceIdResolver                    *serviceidresolver.Resolver
	eventRecorder                        record.EventRecorder
	registerOnlyPodsWithSecretAnnotation bool
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=create;get;list;update;patch;watch
// +kubebuilder:rbac:groups=apps,resources=replicasets;daemonsets;statefulsets;deployments,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=get;update;patch;list;watch;create
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;update;patch;list;watch;create

// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=iam.cnrm.cloud.google.com,resources=iamserviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=iam.cnrm.cloud.google.com,resources=iamserviceaccountkeys,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=iam.cnrm.cloud.google.com,resources=iampolicymembers,verbs=get;list;watch;create;update;patch;delete

func NewCertificatePodReconciler(client client.Client, scheme *runtime.Scheme, workloadRegistry WorkloadRegistry,
	secretsManager SecretsManager, serviceIdResolver *serviceidresolver.Resolver, eventRecorder record.EventRecorder,
	registerOnlyPodsWithSecretAnnotation bool) *PodReconciler {
	return &PodReconciler{
		Client:                               client,
		scheme:                               scheme,
		workloadRegistry:                     workloadRegistry,
		secretsManager:                       secretsManager,
		serviceIdResolver:                    serviceIdResolver,
		eventRecorder:                        eventRecorder,
		registerOnlyPodsWithSecretAnnotation: registerOnlyPodsWithSecretAnnotation,
	}
}

func (r *PodReconciler) updatePodLabel(ctx context.Context, pod *corev1.Pod, labelKey string, labelValue string) (ctrl.Result, error) {
	log := logrus.WithFields(logrus.Fields{"pod": pod.Name, "namespace": pod.Namespace, "label.key": labelKey, "label.value": labelValue})

	if pod.Labels != nil && pod.Labels[labelKey] == labelValue {
		log.Info("no updates required - label already exists")
		return ctrl.Result{}, nil
	}

	log.Info("updating pod label")

	updatedPod := pod.DeepCopy()
	if pod.Labels == nil {
		updatedPod.Labels = map[string]string{}
	}

	updatedPod.Labels[labelKey] = labelValue

	if err := r.Patch(ctx, updatedPod, client.MergeFrom(pod)); err != nil {
		if apierrors.IsConflict(err) || apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
			// The Pod has been updated since we read it.
			// Requeue the Pod to try to reconcile again.
			return ctrl.Result{Requeue: true}, nil
		}
		if apierrors.IsNotFound(err) {
			// The Pod has been deleted since we read it.
			// Requeue the Pod to try to reconciliate again.
			return ctrl.Result{Requeue: true}, nil
		}
		log.WithError(err).Error("failed updating Pod")
		return ctrl.Result{}, errors.Wrap(err)
	}

	return ctrl.Result{}, nil
}

func (r *PodReconciler) ensurePodTLSSecret(ctx context.Context, pod *corev1.Pod, serviceName string, entryID string, entryHash string, shouldRestartPodOnRenewal bool) error {
	log := logrus.WithFields(logrus.Fields{"pod": pod.Name, "namespace": pod.Namespace})
	secretName := metadata.GetAnnotationValue(pod.Annotations, metadata.TLSSecretNameAnnotation)
	certConfig, err := certConfigFromPod(pod)
	if err != nil {
		return errors.Errorf("failed parsing annotations: %w", err)
	}
	log.WithFields(logrus.Fields{"secret_name": secretName, "cert_config": certConfig}).Info("ensuring TLS secret")
	secretConfig := secretstypes.NewSecretConfig(entryID, entryHash, secretName, pod.Namespace, serviceName, certConfig, shouldRestartPodOnRenewal)
	if err := r.secretsManager.EnsureTLSSecret(ctx, secretConfig, pod); err != nil {
		log.WithError(err).Error("failed creating TLS secret")
		return errors.Wrap(err)
	}

	r.eventRecorder.Eventf(pod, corev1.EventTypeNormal, ReasonEnsuredPodTLS, "Successfully ensured secret under name '%s'", secretName)

	return nil
}

func certConfigFromPod(pod *corev1.Pod) (secretstypes.CertConfig, error) {
	certTypeStr := metadata.GetAnnotationValue(pod.Annotations, metadata.CertTypeAnnotation)
	certTypeStr, _ = lo.Coalesce(certTypeStr, "pem")
	certType, err := secretstypes.StrToCertType(certTypeStr)
	if err != nil {
		return secretstypes.CertConfig{}, errors.Wrap(err)
	}
	certConfig := secretstypes.CertConfig{CertType: certType}
	switch certType {
	case secretstypes.PEMCertType:
		certConfig.PEMConfig = secretstypes.NewPEMConfig(
			metadata.GetAnnotationValue(pod.Annotations, metadata.CertFileNameAnnotation),
			metadata.GetAnnotationValue(pod.Annotations, metadata.CAFileNameAnnotation),
			metadata.GetAnnotationValue(pod.Annotations, metadata.KeyFileNameAnnotation),
		)
	case secretstypes.JKSCertType:
		certConfig.JKSConfig = secretstypes.NewJKSConfig(
			metadata.GetAnnotationValue(pod.Annotations, metadata.KeyStoreFileNameAnnotation),
			metadata.GetAnnotationValue(pod.Annotations, metadata.TrustStoreFileNameAnnotation),
			metadata.GetAnnotationValue(pod.Annotations, metadata.JKSPasswordAnnotation),
		)

	}
	return certConfig, nil
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
		return ctrl.Result{}, errors.Wrap(err)
	}

	// nothing to reconcile on deletions
	if !pod.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	if !r.shouldRegisterEntryForPod(pod) {
		return ctrl.Result{}, nil
	}

	if metadata.HasDeprecatedAnnotations(pod.Annotations) {
		r.eventRecorder.Event(pod, corev1.EventTypeWarning, ReasonUsingDeprecatedAnnotations, "This pod using deprecated otterize-credentials annotations. Please check the documentation at https://docs.otterize.com/components/credentials-operator")
	}

	log.Info("updating workload entries & secrets for pod")

	// resolve pod to otterize service name
	serviceID, err := r.serviceIdResolver.ResolvePodToServiceIdentity(ctx, pod)
	if err != nil {
		r.eventRecorder.Eventf(pod, corev1.EventTypeWarning, ReasonPodOwnerResolutionFailed, "Could not resolve pod to its owner: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	log.Info("updating workload entries & secrets for pod")

	// Ensure the RegisteredServiceNameLabel is set
	result, err := r.updatePodLabel(ctx, pod, metadata.RegisteredServiceNameLabel, serviceID.Name)
	if err != nil {
		r.eventRecorder.Eventf(pod, corev1.EventTypeWarning, ReasonPodLabelUpdateFailed, "Pod label update failed: %s", err.Error())
		return result, errors.Wrap(err)
	}
	if result.Requeue {
		return result, nil
	}

	dnsNames, err := r.resolvePodToCertDNSNames(pod)
	if err != nil {
		err = errors.Errorf("error resolving pod cert DNS names, will continue with an empty DNS names list: %w", err)
		r.eventRecorder.Eventf(pod, corev1.EventTypeWarning, ReasonCertDNSResolutionFailed, "Resolving cert DNS names failed: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	ttl, err := r.resolvePodToCertTTl(pod)
	if err != nil {
		r.eventRecorder.Eventf(pod, corev1.EventTypeWarning, ReasonCertTTLError, "Getting cert TTL failed: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	// Add workload entry for pod
	entryID, err := r.workloadRegistry.RegisterK8SPod(ctx, pod.Namespace, metadata.RegisteredServiceNameLabel, serviceID.Name, ttl, dnsNames)
	if err != nil {
		log.WithError(err).Error("failed registering workload entry for pod")
		r.eventRecorder.Eventf(pod, corev1.EventTypeWarning, ReasonEntryRegistrationFailed, "Failed registering workload entry: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}
	r.eventRecorder.Eventf(pod, corev1.EventTypeNormal, ReasonPodRegistered, "Successfully registered pod under workload with entry ID '%s'", entryID)

	hashStr, err := getEntryHash(pod.Namespace, serviceID.Name, ttl, dnsNames)
	if err != nil {
		r.eventRecorder.Eventf(pod, corev1.EventTypeWarning, ReasonEntryHashCalculationFailed, "Failed calculating workload entry hash: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	shouldRestartPodOnRenewal := r.resolvePodToShouldRestartOnRenewal(pod)

	if r.shouldCreateTLSSecretForPod(pod) {
		// generate TLS secret for pod
		if err := r.ensurePodTLSSecret(ctx, pod, serviceID.Name, entryID, hashStr, shouldRestartPodOnRenewal); err != nil {
			r.eventRecorder.Eventf(pod, corev1.EventTypeWarning, ReasonEnsuringPodTLSFailed, "Failed creating TLS secret for pod: %s", err.Error())
		}
	} else {
		log.WithField("annotation.key", metadata.TLSSecretNameAnnotation).Info("skipping TLS secrets creation - annotation not found")
	}

	return ctrl.Result{}, nil
}

func getEntryHash(namespace string, serviceName string, ttl int32, dnsNames []string) (string, error) {
	entryPropertiesHashMaker := fnv.New32a()
	_, err := entryPropertiesHashMaker.Write([]byte(namespace + metadata.RegisteredServiceNameLabel + serviceName + string(ttl) + strings.Join(dnsNames, "")))
	if err != nil {
		return "", errors.Errorf("failed hashing workload entry properties %w", err)
	}
	hashStr := strconv.Itoa(int(entryPropertiesHashMaker.Sum32()))
	return hashStr, nil
}

func (r *PodReconciler) resolvePodToCertDNSNames(pod *corev1.Pod) ([]string, error) {
	dnsNamesStr := metadata.GetAnnotationValue(pod.Annotations, metadata.DNSNamesAnnotation)
	if len(dnsNamesStr) == 0 {
		return nil, nil
	}

	dnsNames := strings.Split(dnsNamesStr, ",")
	for _, name := range dnsNames {
		if !govalidator.IsDNSName(name) {
			return nil, errors.Errorf("invalid DNS name: %s", name)
		}
	}
	return dnsNames, nil
}

func (r *PodReconciler) resolvePodToCertTTl(pod *corev1.Pod) (int32, error) {
	ttlString := metadata.GetAnnotationValue(pod.Annotations, metadata.CertTTLAnnotation)
	if len(ttlString) == 0 {
		return 0, nil
	}

	ttl64, err := strconv.ParseInt(ttlString, 0, 32)
	if err != nil {
		return 0, errors.Errorf("failed converting ttl: %s str to int. %w", ttlString, err)
	}

	return int32(ttl64), nil
}

func (r *PodReconciler) shouldCreateTLSSecretForPod(pod *corev1.Pod) bool {
	return pod.Annotations != nil &&
		len(metadata.GetAnnotationValue(pod.Annotations, metadata.TLSSecretNameAnnotation)) != 0
}

func (r *PodReconciler) shouldRegisterEntryForPod(pod *corev1.Pod) bool {
	return !r.registerOnlyPodsWithSecretAnnotation || r.shouldCreateTLSSecretForPod(pod)
}

func (r *PodReconciler) resolvePodToShouldRestartOnRenewal(pod *corev1.Pod) bool {
	return metadata.AnnotationExists(pod.Annotations, metadata.ShouldRestartOnRenewalAnnotation)
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		For(&corev1.Pod{}).
		Complete(r)
}

func (r *PodReconciler) cleanupOrphanEntries(ctx context.Context) error {
	podsList := corev1.PodList{}
	if err := r.Client.List(ctx, &podsList, client.HasLabels{metadata.RegisteredServiceNameLabel}); err != nil {
		return errors.Errorf("error listing pods with service name labels: %w", err)
	}

	existingServicesByNamespace := map[string]*goset.Set[string]{}
	for _, pod := range podsList.Items {
		if _, ok := existingServicesByNamespace[pod.Namespace]; !ok {
			existingServicesByNamespace[pod.Namespace] = goset.NewSet[string]()
		}

		if r.shouldRegisterEntryForPod(&pod) {
			existingServicesByNamespace[pod.Namespace].Add(pod.Labels[metadata.RegisteredServiceNameLabel])
		}
	}

	if err := r.workloadRegistry.CleanupOrphanK8SPodEntries(ctx, metadata.RegisteredServiceNameLabel, existingServicesByNamespace); err != nil {
		return errors.Errorf("error cleaning up orphan entries: %w", err)
	}

	return nil
}

func (r *PodReconciler) MaintenanceLoop(ctx context.Context) {
	refreshSecretsTicker := time.NewTicker(refreshSecretsLoopTick)
	cleanupOrphanEntriesTicker := time.NewTicker(cleanupOrphanEntriesLoopTick)
	for {
		select {
		case <-refreshSecretsTicker.C:
			go func() {
				err := r.secretsManager.RefreshTLSSecrets(ctx)
				if err != nil {
					logrus.WithError(err).Error("failed refreshing TLS secrets")
				}
			}()
		case <-cleanupOrphanEntriesTicker.C:
			go func() {
				err := r.cleanupOrphanEntries(ctx)
				if err != nil {
					logrus.WithError(err).Error("failed cleaning up orphan entries")
				}
				logrus.Info("successfully cleaned up entries of inactive pods")
			}()
		case <-ctx.Done():
			return
		}
	}
}
