package secrets

import (
	"context"
	"errors"
	"fmt"
	"github.com/otterize/spire-integration-operator/src/controllers/metadata"
	"github.com/otterize/spire-integration-operator/src/controllers/spireclient/bundles"
	"github.com/otterize/spire-integration-operator/src/controllers/spireclient/svids"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"
	"time"
)

const (
	secretExpiryDelta = 10 * time.Minute
)

type SecretType string
type CertType string

const (
	tlsSecretType = SecretType("TLS")
	JksCertType   = CertType("jks")
	PemCertType   = CertType("pem")
)

func StrToCertType(strCertType string) CertType {
	switch CertType(strings.ToLower(strCertType)) {
	case JksCertType:
		return JksCertType
	case PemCertType:
		return PemCertType
	default:
		return PemCertType
	}
}

type PemConfig struct {
	SvidFileName   string
	BundleFileName string
	KeyFileName    string
}

func NewPemConfig(svidFileName string, bundleFileName string, keyFileName string) PemConfig {
	newFileNames := PemConfig{}
	newFileNames.SvidFileName, _ = lo.Coalesce(svidFileName, "svid.pem")
	newFileNames.KeyFileName, _ = lo.Coalesce(keyFileName, "key.pem")
	newFileNames.BundleFileName, _ = lo.Coalesce(bundleFileName, "bundle.pem")
	return newFileNames
}

type JksConfig struct {
	KeyStoreFileName   string
	TrustStoreFileName string
	Password           string
}

func NewJksConfig(keystoreFileName string, truststoreFileName string, password string) JksConfig {
	newFileNames := JksConfig{}
	newFileNames.KeyStoreFileName, _ = lo.Coalesce(keystoreFileName, "keystore.jks")
	newFileNames.TrustStoreFileName, _ = lo.Coalesce(truststoreFileName, "truststore.jks")
	newFileNames.Password, _ = lo.Coalesce(password, "password")
	return newFileNames
}

type CertConfig struct {
	CertType  CertType
	JksConfig JksConfig
	PemConfig PemConfig
}

type SecretConfig struct {
	EntryID     string
	EntryHash   string
	SecretName  string
	Namespace   string
	ServiceName string
	CertConfig  CertConfig
}

func NewSecretConfig(entryID string, entryHash string, secretName string, namespace string, serviceName string, certConfig CertConfig) SecretConfig {
	return SecretConfig{
		EntryID:     entryID,
		EntryHash:   entryHash,
		SecretName:  secretName,
		Namespace:   namespace,
		ServiceName: serviceName,
		CertConfig:  certConfig,
	}
}

func SecretConfigFromExistingSecret(secret *corev1.Secret) SecretConfig {
	return SecretConfig{
		SecretName:  secret.Name,
		ServiceName: secret.Annotations[metadata.TLSSecretServiceNameAnnotation],
		EntryID:     secret.Annotations[metadata.TLSSecretEntryIDAnnotation],
		EntryHash:   secret.Annotations[metadata.TLSSecretEntryHashAnnotation],
		Namespace:   secret.Namespace,
		CertConfig: CertConfig{
			CertType: CertType(secret.Annotations[metadata.CertTypeAnnotation]),
			PemConfig: PemConfig{
				SvidFileName:   secret.Annotations[metadata.SVIDFileNameAnnotation],
				BundleFileName: secret.Annotations[metadata.BundleFileNameAnnotation],
				KeyFileName:    secret.Annotations[metadata.KeyFileNameAnnotation],
			},
			JksConfig: JksConfig{
				KeyStoreFileName:   secret.Annotations[metadata.KeyStoreFileNameAnnotation],
				TrustStoreFileName: secret.Annotations[metadata.TrustStoreFileNameAnnotation],
				Password:           secret.Annotations[metadata.JksPasswordAnnotation],
			},
		},
	}
}

type Manager interface {
	EnsureTLSSecret(ctx context.Context, config SecretConfig, owner metav1.Object) error
	RefreshTLSSecrets(ctx context.Context) error
}

type managerImpl struct {
	client.Client
	bundlesStore bundles.Store
	svidsStore   svids.Store
}

func NewSecretsManager(c client.Client, bundlesStore bundles.Store, svidsStore svids.Store) Manager {
	return &managerImpl{Client: c, bundlesStore: bundlesStore, svidsStore: svidsStore}
}

func (m *managerImpl) isRefreshNeeded(secret *corev1.Secret) bool {
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

func (m *managerImpl) getExistingSecret(ctx context.Context, namespace string, name string) (*corev1.Secret, bool, error) {
	found := corev1.Secret{}
	if err := m.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &found); err != nil && apierrors.IsNotFound(err) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}

	return &found, true, nil
}

func (m *managerImpl) updateTLSSecretConfig(ctx context.Context, config SecretConfig, secret *corev1.Secret) error {
	trustBundle, err := m.bundlesStore.GetTrustBundle(ctx)
	if err != nil {
		return err
	}

	privateKey, err := m.svidsStore.GeneratePrivateKey()
	if err != nil {
		return err
	}

	svid, err := m.svidsStore.GetX509SVID(ctx, config.EntryID, privateKey)
	if err != nil {
		return err
	}

	expiry := time.Unix(svid.ExpiresAt, 0)
	expiryStr := expiry.Format(time.RFC3339)

	secretData, err := m.generateSecretData(trustBundle, svid, config.CertConfig)
	if err != nil {
		return err
	}

	secret.Labels = map[string]string{
		metadata.SecretTypeLabel: string(tlsSecretType),
	}

	secret.Annotations = map[string]string{
		metadata.TLSSecretSVIDExpiryAnnotation:  expiryStr,
		metadata.TLSSecretServiceNameAnnotation: config.ServiceName,
		metadata.TLSSecretEntryIDAnnotation:     config.EntryID,
		metadata.TLSSecretEntryHashAnnotation:   config.EntryHash,
		metadata.SVIDFileNameAnnotation:         config.CertConfig.PemConfig.SvidFileName,
		metadata.BundleFileNameAnnotation:       config.CertConfig.PemConfig.BundleFileName,
		metadata.KeyFileNameAnnotation:          config.CertConfig.PemConfig.KeyFileName,
		metadata.KeyStoreFileNameAnnotation:     config.CertConfig.JksConfig.KeyStoreFileName,
		metadata.TrustStoreFileNameAnnotation:   config.CertConfig.JksConfig.TrustStoreFileName,
		metadata.JksPasswordAnnotation:          config.CertConfig.JksConfig.Password,
		metadata.CertTypeAnnotation:             string(config.CertConfig.CertType),
	}

	secret.Data = secretData
	return nil
}

func (m *managerImpl) generateSecretData(trustBundle bundles.EncodedTrustBundle, svid svids.EncodedX509SVID, certConfig CertConfig) (map[string][]byte, error) {
	switch certConfig.CertType {
	case JksCertType:
		trustStoreBytes, err := trustBundleToTrustStore(trustBundle, certConfig.JksConfig.Password)
		if err != nil {
			return nil, err
		}

		keyStoreBytes, err := svidToKeyStore(svid, certConfig.JksConfig.Password)
		if err != nil {
			return nil, err
		}
		return map[string][]byte{
			certConfig.JksConfig.TrustStoreFileName: trustStoreBytes,
			certConfig.JksConfig.KeyStoreFileName:   keyStoreBytes,
		}, nil
	case PemCertType:
		return map[string][]byte{
			certConfig.PemConfig.BundleFileName: trustBundle.BundlePEM,
			certConfig.PemConfig.KeyFileName:    svid.KeyPEM,
			certConfig.PemConfig.SvidFileName:   svid.SVIDPEM,
		}, nil
	default:
		return nil, fmt.Errorf("failed generating secret data. unsupported cert type %s", certConfig.CertType)
	}
}

func (m *managerImpl) EnsureTLSSecret(ctx context.Context, config SecretConfig, owner metav1.Object) error {
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
		if err := m.updateTLSSecretConfig(ctx, config, secret); err != nil {
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

func (m *managerImpl) refreshTLSSecret(ctx context.Context, secret *corev1.Secret) error {
	log := logrus.WithFields(logrus.Fields{"secret.namespace": secret.Namespace, "secret.name": secret.Name})
	_, ok := secret.Annotations[metadata.TLSSecretServiceNameAnnotation]
	if !ok {
		return errors.New("service name annotation is missing")
	}

	_, ok = secret.Annotations[metadata.TLSSecretEntryIDAnnotation]

	if !ok {
		return errors.New("entry ID annotation is missing")
	}

	if err := m.updateTLSSecretConfig(ctx, SecretConfigFromExistingSecret(secret), secret); err != nil {
		return err
	}

	log.Info("Updating existing secret")
	return m.Update(ctx, secret)
}

func (m *managerImpl) RefreshTLSSecrets(ctx context.Context) error {
	logrus.Info("refreshing TLS secrets")
	secrets := corev1.SecretList{}
	if err := m.List(ctx, &secrets, &client.MatchingLabels{metadata.SecretTypeLabel: string(tlsSecretType)}); err != nil {
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

func (m *managerImpl) isUpdateNeeded(existingSecretConfig SecretConfig, newSecretConfig SecretConfig) bool {
	log := logrus.WithFields(logrus.Fields{"secret.namespace": existingSecretConfig.Namespace, "secret.name": existingSecretConfig.SecretName})
	needsUpdate := existingSecretConfig != newSecretConfig
	log.Infof("needs update: %v", needsUpdate)

	return needsUpdate
}
