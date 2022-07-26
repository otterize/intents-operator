package svids

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/otterize/spifferize/src/spireclient"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	entryv1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/entry/v1"
	svidv1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/svid/v1"
	"net/url"
)

type Store interface {
	GeneratePrivateKey() (crypto.PrivateKey, error)
	GetX509SVID(ctx context.Context, entryID string, privateKey crypto.PrivateKey) (EncodedX509SVID, error)
}

type storeImpl struct {
	svidClient  svidv1.SVIDClient
	entryClient entryv1.EntryClient
}

type EncodedX509SVID struct {
	SVIDPEM   []byte
	KeyPEM    []byte
	ExpiresAt int64
}

func NewSVIDsStore(spireClient spireclient.ServerClient) Store {
	return &storeImpl{svidClient: spireClient.NewSVIDClient(), entryClient: spireClient.NewEntryClient()}
}

func (s *storeImpl) GeneratePrivateKey() (crypto.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

func (s *storeImpl) GetX509SVID(ctx context.Context, entryID string, privateKey crypto.PrivateKey) (EncodedX509SVID, error) {
	entry, err := s.entryClient.GetEntry(ctx, &entryv1.GetEntryRequest{Id: entryID})
	if err != nil {
		return EncodedX509SVID{}, fmt.Errorf("failed querying for entry: %w", err)
	}

	trustDomain, err := spiffeid.TrustDomainFromString(entry.SpiffeId.GetTrustDomain())
	if err != nil {
		return EncodedX509SVID{}, fmt.Errorf("failed parsing trust domain: %w", err)
	}
	spiffeID, err := spiffeid.FromPath(trustDomain, entry.SpiffeId.Path)
	if err != nil {
		return EncodedX509SVID{}, fmt.Errorf("failed parsing spiffID: %w", err)
	}

	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		DNSNames: entry.DnsNames,
		URIs:     []*url.URL{spiffeID.URL()},
	}, privateKey)

	if err != nil {
		return EncodedX509SVID{}, fmt.Errorf("unable to generate CSR: %w", err)
	}

	resp, err := s.svidClient.MintX509SVID(ctx, &svidv1.MintX509SVIDRequest{
		Csr: csr,
		Ttl: entry.Ttl,
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

	keyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
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
