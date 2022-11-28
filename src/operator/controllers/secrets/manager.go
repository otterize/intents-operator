package secrets

import (
	"context"
	"errors"
	"fmt"
	"github.com/otterize/spire-integration-operator/src/controllers/metadata"
	"github.com/otterize/spire-integration-operator/src/controllers/secrets/types"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"time"
)

const (
	secretExpiryDelta = 10 * time.Minute
)

func SecretConfigFromExistingSecret(secret *corev1.Secret) secretstypes.SecretConfig {
	return secretstypes.SecretConfig{
		SecretName:  secret.Name,
		ServiceName: secret.Annotations[metadata.TLSSecretRegisteredServiceNameAnnotation],
		EntryID:     secret.Annotations[metadata.TLSSecretEntryIDAnnotation],
		EntryHash:   secret.Annotations[metadata.TLSSecretEntryHashAnnotation],
		Namespace:   secret.Namespace,
		CertConfig: secretstypes.CertConfig{
			CertType: secretstypes.CertType(secret.Annotations[metadata.CertTypeAnnotation]),
			PEMConfig: secretstypes.PEMConfig{
				SVIDFileName:   secret.Annotations[metadata.SVIDFileNameAnnotation],
				BundleFileName: secret.Annotations[metadata.BundleFileNameAnnotation],
				KeyFileName:    secret.Annotations[metadata.KeyFileNameAnnotation],
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
	certificateDataGenerator secretstypes.CertificateDataGenerator
}

func NewSecretManager(c client.Client, tlsSecretUpdater secretstypes.CertificateDataGenerator) *KubernetesSecretsManager {
	return &KubernetesSecretsManager{Client: c, certificateDataGenerator: tlsSecretUpdater}
}

func (m *KubernetesSecretsManager) isRefreshNeeded(secret *corev1.Secret) bool {
	log := logrus.WithFields(logrus.Fields{"secret.namespace": secret.Namespace, "secret.name": secret.Name})
	expiryBaseline := time.Now().Add(secretExpiryDelta)
	expiryStr, ok := secret.Annotations[metadata.TLSSecretSVIDExpiryAnnotation]
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
				certConfig.PEMConfig.BundleFileName: pemCert.Bundle,
				certConfig.PEMConfig.KeyFileName:    pemCert.Key,
				certConfig.PEMConfig.SVIDFileName:   pemCert.SVID,
			},
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
		metadata.TLSSecretSVIDExpiryAnnotation:            certificateData.ExpiryStr,
		metadata.TLSSecretRegisteredServiceNameAnnotation: config.ServiceName,
		metadata.TLSSecretEntryIDAnnotation:               config.EntryID,
		metadata.TLSSecretEntryHashAnnotation:             config.EntryHash,
		metadata.SVIDFileNameAnnotation:                   config.CertConfig.PEMConfig.SVIDFileName,
		metadata.BundleFileNameAnnotation:                 config.CertConfig.PEMConfig.BundleFileName,
		metadata.KeyFileNameAnnotation:                    config.CertConfig.PEMConfig.KeyFileName,
		metadata.KeyStoreFileNameAnnotation:               config.CertConfig.JKSConfig.KeyStoreFileName,
		metadata.TrustStoreFileNameAnnotation:             config.CertConfig.JKSConfig.TrustStoreFileName,
		metadata.JKSPasswordAnnotation:                    config.CertConfig.JKSConfig.Password,
		metadata.CertTypeAnnotation:                       string(config.CertConfig.CertType),
	}

	secret.Data = certificateData.Files
	return nil
}

func (m *KubernetesSecretsManager) EnsureTLSSecret(ctx context.Context, config secretstypes.SecretConfig, owner metav1.Object) error {
	log := logrus.WithFields(logrus.Fields{"secret.namespace": config.Namespace, "secret.name": config.SecretName})

	existingSecret, isExistingSecret, err := m.getExistingSecret(ctx, config.Namespace, config.SecretName)
	if err != nil {
		log.WithError(err).Error("failed querying for secret")
		return err
	}

	var secret *corev1.Secret
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
	}

	if owner != nil {
		if err := controllerutil.SetOwnerReference(owner, secret, m.Scheme()); err != nil {
			log.WithError(err).Error("failed setting pod as owner reference")
			return err
		}
	}

	if isExistingSecret {
		log.Info("Updating existing secret")
		if err := m.Update(ctx, secret); err != nil {
			logrus.WithError(err).Error("failed updating existing secret")
			return err
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
