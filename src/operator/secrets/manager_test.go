package secrets

import (
	"context"
	"fmt"
	"github.com/golang/mock/gomock"
	mock_client "github.com/otterize/spifferize/src/mocks/controller-runtime/client"
	mock_bundles "github.com/otterize/spifferize/src/mocks/spireclient/bundles"
	mock_svids "github.com/otterize/spifferize/src/mocks/spireclient/svids"
	"github.com/otterize/spifferize/src/spireclient/bundles"
	"github.com/otterize/spifferize/src/spireclient/svids"
	"github.com/otterize/spifferize/src/testdata"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/spire/pkg/common/pemutil"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"
)

var (
	trustDomain = spiffeid.RequireTrustDomainFromString("example.org")
)

type ManagerSuite struct {
	suite.Suite
	controller   *gomock.Controller
	client       *mock_client.MockClient
	bundlesStore *mock_bundles.MockStore
	svidsStore   *mock_svids.MockStore
	manager      Manager
}

func (s *ManagerSuite) SetupTest() {
	s.controller = gomock.NewController(s.T())
	s.client = mock_client.NewMockClient(s.controller)
	s.bundlesStore = mock_bundles.NewMockStore(s.controller)
	s.svidsStore = mock_svids.NewMockStore(s.controller)
	s.manager = NewSecretsManager(s.client, s.bundlesStore, s.svidsStore)
}

type TLSSecretMatcher struct {
	name      string
	namespace string
	tlsData   map[string][]byte
}

func (m *TLSSecretMatcher) Matches(x interface{}) bool {
	secret, ok := x.(*corev1.Secret)
	if !ok {
		return false
	}

	if secret.Name != m.name && secret.Namespace != m.namespace {
		return false
	}

	if secret.Labels == nil || secret.Labels[secretTypeLabel] != string(tlsSecretType) {
		return false
	}

	if !reflect.DeepEqual(secret.Data, m.tlsData) {
		return false
	}

	return true
}

func (m *TLSSecretMatcher) String() string {
	return fmt.Sprintf("TLSSecretsMatcher(name=%s, namespace=%s)", m.name, m.namespace)
}

func (s *ManagerSuite) mockTLSStores(spiffeID spiffeid.ID, testData testdata.TestData) {
	encodedBundle := bundles.EncodedTrustBundle{BundlePEM: testData.BundlePEM}
	s.bundlesStore.EXPECT().GetTrustBundle(gomock.Any()).Return(encodedBundle, nil)

	privateKey, err := pemutil.ParseECPrivateKey(testData.KeyPEM)
	s.Require().NoError(err)
	s.svidsStore.EXPECT().GeneratePrivateKey().Return(privateKey, nil)

	encodedX509SVID := svids.EncodedX509SVID{
		SVIDPEM: testData.SVIDPEM,
		KeyPEM:  testData.KeyPEM,
	}
	s.svidsStore.EXPECT().GetX509SVID(
		gomock.Any(), spiffeID, privateKey,
	).Return(encodedX509SVID, nil)
}

func (s *ManagerSuite) TestManager_EnsureTLSSecret_NoExistingSecret() {
	namespace := "test_namespace"
	secretName := "test_secretname"
	serviceName := "test_servicename"

	s.client.EXPECT().Get(
		gomock.Any(),
		types.NamespacedName{Name: secretName, Namespace: namespace},
		gomock.Any(),
	).Return(errors.NewNotFound(schema.GroupResource{}, ""))

	testData, err := testdata.LoadTestData()
	s.Require().NoError(err)
	spiffeID, err := spiffeid.FromPath(trustDomain, "/test")
	s.Require().NoError(err)

	s.mockTLSStores(spiffeID, testData)

	s.client.EXPECT().Create(
		gomock.Any(),
		&TLSSecretMatcher{
			namespace: namespace,
			name:      secretName,
			tlsData: map[string][]byte{
				"bundle.pem": testData.BundlePEM,
				"key.pem":    testData.KeyPEM,
				"svid.pem":   testData.SVIDPEM,
			},
		},
	).Return(nil)

	err = s.manager.EnsureTLSSecret(context.Background(), namespace, secretName, serviceName, spiffeID)
	s.Require().NoError(err)
}

func (s *ManagerSuite) TestManager_EnsureTLSSecret_ExistingSecretFound_NeedsRefresh() {
	namespace := "test_namespace"
	secretName := "test_secretname"
	serviceName := "test_servicename"

	s.client.EXPECT().Get(
		gomock.Any(),
		types.NamespacedName{Name: secretName, Namespace: namespace},
		gomock.Any(),
	).Return(nil).Do(func(ctx context.Context, key client.ObjectKey, found *corev1.Secret) {
		*found = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Annotations: map[string]string{
					svidExpiryAnnotation: time.Now().Format(time.RFC3339),
				},
			},
		}
	})

	testData, err := testdata.LoadTestData()
	s.Require().NoError(err)
	spiffeID, err := spiffeid.FromPath(trustDomain, "/test")
	s.Require().NoError(err)

	s.mockTLSStores(spiffeID, testData)

	s.client.EXPECT().Update(
		gomock.Any(),
		&TLSSecretMatcher{
			namespace: namespace,
			name:      secretName,
			tlsData: map[string][]byte{
				"bundle.pem": testData.BundlePEM,
				"key.pem":    testData.KeyPEM,
				"svid.pem":   testData.SVIDPEM,
			},
		},
	).Return(nil)

	err = s.manager.EnsureTLSSecret(context.Background(), namespace, secretName, serviceName, spiffeID)
	s.Require().NoError(err)
}

func (s *ManagerSuite) TestManager_EnsureTLSSecret_ExistingSecretFound_NoRefreshNeeded() {
	namespace := "test_namespace"
	secretName := "test_secretname"
	serviceName := "test_servicename"

	s.client.EXPECT().Get(
		gomock.Any(),
		types.NamespacedName{Name: secretName, Namespace: namespace},
		gomock.Any(),
	).Return(nil).Do(func(ctx context.Context, key client.ObjectKey, found *corev1.Secret) {
		*found = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Annotations: map[string]string{
					svidExpiryAnnotation: time.Now().Add(2 * secretExpiryDelta).Format(time.RFC3339),
				},
			},
		}
	})

	spiffeID, err := spiffeid.FromPath(trustDomain, "/test")
	s.Require().NoError(err)

	err = s.manager.EnsureTLSSecret(context.Background(), namespace, secretName, serviceName, spiffeID)
	s.Require().NoError(err)
}

func (s *ManagerSuite) TestManager_RefreshTLSSecrets_RefreshNeeded() {
	namespace := "test_namespace"
	secretName := "test_secretname"
	serviceName := "test_servicename"
	spiffeID, err := spiffeid.FromPath(trustDomain, "/test")
	s.Require().NoError(err)

	s.client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&corev1.SecretList{}),
		gomock.AssignableToTypeOf(&client.MatchingLabels{}),
	).Do(func(ctx context.Context, list *corev1.SecretList, opts ...client.ListOption) {
		*list = corev1.SecretList{
			Items: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName,
						Namespace: namespace,
						Annotations: map[string]string{
							svidExpiryAnnotation:           time.Now().Format(time.RFC3339),
							tlsSecretServiceNameAnnotation: serviceName,
							tlsSecretSPIFFEIDAnnotation:    spiffeID.String(),
						},
					},
				},
			},
		}
	})

	testData, err := testdata.LoadTestData()
	s.Require().NoError(err)
	s.mockTLSStores(spiffeID, testData)

	s.client.EXPECT().Update(
		gomock.Any(),
		&TLSSecretMatcher{
			namespace: namespace,
			name:      secretName,
			tlsData: map[string][]byte{
				"bundle.pem": testData.BundlePEM,
				"key.pem":    testData.KeyPEM,
				"svid.pem":   testData.SVIDPEM,
			},
		},
	).Return(nil)

	err = s.manager.RefreshTLSSecrets(context.Background())
	s.Require().NoError(err)
}

func (s *ManagerSuite) TestManager_RefreshTLSSecrets_NoRefreshNeeded() {
	namespace := "test_namespace"
	secretName := "test_secretname"
	serviceName := "test_servicename"
	spiffeID, err := spiffeid.FromPath(trustDomain, "/test")
	s.Require().NoError(err)

	s.client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&corev1.SecretList{}),
		gomock.AssignableToTypeOf(&client.MatchingLabels{}),
	).Do(func(ctx context.Context, list *corev1.SecretList, opts ...client.ListOption) {
		*list = corev1.SecretList{
			Items: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName,
						Namespace: namespace,
						Annotations: map[string]string{
							svidExpiryAnnotation:           time.Now().Add(2 * secretExpiryDelta).Format(time.RFC3339),
							tlsSecretServiceNameAnnotation: serviceName,
							tlsSecretSPIFFEIDAnnotation:    spiffeID.String(),
						},
					},
				},
			},
		}
	})

	err = s.manager.RefreshTLSSecrets(context.Background())
	s.Require().NoError(err)
}

func TestRunManagerSuite(t *testing.T) {
	suite.Run(t, new(ManagerSuite))
}
