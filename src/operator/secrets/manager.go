package secrets

import (
	"context"
	"errors"
	"github.com/otterize/spire-integration-operator/src/spireclient/bundles"
	"github.com/otterize/spire-integration-operator/src/spireclient/svids"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	secretTypeLabel                = "spire-integration-operator/secret-type"
	tlsSecretServiceNameAnnotation = "spire-integration-operator/service-name"
	tlsSecretEntryIDAnnotation     = "spire-integration-operator/entry-id"
	svidExpiryAnnotation           = "spire-integration-operator/svid-expires-at"
	SVIDFileNameAnnotation         = "otterize/svid-file-name"
	BundleFileNameAnnotation       = "otterize/bundle-file-name"
	KeyFileNameAnnotation          = "otterize/key-file-name"
	entryHashAnnotation            = "otterize/entry-hash"
	secretExpiryDelta              = 10 * time.Minute
)

type SecretType string

const (
	tlsSecretType = SecretType("TLS")
)

type SecretFileNames struct {
	SvidFileName   string
	BundleFileName string
	KeyFileName    string
}

func NewSecretFileNames(svidFileName string, bundleFileName string, keyFileName string) SecretFileNames {
	newFileNames := SecretFileNames{}
	newFileNames.SvidFileName, _ = lo.Coalesce(svidFileName, "svid.pem")
	newFileNames.KeyFileName, _ = lo.Coalesce(keyFileName, "key.pem")
	newFileNames.BundleFileName, _ = lo.Coalesce(bundleFileName, "bundle.pem")
	return newFileNames
}

type Manager interface {
	EnsureTLSSecret(ctx context.Context, namespace string, secretName string, serviceName string, entryID string, entryHash string, secretFileNames SecretFileNames) error
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
	expiryStr, ok := secret.Annotations[svidExpiryAnnotation]
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

	log.Info("secret expiry is far enough")
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

func (m *managerImpl) createTLSSecret(ctx context.Context, namespace string, secretName string, serviceName string, entryID string, entryHash string, secretFileNames SecretFileNames) (*corev1.Secret, error) {
	trustBundle, err := m.bundlesStore.GetTrustBundle(ctx)
	if err != nil {
		return nil, err
	}

	privateKey, err := m.svidsStore.GeneratePrivateKey()
	if err != nil {
		return nil, err
	}

	svid, err := m.svidsStore.GetX509SVID(ctx, entryID, privateKey)
	if err != nil {
		return nil, err
	}

	expiry := time.Unix(svid.ExpiresAt, 0)
	expiryStr := expiry.Format(time.RFC3339)

	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels: map[string]string{
				secretTypeLabel: string(tlsSecretType),
			},
			Annotations: map[string]string{
				svidExpiryAnnotation:           expiryStr,
				tlsSecretServiceNameAnnotation: serviceName,
				tlsSecretEntryIDAnnotation:     entryID,
				SVIDFileNameAnnotation:         secretFileNames.SvidFileName,
				BundleFileNameAnnotation:       secretFileNames.BundleFileName,
				KeyFileNameAnnotation:          secretFileNames.KeyFileName,
				entryHashAnnotation:            entryHash,
			},
		},
		Data: map[string][]byte{
			secretFileNames.BundleFileName: trustBundle.BundlePEM,
			secretFileNames.KeyFileName:    svid.KeyPEM,
			secretFileNames.SvidFileName:   svid.SVIDPEM,
		},
	}

	return &secret, nil
}

func (m *managerImpl) EnsureTLSSecret(ctx context.Context, namespace string, secretName string, serviceName string, entryID string, entryHash string, secretFileNames SecretFileNames) error {
	log := logrus.WithFields(logrus.Fields{"secret.namespace": namespace, "secret.name": secretName})

	existingSecret, isExistingSecret, err := m.getExistingSecret(ctx, namespace, secretName)
	if err != nil {
		log.WithError(err).Error("failed querying for secret")
		return err
	}

	if isExistingSecret && !m.isRefreshNeeded(existingSecret) && !m.isUpdateNeeded(existingSecret, serviceName, entryID, entryHash, secretFileNames) {
		log.Info("secret already exists and does not require refreshing or updating")
		return nil
	}

	secret, err := m.createTLSSecret(ctx, namespace, secretName, serviceName, entryID, entryHash, secretFileNames)
	if err != nil {
		log.WithError(err).Error("failed creating TLS secret")
		return err
	}

	if isExistingSecret {
		log.Info("Updating existing secret")
		return m.Update(ctx, secret)
	} else {
		log.Info("Creating a new secret")
		return m.Create(ctx, secret)
	}
}

func (m *managerImpl) refreshTLSSecret(ctx context.Context, secret *corev1.Secret) error {
	log := logrus.WithFields(logrus.Fields{"secret.namespace": secret.Namespace, "secret.name": secret.Name})
	serviceName, ok := secret.Annotations[tlsSecretServiceNameAnnotation]
	if !ok {
		return errors.New("service name annotation is missing")
	}

	entryId, ok := secret.Annotations[tlsSecretEntryIDAnnotation]

	if !ok {
		return errors.New("entry ID annotation is missing")
	}

	secretFileNamesFromAnnotations := NewSecretFileNames(secret.Annotations[SVIDFileNameAnnotation], secret.Annotations[BundleFileNameAnnotation], secret.Annotations[KeyFileNameAnnotation])

	newSecret, err := m.createTLSSecret(ctx, secret.Namespace, secret.Name, serviceName, entryId, secret.Annotations[entryHashAnnotation], secretFileNamesFromAnnotations)
	if err != nil {
		return err
	}

	log.Info("Updating existing secret")
	return m.Update(ctx, newSecret)
}

func (m *managerImpl) RefreshTLSSecrets(ctx context.Context) error {
	logrus.Info("refreshing TLS secrets")
	secrets := corev1.SecretList{}
	if err := m.List(ctx, &secrets, &client.MatchingLabels{secretTypeLabel: string(tlsSecretType)}); err != nil {
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

func (m *managerImpl) isUpdateNeeded(existingSecret *corev1.Secret, serviceName string, entryID string, entryHash string, secretFileNames SecretFileNames) bool {
	log := logrus.WithFields(logrus.Fields{"secret.namespace": existingSecret.Namespace, "secret.name": existingSecret.Name})
	serviceNameCheck := existingSecret.Annotations[tlsSecretServiceNameAnnotation] == serviceName
	EntryIDCheck := existingSecret.Annotations[tlsSecretEntryIDAnnotation] == entryID
	SVIDFileNameCheck := existingSecret.Annotations[SVIDFileNameAnnotation] == secretFileNames.SvidFileName
	BundleFileNameCheck := existingSecret.Annotations[BundleFileNameAnnotation] == secretFileNames.BundleFileName
	KeyFileNameCheck := existingSecret.Annotations[KeyFileNameAnnotation] == secretFileNames.KeyFileName
	HashCheck := existingSecret.Annotations[entryHashAnnotation] == entryHash

	needsUpdate := !(serviceNameCheck && EntryIDCheck && SVIDFileNameCheck && BundleFileNameCheck && KeyFileNameCheck && HashCheck)

	log.Infof("needs update: %v", needsUpdate)

	return needsUpdate
}
