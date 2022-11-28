package spirecertgen

import (
	"context"
	"github.com/otterize/spire-integration-operator/src/controllers/secrets/types"
	"github.com/otterize/spire-integration-operator/src/controllers/spireclient/bundles"
	"github.com/otterize/spire-integration-operator/src/controllers/spireclient/svids"
	"time"
)

type SpireCertificateDataGenerator struct {
	bundlesStore bundles.Store
	svidsStore   svids.Store
}

type spireCert struct {
	TrustBundle bundles.EncodedTrustBundle
	SVID        svids.EncodedX509SVID
	Expiry      string
}

func NewSpireCertificateDataGenerator(bundlesStore bundles.Store, svidsStore svids.Store) *SpireCertificateDataGenerator {
	return &SpireCertificateDataGenerator{bundlesStore: bundlesStore, svidsStore: svidsStore}
}

func (m *SpireCertificateDataGenerator) getSpireCert(ctx context.Context, entryID string) (spireCert, error) {
	trustBundle, err := m.bundlesStore.GetTrustBundle(ctx)
	if err != nil {
		return spireCert{}, err
	}

	privateKey, err := m.svidsStore.GeneratePrivateKey()

	if err != nil {
		return spireCert{}, err
	}

	svid, err := m.svidsStore.GetX509SVID(ctx, entryID, privateKey)
	if err != nil {
		return spireCert{}, err
	}

	expiry := time.Unix(svid.ExpiresAt, 0)
	expiryStr := expiry.Format(time.RFC3339)

	return spireCert{TrustBundle: trustBundle, SVID: svid, Expiry: expiryStr}, nil
}

func (m *SpireCertificateDataGenerator) GenerateJKS(ctx context.Context, entryID string, password string) (secretstypes.JKSCert, error) {
	spireCertificate, err := m.getSpireCert(ctx, entryID)
	if err != nil {
		return secretstypes.JKSCert{}, err
	}

	trustStoreBytes, err := trustBundleToTrustStore(spireCertificate.TrustBundle, password)

	if err != nil {
		return secretstypes.JKSCert{}, err
	}

	keyStoreBytes, err := svidToKeyStore(spireCertificate.SVID, password)

	if err != nil {
		return secretstypes.JKSCert{}, err
	}

	return secretstypes.JKSCert{TrustStore: trustStoreBytes, KeyStore: keyStoreBytes, Expiry: spireCertificate.Expiry}, nil
}

func (m *SpireCertificateDataGenerator) GeneratePEM(ctx context.Context, entryID string) (secretstypes.PEMCert, error) {
	spireCertificate, err := m.getSpireCert(ctx, entryID)
	if err != nil {
		return secretstypes.PEMCert{}, err
	}

	return secretstypes.PEMCert{
		SVID:   spireCertificate.SVID.SVIDPEM,
		Key:    spireCertificate.SVID.KeyPEM,
		Bundle: spireCertificate.TrustBundle.BundlePEM,
		Expiry: spireCertificate.Expiry}, nil
}
