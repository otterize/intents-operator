package secrets

import (
	"context"
	"errors"
	"github.com/otterize/spifferize/src/spireclient/bundles"
	"github.com/otterize/spifferize/src/spireclient/svids"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	secretTypeLabel                = "spifferize/secret-type"
	tlsSecretServiceNameAnnotation = "spifferize/service-name"
	tlsSecretSPIFFEIDAnnotation    = "spifferize/spiffeid"
	svidExpiryAnnotation           = "spifferize/svid-expires-at"
	secretExpiryDelta              = 10 * time.Minute
)

type SecretType string

const (
	tlsSecretType = SecretType("TLS")
)

type Manager struct {
	client.Client
	bundlesStore *bundles.Store
	svidsStore   *svids.Store
}

func NewSecretsManager(c client.Client, bundlesStore *bundles.Store, svidsStore *svids.Store) *Manager {
	return &Manager{Client: c, bundlesStore: bundlesStore, svidsStore: svidsStore}
}

func (m *Manager) isRefreshNeeded(secret *corev1.Secret) bool {
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

	log.Info("secret expiry is far enough, not re-creating it")
	return false
}

func (m *Manager) getExistingSecret(ctx context.Context, namespace string, name string) (*corev1.Secret, bool, error) {
	found := corev1.Secret{}
	if err := m.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &found); err != nil && apierrors.IsNotFound(err) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}

	return &found, true, nil
}

func (m *Manager) createTLSSecret(ctx context.Context, namespace string, secretName string, serviceName string, spiffeID spiffeid.ID) (*corev1.Secret, error) {
	trustBundle, err := m.bundlesStore.GetTrustBundle(ctx)
	if err != nil {
		return nil, err
	}

	privateKey, err := m.svidsStore.GeneratePrivateKey()
	if err != nil {
		return nil, err
	}

	svid, err := m.svidsStore.GetX509SVID(ctx, spiffeID, privateKey)
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
				tlsSecretSPIFFEIDAnnotation:    spiffeID.String(),
			},
		},
		Data: map[string][]byte{
			"bundle.pem": trustBundle.BundlePEM,
			"key.pem":    svid.KeyPEM,
			"svid.pem":   svid.SVIDPEM,
		},
	}

	return &secret, nil
}

func (m *Manager) EnsureTLSSecret(ctx context.Context, namespace string, secretName string, serviceName string, spiffeID spiffeid.ID) error {
	log := logrus.WithFields(logrus.Fields{"secret.namespace": namespace, "secret.name": secretName})

	existingSecret, isExistingSecret, err := m.getExistingSecret(ctx, namespace, secretName)
	if err != nil {
		log.WithError(err).Error("failed querying for secret")
		return err
	}

	if isExistingSecret && !m.isRefreshNeeded(existingSecret) {
		log.Info("secret already exists and does not require refreshing")
		return nil
	}

	secret, err := m.createTLSSecret(ctx, namespace, secretName, serviceName, spiffeID)
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

func (m *Manager) refreshTLSSecret(ctx context.Context, secret *corev1.Secret) error {
	log := logrus.WithFields(logrus.Fields{"secret.namespace": secret.Namespace, "secret.name": secret.Name})
	serviceName, ok := secret.Annotations[tlsSecretServiceNameAnnotation]
	if !ok {
		return errors.New("service name annotation is missing")
	}

	spiffeIDStr, ok := secret.Annotations[tlsSecretSPIFFEIDAnnotation]
	if !ok {
		return errors.New("spiffe ID annotation is missing")
	}

	spiffeID, err := spiffeid.FromString(spiffeIDStr)
	if err != nil {
		log.WithField("spiffeid", spiffeID).WithError(err).Error("failed parsing spiffeid")
		return err
	}

	newSecret, err := m.createTLSSecret(ctx, secret.Namespace, secret.Name, serviceName, spiffeID)
	if err != nil {
		return err
	}

	log.Info("Updating existing secret")
	return m.Update(ctx, newSecret)
}

func (m *Manager) RefreshTLSSecrets(ctx context.Context) error {
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
