package bundles

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	spire_client "github.com/otterize/spifferize/src/spire-client"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	bundlev1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/bundle/v1"
	svidv1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/svid/v1"
	"net/url"
)

type Manager struct {
	SpireClient spire_client.ServerClient
}

type EncodedTrustBundle struct {
	BundlePEM []byte
}

type EncodedX509SVID struct {
	SVIDPEM   []byte
	KeyPEM    []byte
	ExpiresAt int64
}

func NewBundlesManager(spireClient spire_client.ServerClient) *Manager {
	return &Manager{SpireClient: spireClient}
}

func (m *Manager) GetTrustBundle(ctx context.Context) (EncodedTrustBundle, error) {
	bundleClient := m.SpireClient.NewBundleClient()

	bundle, err := bundleClient.GetBundle(ctx, &bundlev1.GetBundleRequest{})
	if err != nil {
		return EncodedTrustBundle{}, err
	}

	bundlePEM := new(bytes.Buffer)
	for _, rootCA := range bundle.X509Authorities {
		if err := pem.Encode(bundlePEM, &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: rootCA.Asn1,
		}); err != nil {
			return EncodedTrustBundle{}, err
		}
	}

	return EncodedTrustBundle{BundlePEM: bundlePEM.Bytes()}, nil
}

func (m *Manager) GetX509SVID(ctx context.Context, spiffeID spiffeid.ID) (EncodedX509SVID, error) {
	svidClient := m.SpireClient.NewSVIDClient()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return EncodedX509SVID{}, fmt.Errorf("unable to generate key: %w", err)
	}

	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		URIs: []*url.URL{spiffeID.URL()},
		//DNSNames: []string{}, // TODO
	}, key)
	if err != nil {
		return EncodedX509SVID{}, fmt.Errorf("unable to generate CSR: %w", err)
	}

	resp, err := svidClient.MintX509SVID(ctx, &svidv1.MintX509SVIDRequest{
		Csr: csr,
		//Ttl: ttlToSeconds(c.ttl), // TODO
	})
	if err != nil {
		return EncodedX509SVID{}, fmt.Errorf("unable to mint SVID: %w", err)
	}

	svidPEM := new(bytes.Buffer)
	for _, certDER := range resp.Svid.CertChain {
		if err := pem.Encode(svidPEM, &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: certDER,
		}); err != nil {
			return EncodedX509SVID{}, fmt.Errorf("failed encoding certificate: %w", err)
		}
	}

	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return EncodedX509SVID{}, fmt.Errorf("failed marshaling private key: %w", err)
	}

	keyPEM := new(bytes.Buffer)
	if err := pem.Encode(keyPEM, &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	}); err != nil {
		return EncodedX509SVID{}, fmt.Errorf("failed encoding private key: %w", err)
	}

	return EncodedX509SVID{SVIDPEM: svidPEM.Bytes(), KeyPEM: keyPEM.Bytes(), ExpiresAt: resp.Svid.ExpiresAt}, nil
}
