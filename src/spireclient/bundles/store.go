package bundles

import (
	"bytes"
	"context"
	"encoding/pem"
	"github.com/otterize/spifferize/src/spireclient"
	bundlev1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/bundle/v1"
)

type Store struct {
	spireClient spireclient.ServerClient
}

type EncodedTrustBundle struct {
	BundlePEM []byte
}

func NewBundlesStore(spireClient spireclient.ServerClient) *Store {
	return &Store{spireClient: spireClient}
}

func (s *Store) GetTrustBundle(ctx context.Context) (EncodedTrustBundle, error) {
	bundleClient := s.spireClient.NewBundleClient()

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
