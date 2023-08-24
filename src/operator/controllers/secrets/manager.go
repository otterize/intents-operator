package secrets

import (
	"context"
	"errors"
	"fmt"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/otterize/credentials-operator/src/controllers/secrets/types"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"time"
)

const (
	secretExpiryDelta = 10 * time.Minute
	CertRenewReason   = "CertificateRenewed"
)

func SecretConfigFromExistingSecret(secret *corev1.Secret) secretstypes.SecretConfig {
	_, shouldRestartOnRenewalBool := secret.Annotations[metadata.ShouldRestartOnRenewalAnnotation]
	return secretstypes.SecretConfig{
		SecretName:                secret.Name,
		ServiceName:               secret.Annotations[metadata.TLSSecretRegisteredServiceNameAnnotation],
		EntryID:                   secret.Annotations[metadata.TLSSecretEntryIDAnnotation],
		EntryHash:                 secret.Annotations[metadata.TLSSecretEntryHashAnnotation],
		Namespace:                 secret.Namespace,
		ShouldRestartPodOnRenewal: shouldRestartOnRenewalBool,
		CertConfig: secretstypes.CertConfig{
			CertType: secretstypes.CertType(secret.Annotations[metadata.CertTypeAnnotation]),
			PEMConfig: secretstypes.PEMConfig{
				CertFileName: secret.Annotations[metadata.CertFileNameAnnotation],
				CAFileName:   secret.Annotations[metadata.CAFileNameAnnotation],
				KeyFileName:  secret.Annotations[metadata.KeyFileNameAnnotation],
			},
			JKSConfig: secretstypes.JKSConfig{
				KeyStoreFileName:   secret.Annotations[metadata.KeyStoreFileNameAnnotation],
				TrustStoreFileName: secret.Annotations[metadata.TrustStoreFileNameAnnotation],
				Password:           secret.Annotations[metadata.JKSPasswordAnnotation],
			},
		},
	}
}

type KubernetesSecretsManager struct {
	client.Client
	eventRecorder            record.EventRecorder
	certificateDataGenerator secretstypes.CertificateDataGenerator
	serviceIdResolver        secretstypes.ServiceIdResolver
}

func NewSecretManager(
	c client.Client,
	tlsSecretUpdater secretstypes.CertificateDataGenerator,
	serviceIdResolver secretstypes.ServiceIdResolver,
	eventRecorder record.EventRecorder) *KubernetesSecretsManager {
	return &KubernetesSecretsManager{Client: c, certificateDataGenerator: tlsSecretUpdater, serviceIdResolver: serviceIdResolver, eventRecorder: eventRecorder}
}

func (m *KubernetesSecretsManager) isRefreshNeeded(secret *corev1.Secret) bool {
	log := logrus.WithFields(logrus.Fields{"secret.namespace": secret.Namespace, "secret.name": secret.Name})
	expiryBaseline := time.Now().Add(secretExpiryDelta)
	expiryStr, ok := secret.Annotations[metadata.TLSSecretExpiryAnnotation]
	if !ok {
		log.Warn("secret missing expiry annotation, will re-create it")
		return true
	}

	log = log.WithField("expiry", expiryStr)

	expiry, err := time.Parse(time.RFC3339, expiryStr)
	if err != nil {
		log.WithError(err).Error("failed parsing secret expiry time, will re-create it")
		return true
	}

	if expiry.Before(expiryBaseline) {
		log.Info("secret expiry is near, will re-create it")
		return true
	}

	log.Debug("secret expiry is far enough")
	return false
}

func (m *KubernetesSecretsManager) getExistingSecret(ctx context.Context, namespace string, name string) (*corev1.Secret, bool, error) {
	found := corev1.Secret{}
	if err := m.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &found); err != nil && apierrors.IsNotFound(err) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}

	return &found, true, nil
}

func (m *KubernetesSecretsManager) getCertificateData(ctx context.Context, entryID string, certConfig secretstypes.CertConfig) (secretstypes.CertificateData, error) {
	switch certConfig.CertType {
	case secretstypes.JKSCertType:
		jksCert, err := m.certificateDataGenerator.GenerateJKS(ctx, entryID, certConfig.JKSConfig.Password)
		if err != nil {
			return secretstypes.CertificateData{}, err
		}
		return secretstypes.CertificateData{
			Files: map[string][]byte{
				certConfig.JKSConfig.KeyStoreFileName:   jksCert.KeyStore,
				certConfig.JKSConfig.TrustStoreFileName: jksCert.TrustStore,
			},
			ExpiryStr: jksCert.Expiry,
		}, nil
	case secretstypes.PEMCertType:
		pemCert, err := m.certificateDataGenerator.GeneratePEM(ctx, entryID)
		if err != nil {
			return secretstypes.CertificateData{}, err
		}
		return secretstypes.CertificateData{
			Files: map[string][]byte{
				certConfig.PEMConfig.CAFileName:   pemCert.CA,
				certConfig.PEMConfig.KeyFileName:  pemCert.Key,
				certConfig.PEMConfig.CertFileName: pemCert.Certificate,
			},
			ExpiryStr: pemCert.Expiry,
		}, nil
	default:
		return secretstypes.CertificateData{}, fmt.Errorf("failed generating secret data. unsupported cert type %s", certConfig.CertType)
	}
}

func (m *KubernetesSecretsManager) updateTLSSecret(ctx context.Context, config secretstypes.SecretConfig, secret *corev1.Secret) error {
	certificateData, err := m.getCertificateData(ctx, config.EntryID, config.CertConfig)
	if err != nil {
		return err
	}

	secret.Labels = map[string]string{
		metadata.SecretTypeLabel: string(secretstypes.TlsSecretType),
	}

	secret.Annotations = map[string]string{
		metadata.TLSSecretExpiryAnnotation:                certificateData.ExpiryStr,
		metadata.TLSSecretRegisteredServiceNameAnnotation: config.ServiceName,
		metadata.TLSSecretEntryIDAnnotation:               config.EntryID,
		metadata.TLSSecretEntryHashAnnotation:             config.EntryHash,
		metadata.CertFileNameAnnotation:                   config.CertConfig.PEMConfig.CertFileName,
		metadata.CAFileNameAnnotation:                     config.CertConfig.PEMConfig.CAFileName,
		metadata.KeyFileNameAnnotation:                    config.CertConfig.PEMConfig.KeyFileName,
		metadata.KeyStoreFileNameAnnotation:               config.CertConfig.JKSConfig.KeyStoreFileName,
		metadata.TrustStoreFileNameAnnotation:             config.CertConfig.JKSConfig.TrustStoreFileName,
		metadata.JKSPasswordAnnotation:                    config.CertConfig.JKSConfig.Password,
		metadata.CertTypeAnnotation:                       string(config.CertConfig.CertType),
	}
	if config.ShouldRestartPodOnRenewal {
		// it only has to exist, we don't check the value
		secret.Annotations[metadata.ShouldRestartOnRenewalAnnotation] = ""
	}

	secret.Data = certificateData.Files
	return nil
}

func (m *KubernetesSecretsManager) EnsureTLSSecret(ctx context.Context, config secretstypes.SecretConfig, pod *corev1.Pod) error {
	log := logrus.WithFields(logrus.Fields{"secret.namespace": config.Namespace, "secret.name": config.SecretName})

	existingSecret, isExistingSecret, err := m.getExistingSecret(ctx, config.Namespace, config.SecretName)
	if err != nil {
		log.WithError(err).Error("failed querying for secret")
		return err
	}

	var secret *corev1.Secret
	shouldUpdate := false

	if isExistingSecret {
		secret = existingSecret
	} else {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.SecretName,
				Namespace: config.Namespace,
			},
		}
	}

	if !isExistingSecret ||
		m.isRefreshNeeded(secret) ||
		m.isUpdateNeeded(SecretConfigFromExistingSecret(secret), config) {
		if err := m.updateTLSSecret(ctx, config, secret); err != nil {
			log.WithError(err).Error("failed updating TLS secret config")
			return err
		}
		shouldUpdate = true
	}

	ownerCount := len(secret.OwnerReferences)
	if pod != nil {
		podOwner, err := m.serviceIdResolver.GetOwnerObject(ctx, pod)
		if err != nil {
			return err
		}
		if err := controllerutil.SetOwnerReference(podOwner, secret, m.Scheme()); err != nil {
			log.WithError(err).Error("failed setting pod as owner reference")
			return err
		}
		shouldUpdate = shouldUpdate || len(secret.OwnerReferences) != ownerCount
	}

	if isExistingSecret {
		if shouldUpdate {
			log.Info("Updating existing secret")
			if err := m.Update(ctx, secret); err != nil {
				logrus.WithError(err).Error("failed updating existing secret")
				return err
			}
		}
	} else {
		log.Info("Creating a new secret")
		if err := m.Create(ctx, secret); err != nil {
			logrus.WithError(err).Error("failed creating new secret")
			return err
		}
	}

	return nil
}

func (m *KubernetesSecretsManager) refreshTLSSecret(ctx context.Context, secret *corev1.Secret) error {
	log := logrus.WithFields(logrus.Fields{"secret.namespace": secret.Namespace, "secret.name": secret.Name})
	_, ok := secret.Annotations[metadata.TLSSecretRegisteredServiceNameAnnotation]
	if !ok {
		return errors.New("service name annotation is missing")
	}

	_, ok = secret.Annotations[metadata.TLSSecretEntryIDAnnotation]

	if !ok {
		return errors.New("entry ID annotation is missing")
	}

	if err := m.updateTLSSecret(ctx, SecretConfigFromExistingSecret(secret), secret); err != nil {
		return err
	}

	log.Info("Updating existing secret")
	return m.Update(ctx, secret)
}

func (m *KubernetesSecretsManager) RefreshTLSSecrets(ctx context.Context) error {
	logrus.Info("refreshing TLS secrets")
	secrets := corev1.SecretList{}
	if err := m.List(ctx, &secrets, &client.MatchingLabels{metadata.SecretTypeLabel: string(secretstypes.TlsSecretType)}); err != nil {
		logrus.WithError(err).Error("failed listing TLS secrets")
		return err
	}

	secretsNeedingRefresh := lo.Filter(
		secrets.Items,
		func(secret corev1.Secret, _ int) bool { return m.isRefreshNeeded(&secret) },
	)

	log := logrus.WithFields(logrus.Fields{"secrets_count": len(secrets.Items), "refresh_count": len(secretsNeedingRefresh)})
	log.Info("finished listing secrets")

	for _, secret := range secretsNeedingRefresh {
		log := logrus.WithFields(logrus.Fields{"secret.namespace": secret.Namespace, "secret.name": secret.Name})
		if err := m.refreshTLSSecret(ctx, &secret); err != nil {
			log.WithError(err).Error("failed refreshing TLS secret")
		}
		if err := m.handlePodRestarts(ctx, &secret); err != nil {
			log.WithError(err).Error("failed restarting pods after secret refresh")
		}
	}

	log.Info("finished refreshing secrets")
	return nil
}

func (m *KubernetesSecretsManager) isUpdateNeeded(existingSecretConfig secretstypes.SecretConfig, newSecretConfig secretstypes.SecretConfig) bool {
	log := logrus.WithFields(logrus.Fields{"secret.namespace": existingSecretConfig.Namespace, "secret.name": existingSecretConfig.SecretName})
	needsUpdate := existingSecretConfig != newSecretConfig
	log.Infof("needs update: %v", needsUpdate)

	return needsUpdate
}

func (m *KubernetesSecretsManager) handlePodRestarts(ctx context.Context, secret *corev1.Secret) error {
	podList := corev1.PodList{}
	labelSelector, err := labels.Parse(fmt.Sprintf("%s=%s", metadata.RegisteredServiceNameLabel, secret.Annotations[metadata.RegisteredServiceNameLabel]))
	if err != nil {
		return err
	}

	err = m.List(ctx, &podList, &client.ListOptions{
		LabelSelector: labelSelector,
		Namespace:     secret.Namespace,
	})
	if err != nil {
		return err
	}
	// create unique owner list
	owners := make(map[secretstypes.PodOwnerIdentifier]client.Object)
	for _, pod := range podList.Items {
		if ok := metadata.AnnotationExists(pod.Annotations, metadata.ShouldRestartOnRenewalAnnotation); ok {
			owner, err := m.serviceIdResolver.GetOwnerObject(ctx, &pod)
			if err != nil {
				return err
			}
			owners[secretstypes.PodOwnerIdentifier{Name: owner.GetName(), GroupVersionKind: owner.GetObjectKind().GroupVersionKind()}] = owner
		}
	}
	for _, owner := range owners {
		logrus.Infof("Restarting pods for owner %s of type %s after certificate renewal",
			owner.GetName(), owner.GetObjectKind().GroupVersionKind().Kind)
		err = m.TriggerPodRestarts(ctx, owner, secret)
		if err != nil {
			return err
		}
	}

	return nil
}

// TriggerPodRestarts edits the pod owner's template spec with an annotation about the secret's expiry date
// If the secret is refreshed, its expiry will be updated in the pod owner's spec which will trigger the pods to restart
func (m *KubernetesSecretsManager) TriggerPodRestarts(ctx context.Context, owner client.Object, secret *corev1.Secret) error {
	kind := owner.GetObjectKind().GroupVersionKind().Kind
	switch kind {
	case "Deployment":
		deployment := v1.Deployment{}
		if err := m.Get(ctx, types.NamespacedName{Namespace: secret.Namespace, Name: owner.GetName()}, &deployment); err != nil {
			return err
		}
		deployment.Spec.Template = m.updatePodTemplateSpec(deployment.Spec.Template)
		if err := m.Update(ctx, &deployment); err != nil {
			return err
		}
		m.eventRecorder.Eventf(&deployment, corev1.EventTypeNormal, CertRenewReason, "Successfully restarted Deployment after secret '%s' renewal", secret.Name)
	case "ReplicaSet":
		replicaSet := v1.ReplicaSet{}
		if err := m.Get(ctx, types.NamespacedName{Namespace: secret.Namespace, Name: owner.GetName()}, &replicaSet); err != nil {
			return err
		}
		replicaSet.Spec.Template = m.updatePodTemplateSpec(replicaSet.Spec.Template)
		if err := m.Update(ctx, &replicaSet); err != nil {
			return err
		}
		m.eventRecorder.Eventf(&replicaSet, corev1.EventTypeNormal, CertRenewReason, "Successfully restarted ReplicaSet after secret '%s' renewal", secret.Name)
	case "StatefulSet":
		statefulSet := v1.StatefulSet{}
		if err := m.Get(ctx, types.NamespacedName{Namespace: secret.Namespace, Name: owner.GetName()}, &statefulSet); err != nil {
			return err
		}
		statefulSet.Spec.Template = m.updatePodTemplateSpec(statefulSet.Spec.Template)
		if err := m.Update(ctx, &statefulSet); err != nil {
			return err
		}
		m.eventRecorder.Eventf(&statefulSet, corev1.EventTypeNormal, CertRenewReason, "Successfully restarted StatefulSet after secret '%s' renewal", secret.Name)

	case "DaemonSet":
		daemonSet := v1.DaemonSet{}
		if err := m.Get(ctx, types.NamespacedName{Namespace: secret.Namespace, Name: owner.GetName()}, &daemonSet); err != nil {
			return err
		}
		daemonSet.Spec.Template = m.updatePodTemplateSpec(daemonSet.Spec.Template)
		if err := m.Update(ctx, &daemonSet); err != nil {
			return err
		}
		m.eventRecorder.Eventf(&daemonSet, corev1.EventTypeNormal, CertRenewReason, "Successfully restarted DaemonSet after secret '%s' renewal", secret.Name)

	default:
		return fmt.Errorf("unsupported owner type: %s", kind)
	}
	return nil
}

func (m *KubernetesSecretsManager) updatePodTemplateSpec(podTemplateSpec corev1.PodTemplateSpec) corev1.PodTemplateSpec {
	if podTemplateSpec.Annotations == nil {
		podTemplateSpec.Annotations = map[string]string{}
	}
	podTemplateSpec.Annotations[metadata.TLSRestartTimeAfterRenewal] = time.Now().String()
	return podTemplateSpec
}
