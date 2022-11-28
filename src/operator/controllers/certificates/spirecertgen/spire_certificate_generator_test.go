package spirecertgen

import (
	"bytes"
	"context"
	"encoding/pem"
	"github.com/golang/mock/gomock"
	"github.com/otterize/spire-integration-operator/src/controllers/secrets/types"
	"github.com/otterize/spire-integration-operator/src/controllers/spireclient/bundles"
	"github.com/otterize/spire-integration-operator/src/controllers/spireclient/svids"
	mock_bundles "github.com/otterize/spire-integration-operator/src/mocks/spireclient/bundles"
	mock_svids "github.com/otterize/spire-integration-operator/src/mocks/spireclient/svids"
	"github.com/otterize/spire-integration-operator/src/testdata"
	"github.com/pavlo-v-chernykh/keystore-go/v4"
	"github.com/spiffe/spire/pkg/common/pemutil"
	"github.com/stretchr/testify/suite"
	"strings"
	"testing"
	"time"
)

const ExpiryTimeTestStr = "06/24/94"
const ExpiryTimeTestLayout = "01/02/06"

type ManagerSuite struct {
	suite.Suite
	controller    *gomock.Controller
	bundlesStore  *mock_bundles.MockStore
	svidsStore    *mock_svids.MockStore
	certGenerator *SpireCertificateDataGenerator
}

func (s *ManagerSuite) SetupTest() {
	s.controller = gomock.NewController(s.T())
	s.bundlesStore = mock_bundles.NewMockStore(s.controller)
	s.svidsStore = mock_svids.NewMockStore(s.controller)
	s.certGenerator = NewSpireCertificateDataGenerator(s.bundlesStore, s.svidsStore)

}

func (s *ManagerSuite) mockTLSStores(entryId string, testData testdata.TestData) {
	encodedBundle := bundles.EncodedTrustBundle{BundlePEM: testData.BundlePEM}
	s.bundlesStore.EXPECT().GetTrustBundle(gomock.Any()).Return(encodedBundle, nil)

	privateKey, err := pemutil.ParseECPrivateKey(testData.KeyPEM)
	s.Require().NoError(err)
	s.svidsStore.EXPECT().GeneratePrivateKey().Return(privateKey, nil)

	expiry, err := time.Parse(ExpiryTimeTestLayout, ExpiryTimeTestStr)
	s.Require().NoError(err)

	encodedX509SVID := svids.EncodedX509SVID{
		SVIDPEM:   testData.SVIDPEM,
		KeyPEM:    testData.KeyPEM,
		ExpiresAt: expiry.Unix(),
	}
	s.svidsStore.EXPECT().GetX509SVID(
		gomock.Any(), entryId, privateKey,
	).Return(encodedX509SVID, nil)
}

func (s *ManagerSuite) TestCertGenerator_GeneratePEM() {

	testData, err := testdata.LoadTestData()
	s.Require().NoError(err)
	entryId := "/test"

	s.mockTLSStores(entryId, testData)

	certPEM, err := s.certGenerator.GeneratePEM(context.Background(), entryId)
	s.Require().NoError(err)
	expiry, err := time.Parse(ExpiryTimeTestLayout, ExpiryTimeTestStr)
	s.Require().NoError(err)
	expiryUnix := time.Unix(expiry.Unix(), 0)
	expectedCertData := secretstypes.PEMCert{
		Bundle: testData.BundlePEM,
		Key:    testData.KeyPEM,
		SVID:   testData.SVIDPEM,
		Expiry: expiryUnix.Format(time.RFC3339),
	}
	s.Equal(expectedCertData, certPEM)
}

func (s *ManagerSuite) TestCertGenerator_GenerateJKS() {
	entryId := "/test"
	password := []byte("password")
	testData, err := testdata.LoadTestData()
	s.Require().NoError(err)
	s.mockTLSStores(entryId, testData)
	certJKS, err := s.certGenerator.GenerateJKS(context.Background(), entryId, string(password))
	s.Require().NoError(err)

	// test cert expiry
	expiry, err := time.Parse(ExpiryTimeTestLayout, ExpiryTimeTestStr)
	s.Require().NoError(err)
	expiryUnix := time.Unix(expiry.Unix(), 0)
	s.Require().Equal(expiryUnix.Format(time.RFC3339), certJKS.Expiry)

	// test truststore is as expected
	ts := keystore.New()
	trustStoreReader := bytes.NewReader(certJKS.TrustStore)
	err = ts.Load(trustStoreReader, password)
	s.Require().NoError(err)
	s.Require().Equal(len(ts.Aliases()), 1)
	caAlias := ts.Aliases()[0]
	ca, err := ts.GetTrustedCertificateEntry(caAlias)
	s.Require().NoError(err)
	s.Require().Equal(testData.BundlePEM, ca.Certificate.Content)

	// test keystore is as expected
	ks := keystore.New()
	keyStoreReader := bytes.NewReader(certJKS.KeyStore)
	err = ks.Load(keyStoreReader, password)
	s.Require().NoError(err)
	s.Require().Equal(len(ks.Aliases()), 1)
	pkey, err := ks.GetPrivateKeyEntry("pkey", password)
	s.Require().NoError(err)
	// compare pkey
	decodedPkey, _ := pem.Decode(testData.KeyPEM)
	s.Require().Equal(decodedPkey.Bytes, pkey.PrivateKey)
	// compare certificate chain
	certChain := strings.Split(string(testData.SVIDPEM), "-\n-")
	// remove padding in order to compare
	s.Require().Equal([]byte(certChain[0]+"-"), pkey.CertificateChain[0].Content[:len(pkey.CertificateChain[0].Content)-1])
	s.Require().Equal([]byte("-"+certChain[1]), pkey.CertificateChain[1].Content)

}

func (s *ManagerSuite) TestJKSTrustStoreCreate() {
	testData, err := testdata.LoadTestData()
	s.Require().NoError(err)
	bundle := bundles.EncodedTrustBundle{BundlePEM: testData.BundlePEM}
	trustBundle, err := trustBundleToTrustStore(bundle, "password")
	s.Require().NoError(err)
	s.Require().NotNil(trustBundle)
}

func (s *ManagerSuite) TestJKSKeyStoreCreate() {
	testData, err := testdata.LoadTestData()
	s.Require().NoError(err)
	svid := svids.EncodedX509SVID{SVIDPEM: testData.SVIDPEM, KeyPEM: testData.KeyPEM}
	keyStore, err := svidToKeyStore(svid, "password")
	s.Require().NoError(err)
	s.Require().NotNil(keyStore)
}

func TestRunManagerSuite(t *testing.T) {
	suite.Run(t, new(ManagerSuite))
}
