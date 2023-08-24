package svids

import (
	"context"
	"crypto"
	"crypto/x509"
	mock_entryv1 "github.com/otterize/credentials-operator/src/mocks/entryv1"
	mock_spireclient "github.com/otterize/credentials-operator/src/mocks/spireclient"
	mock_svidv1 "github.com/otterize/credentials-operator/src/mocks/svidv1"
	"github.com/otterize/credentials-operator/src/testdata"
	"github.com/samber/lo"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	svidv1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/svid/v1"
	"github.com/spiffe/spire-api-sdk/proto/spire/api/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"testing"
)

var (
	trustDomain = spiffeid.RequireTrustDomainFromString("example.org")
	entryID     = "test-entry"
)

type StoreSuite struct {
	suite.Suite
	controller  *gomock.Controller
	spireClient *mock_spireclient.MockServerClient
	svidClient  *mock_svidv1.MockSVIDClient
	entryClient *mock_entryv1.MockEntryClient
	store       Store
}

func (s *StoreSuite) SetupTest() {
	s.controller = gomock.NewController(s.T())
	s.spireClient = mock_spireclient.NewMockServerClient(s.controller)
	s.svidClient = mock_svidv1.NewMockSVIDClient(s.controller)
	s.spireClient.EXPECT().NewSVIDClient().Return(s.svidClient)
	s.entryClient = mock_entryv1.NewMockEntryClient(s.controller)
	s.spireClient.EXPECT().NewEntryClient().Return(s.entryClient)
	s.store = NewSVIDsStore(s.spireClient)
}

func (s *StoreSuite) TearDownTest() {
	s.spireClient.EXPECT().Close()
	s.spireClient.Close()
}

func loadTestSVID() (string, *types.X509SVID, crypto.PrivateKey, error) {
	testData, err := testdata.LoadTestData()
	if err != nil {
		return "", nil, nil, err
	}
	x509SVID, err := x509svid.Parse(testData.SVIDPEM, testData.KeyPEM)
	if err != nil {
		return "", nil, nil, err
	}
	spiffeID := x509SVID.ID
	apiSVID := &types.X509SVID{
		CertChain: lo.Map(x509SVID.Certificates, func(cert *x509.Certificate, i int) []byte { return cert.Raw }),
		Id:        &types.SPIFFEID{TrustDomain: spiffeID.TrustDomain().String(), Path: spiffeID.Path()},
		ExpiresAt: lo.Min(lo.Map(x509SVID.Certificates, func(cert *x509.Certificate, i int) int64 { return cert.NotAfter.Unix() })),
	}

	return entryID, apiSVID, x509SVID.PrivateKey, nil
}

func (s *StoreSuite) TestStore_GetTrustBundle() {
	entryId, testSVID, privateKey, err := loadTestSVID()
	s.Require().NoError(err)

	s.svidClient.EXPECT().MintX509SVID(gomock.Any(), gomock.Any()).Return(&svidv1.MintX509SVIDResponse{Svid: testSVID}, nil)
	s.entryClient.EXPECT().GetEntry(gomock.Any(), gomock.Any()).Return(&types.Entry{Id: entryID, SpiffeId: &types.SPIFFEID{Path: "/abc", TrustDomain: trustDomain.String()}}, nil)

	retSVID, err := s.store.GetX509SVID(context.Background(), entryId, privateKey)
	s.Require().NoError(err)
	s.Require().Equal(retSVID.ExpiresAt, testSVID.ExpiresAt)

	parsedSVID, err := x509svid.Parse(retSVID.SVIDPEM, retSVID.KeyPEM)
	s.Require().NoError(err)
	s.Require().Equal(parsedSVID.ID.TrustDomain().String(), testSVID.Id.TrustDomain)
	s.Require().Equal(parsedSVID.ID.Path(), testSVID.Id.GetPath())
}

func TestRunStoreSuite(t *testing.T) {
	suite.Run(t, new(StoreSuite))
}
