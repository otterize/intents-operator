package bundles

import (
	"context"
	"crypto/x509"
	mock_bundlev1 "github.com/otterize/credentials-operator/src/mocks/bundlev1"
	mock_spireclient "github.com/otterize/credentials-operator/src/mocks/spireclient"
	"github.com/otterize/credentials-operator/src/testdata"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"github.com/spiffe/go-spiffe/v2/bundle/x509bundle"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/spire-api-sdk/proto/spire/api/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"testing"
)

var (
	trustDomain = spiffeid.RequireTrustDomainFromString("example.org")
)

type StoreSuite struct {
	suite.Suite
	controller   *gomock.Controller
	spireClient  *mock_spireclient.MockServerClient
	bundleClient *mock_bundlev1.MockBundleClient
	store        Store
}

func (s *StoreSuite) SetupTest() {
	s.controller = gomock.NewController(s.T())
	s.spireClient = mock_spireclient.NewMockServerClient(s.controller)
	s.bundleClient = mock_bundlev1.NewMockBundleClient(s.controller)
	s.spireClient.EXPECT().NewBundleClient().Return(s.bundleClient)
	s.store = NewBundlesStore(s.spireClient)
}

func (s *StoreSuite) TearDownTest() {
	s.spireClient.EXPECT().Close()
	s.spireClient.Close()
}

func loadTestBundle() (*types.Bundle, error) {
	testData, err := testdata.LoadTestData()
	if err != nil {
		return nil, errors.Wrap(err)
	}

	x509Bundle, err := x509bundle.Parse(trustDomain, testData.BundlePEM)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	apiBundle := &types.Bundle{
		TrustDomain: trustDomain.String(),
		X509Authorities: lo.Map(x509Bundle.X509Authorities(), func(t *x509.Certificate, i int) *types.X509Certificate {
			return &types.X509Certificate{
				Asn1: t.Raw,
			}
		}),
	}

	return apiBundle, nil
}

func (s *StoreSuite) TestStore_GetTrustBundle() {
	testBundle, err := loadTestBundle()
	s.Require().NoError(err)
	s.bundleClient.EXPECT().GetBundle(gomock.Any(), gomock.Any()).Return(testBundle, nil)

	retBundle, err := s.store.GetTrustBundle(context.Background())
	s.Require().NoError(err)

	parsedBundle, err := x509bundle.Parse(trustDomain, retBundle.BundlePEM)
	s.Require().NoError(err)
	s.Require().Equal(parsedBundle.TrustDomain(), trustDomain)
	s.Require().ElementsMatch(
		lo.Map(parsedBundle.X509Authorities(), func(cert *x509.Certificate, i int) []byte { return cert.Raw }),
		lo.Map(testBundle.X509Authorities, func(cert *types.X509Certificate, i int) []byte { return cert.Asn1 }),
	)
}

func TestRunStoreSuite(t *testing.T) {
	suite.Run(t, new(StoreSuite))
}
