package bundles

import (
	"bytes"
	"context"
	"encoding/pem"
	"github.com/otterize/spire-integration-operator/src/spireclient"
	bundlev1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/bundle/v1"
)

type Store interface {
	GetTrustBundle(ctx context.Context) (EncodedTrustBundle, error)
}

type storeImpl struct {
	bundleClient bundlev1.BundleClient
}

type EncodedTrustBundle struct {
	BundlePEM []byte
}

func NewBundlesStore(spireClient spireclient.ServerClient) Store {
	return &storeImpl{bundleClient: spireClient.NewBundleClient()}
}

func (s *storeImpl) GetTrustBundle(ctx context.Context) (EncodedTrustBundle, error) {
	bundle, err := s.bundleClient.GetBundle(ctx, &bundlev1.GetBundleRequest{})
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
