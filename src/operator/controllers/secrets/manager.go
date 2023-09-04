package secrets

import (
	"context"
	"errors"
	"fmt"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/otterize/credentials-operator/src/controllers/secrets/types"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	secretExpiryDelta = 10 * time.Minute
)

var _ K8sSecretsManagerSubclass[*corev1.Secret] = &DirectSecretsManager{}

type DirectSecretsManager struct {
	K8sSecretsManagerBase[*corev1.Secret]
	certificateDataGenerator secretstypes.CertificateDataGenerator
}

func NewDirectSecretsManager(
	c client.Client,
	serviceIdResolver secretstypes.ServiceIdResolver,
	eventRecorder record.EventRecorder,
	tlsSecretUpdater secretstypes.CertificateDataGenerator) *DirectSecretsManager {
	m := &DirectSecretsManager{
		certificateDataGenerator: tlsSecretUpdater,
	}
	m.K8sSecretsManagerBase = *NewK8sSecretsManagerBase[*corev1.Secret](c, serviceIdResolver,
		eventRecorder, m)
	return m
}

func (m *DirectSecretsManager) NewSecretObject() *corev1.Secret {
	return new(corev1.Secret)
}

func (m *DirectSecretsManager) IsRefreshNeeded(secret *corev1.Secret) bool {
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

func (m *DirectSecretsManager) ExtractConfig(secret *corev1.Secret) secretstypes.SecretConfig {
	config := SecretConfigFromAnnotations(secret.Annotations)
	config.SecretName = secret.Name
	config.Namespace = secret.Namespace
	return config
}

func (m *DirectSecretsManager) getCertificateData(ctx context.Context, entryID string, certConfig secretstypes.CertConfig) (secretstypes.CertificateData, error) {
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

func (m *DirectSecretsManager) PopulateSecretObject(ctx context.Context, config secretstypes.SecretConfig, secret *corev1.Secret) error {
	certificateData, err := m.getCertificateData(ctx, config.EntryID, config.CertConfig)
	if err != nil {
		return err
	}

	secret.Annotations[metadata.TLSSecretExpiryAnnotation] = certificateData.ExpiryStr
	secret.Data = certificateData.Files
	return nil
}

func (m *DirectSecretsManager) refreshTLSSecret(ctx context.Context, secret *corev1.Secret) error {
	log := logrus.WithFields(logrus.Fields{"secret.namespace": secret.Namespace, "secret.name": secret.Name})
	_, ok := secret.Annotations[metadata.TLSSecretRegisteredServiceNameAnnotation]
	if !ok {
		return errors.New("service name annotation is missing")
	}

	_, ok = secret.Annotations[metadata.TLSSecretEntryIDAnnotation]

	if !ok {
		return errors.New("entry ID annotation is missing")
	}

	if err := m.UpdateSecretObject(ctx, m.ExtractConfig(secret), secret); err != nil {
		return err
	}

	log.Info("Updating existing secret")
	return m.Update(ctx, secret)
}

func (m *DirectSecretsManager) RefreshTLSSecrets(ctx context.Context) error {
	logrus.Info("refreshing TLS secrets")
	secrets := corev1.SecretList{}
	if err := m.List(ctx, &secrets, &client.MatchingLabels{metadata.SecretTypeLabel: string(secretstypes.TlsSecretType)}); err != nil {
		logrus.WithError(err).Error("failed listing TLS secrets")
		return err
	}

	secretsNeedingRefresh := lo.Filter(
		secrets.Items,
		func(secret corev1.Secret, _ int) bool { return m.IsRefreshNeeded(&secret) },
	)

	log := logrus.WithFields(logrus.Fields{"secrets_count": len(secrets.Items), "refresh_count": len(secretsNeedingRefresh)})
	log.Info("finished listing secrets")

	for _, secret := range secretsNeedingRefresh {
		log := logrus.WithFields(logrus.Fields{"secret.namespace": secret.Namespace, "secret.name": secret.Name})
		if err := m.refreshTLSSecret(ctx, &secret); err != nil {
			log.WithError(err).Error("failed refreshing TLS secret")
		}
		if err := m.HandlePodRestarts(ctx, &secret); err != nil {
			log.WithError(err).Error("failed restarting pods after secret refresh")
		}
	}

	log.Info("finished refreshing secrets")
	return nil
}
