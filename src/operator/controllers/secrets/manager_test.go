package secrets

import (
	"context"
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/otterize/spire-integration-operator/src/controllers/metadata"
	"github.com/otterize/spire-integration-operator/src/controllers/secrets/types"
	mock_certificates "github.com/otterize/spire-integration-operator/src/mocks/certificates"
	mock_client "github.com/otterize/spire-integration-operator/src/mocks/controller-runtime/client"
	"github.com/otterize/spire-integration-operator/src/testdata"
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

type ManagerSuite struct {
	suite.Suite
	controller  *gomock.Controller
	client      *mock_client.MockClient
	mockCertGen *mock_certificates.MockCertificateDataGenerator
	manager     *KubernetesSecretsManager
}

func (s *ManagerSuite) SetupTest() {
	s.controller = gomock.NewController(s.T())
	s.client = mock_client.NewMockClient(s.controller)
	s.mockCertGen = mock_certificates.NewMockCertificateDataGenerator(s.controller)
	s.manager = NewSecretManager(s.client, s.mockCertGen)

	s.client.EXPECT().Scheme().AnyTimes()
}

type TLSSecretMatcher struct {
	name      string
	namespace string
	tlsData   *map[string][]byte
}

func (m *TLSSecretMatcher) Matches(x interface{}) bool {
	secret, ok := x.(*corev1.Secret)
	if !ok {
		return false
	}

	if secret.Name != m.name || secret.Namespace != m.namespace {
		return false
	}

	if secret.Labels == nil || secret.Labels[metadata.SecretTypeLabel] != string(secretstypes.TlsSecretType) {
		return false
	}

	if m.tlsData != nil && !reflect.DeepEqual(secret.Data, *m.tlsData) {
		return false
	}

	return true
}

func (m *TLSSecretMatcher) String() string {
	return fmt.Sprintf("TLSSecretsMatcher(name=%s, namespace=%s)", m.name, m.namespace)
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
	entryId := "/test"

	certType, err := secretstypes.StrToCertType("pem")
	s.Require().NoError(err)
	certConfig := secretstypes.CertConfig{CertType: certType, PEMConfig: secretstypes.NewPEMConfig("", "", "")}
	secretConf := secretstypes.NewSecretConfig(entryId, "", secretName, namespace, serviceName, certConfig)

	certData := secretstypes.CertificateData{Files: map[string][]byte{
		certConfig.PEMConfig.BundleFileName: testData.BundlePEM,
		certConfig.PEMConfig.KeyFileName:    testData.KeyPEM,
		certConfig.PEMConfig.SVIDFileName:   testData.SVIDPEM},
	}
	pem := secretstypes.PEMCert{Key: testData.KeyPEM, Bundle: testData.BundlePEM, SVID: testData.SVIDPEM}
	s.mockCertGen.EXPECT().GeneratePEM(gomock.Any(), secretConf.EntryID).Return(pem, nil)

	s.client.EXPECT().Create(
		gomock.Any(),
		&TLSSecretMatcher{
			namespace: namespace,
			name:      secretName,
			tlsData:   &certData.Files,
		},
	).Return(nil)

	err = s.manager.EnsureTLSSecret(context.Background(), secretConf, nil)
	s.Require().NoError(err)
}

func (s *ManagerSuite) TestManager_EnsureTLSSecret_ExistingSecretFound_NeedsRefresh() {
	namespace := "test_namespace"
	secretName := "test_secretname"
	serviceName := "test_servicename"
	secretFileNames := secretstypes.NewPEMConfig("", "", "")
	entryId := "/test"

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
					metadata.TLSSecretSVIDExpiryAnnotation:            time.Now().Format(time.RFC3339),
					metadata.SVIDFileNameAnnotation:                   secretFileNames.SVIDFileName,
					metadata.BundleFileNameAnnotation:                 secretFileNames.BundleFileName,
					metadata.KeyFileNameAnnotation:                    secretFileNames.KeyFileName,
					metadata.TLSSecretRegisteredServiceNameAnnotation: serviceName,
					metadata.TLSSecretEntryIDAnnotation:               entryId,
					metadata.CertTypeAnnotation:                       "pem",
				},
			},
		}
	})

	testData, err := testdata.LoadTestData()
	s.Require().NoError(err)

	certType, err := secretstypes.StrToCertType("pem")
	s.Require().NoError(err)
	certConfig := secretstypes.CertConfig{CertType: certType, PEMConfig: secretstypes.NewPEMConfig("", "", "")}
	secretConf := secretstypes.NewSecretConfig(entryId, "", secretName, namespace, serviceName, certConfig)

	certData := secretstypes.CertificateData{Files: map[string][]byte{
		certConfig.PEMConfig.BundleFileName: testData.BundlePEM,
		certConfig.PEMConfig.KeyFileName:    testData.KeyPEM,
		certConfig.PEMConfig.SVIDFileName:   testData.SVIDPEM},
	}

	pem := secretstypes.PEMCert{Key: testData.KeyPEM, Bundle: testData.BundlePEM, SVID: testData.SVIDPEM}
	s.mockCertGen.EXPECT().GeneratePEM(gomock.Any(), secretConf.EntryID).Return(pem, nil)

	s.client.EXPECT().Update(
		gomock.Any(),
		&TLSSecretMatcher{
			namespace: namespace,
			name:      secretName,
			tlsData:   &certData.Files,
		},
	).Return(nil)

	err = s.manager.EnsureTLSSecret(context.Background(), secretConf, nil)
	s.Require().NoError(err)
}

func (s *ManagerSuite) TestManager_EnsureTLSSecret_ExistingSecretFound_NoRefreshNeeded() {
	namespace := "test_namespace"
	secretName := "test_secretname"
	serviceName := "test_servicename"
	secretFileNames := secretstypes.NewPEMConfig("", "", "")
	entryId := "/test"

	s.client.EXPECT().Get(
		gomock.Any(),
		types.NamespacedName{Name: secretName, Namespace: namespace},
		gomock.Any(),
	).Return(nil).Do(func(ctx context.Context, key client.ObjectKey, found *corev1.Secret) {
		*found = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Labels: map[string]string{
					metadata.SecretTypeLabel: string(secretstypes.TlsSecretType),
				},
				Annotations: map[string]string{
					metadata.TLSSecretSVIDExpiryAnnotation:            time.Now().Add(2 * secretExpiryDelta).Format(time.RFC3339),
					metadata.SVIDFileNameAnnotation:                   secretFileNames.SVIDFileName,
					metadata.BundleFileNameAnnotation:                 secretFileNames.BundleFileName,
					metadata.KeyFileNameAnnotation:                    secretFileNames.KeyFileName,
					metadata.TLSSecretRegisteredServiceNameAnnotation: serviceName,
					metadata.TLSSecretEntryIDAnnotation:               entryId,
					metadata.CertTypeAnnotation:                       "pem",
				},
			},
		}
	})

	s.client.EXPECT().Update(
		gomock.Any(),
		&TLSSecretMatcher{
			namespace: namespace,
			name:      secretName,
		},
	).Return(nil)

	certType, err := secretstypes.StrToCertType("pem")
	s.Require().NoError(err)
	certConfig := secretstypes.CertConfig{CertType: certType, PEMConfig: secretstypes.NewPEMConfig("", "", "")}
	secretConf := secretstypes.NewSecretConfig(entryId, "", secretName, namespace, serviceName, certConfig)

	err = s.manager.EnsureTLSSecret(context.Background(), secretConf, nil)
	s.Require().NoError(err)
}

func (s *ManagerSuite) TestManager_EnsureTLSSecret_ExistingSecretFound_UpdateNeeded_NewSecrets() {
	namespace := "test_namespace"
	secretName := "test_secretname"
	serviceName := "test_servicename"
	secretFileNames := secretstypes.NewPEMConfig("", "", "")

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
					metadata.TLSSecretSVIDExpiryAnnotation:            time.Now().Add(2 * secretExpiryDelta).Format(time.RFC3339),
					metadata.SVIDFileNameAnnotation:                   secretFileNames.SVIDFileName,
					metadata.BundleFileNameAnnotation:                 secretFileNames.BundleFileName,
					metadata.KeyFileNameAnnotation:                    secretFileNames.KeyFileName,
					metadata.TLSSecretRegisteredServiceNameAnnotation: serviceName,
					metadata.TLSSecretEntryHashAnnotation:             "",
					metadata.CertTypeAnnotation:                       "pem",
				},
			},
		}
	})

	testData, err := testdata.LoadTestData()
	s.Require().NoError(err)

	entryId := "/test"

	newSecrets := secretstypes.NewPEMConfig("different", "names", "this-time")

	certType, err := secretstypes.StrToCertType("pem")
	s.Require().NoError(err)
	certConfig := secretstypes.CertConfig{CertType: certType, PEMConfig: newSecrets}

	secretConf := secretstypes.NewSecretConfig(entryId, "", secretName, namespace, serviceName, certConfig)

	pem := secretstypes.PEMCert{Key: testData.KeyPEM, Bundle: testData.BundlePEM, SVID: testData.SVIDPEM}
	s.mockCertGen.EXPECT().GeneratePEM(gomock.Any(), secretConf.EntryID).Return(pem, nil)

	s.client.EXPECT().Update(
		gomock.Any(),
		&TLSSecretMatcher{
			namespace: namespace,
			name:      secretName,
			tlsData: &map[string][]byte{
				newSecrets.BundleFileName: testData.BundlePEM,
				newSecrets.KeyFileName:    testData.KeyPEM,
				newSecrets.SVIDFileName:   testData.SVIDPEM,
			},
		},
	).Return(nil)

	err = s.manager.EnsureTLSSecret(context.Background(), secretConf, nil)
	s.Require().NoError(err)
}

func (s *ManagerSuite) TestManager_EnsureTLSSecret_ExistingSecretFound_UpdateNeeded_EntryHashChanged() {
	namespace := "test_namespace"
	secretName := "test_secretname"
	serviceName := "test_servicename"
	secretFileNames := secretstypes.NewPEMConfig("", "", "")

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
					metadata.TLSSecretSVIDExpiryAnnotation:            time.Now().Add(2 * secretExpiryDelta).Format(time.RFC3339),
					metadata.SVIDFileNameAnnotation:                   secretFileNames.SVIDFileName,
					metadata.BundleFileNameAnnotation:                 secretFileNames.BundleFileName,
					metadata.KeyFileNameAnnotation:                    secretFileNames.KeyFileName,
					metadata.TLSSecretRegisteredServiceNameAnnotation: serviceName,
					metadata.TLSSecretEntryHashAnnotation:             "",
					metadata.CertTypeAnnotation:                       "pem",
				},
			},
		}
	})

	testData, err := testdata.LoadTestData()
	s.Require().NoError(err)

	entryId := "/test"

	certType, err := secretstypes.StrToCertType("pem")
	s.Require().NoError(err)
	certConfig := secretstypes.CertConfig{CertType: certType, PEMConfig: secretFileNames}

	newEntryHash := "New-Hash"
	secretConf := secretstypes.NewSecretConfig(entryId, newEntryHash, secretName, namespace, serviceName, certConfig)

	pem := secretstypes.PEMCert{Key: testData.KeyPEM, Bundle: testData.BundlePEM, SVID: testData.SVIDPEM}
	s.mockCertGen.EXPECT().GeneratePEM(gomock.Any(), secretConf.EntryID).Return(pem, nil)

	s.client.EXPECT().Update(
		gomock.Any(),
		&TLSSecretMatcher{
			namespace: namespace,
			name:      secretName,
			tlsData: &map[string][]byte{
				secretFileNames.BundleFileName: testData.BundlePEM,
				secretFileNames.KeyFileName:    testData.KeyPEM,
				secretFileNames.SVIDFileName:   testData.SVIDPEM,
			},
		},
	).Return(nil)

	err = s.manager.EnsureTLSSecret(context.Background(), secretConf, nil)
	s.Require().NoError(err)
}

func (s *ManagerSuite) TestManager_EnsureTLSSecret_ExistingSecretFound_UpdateNeeded_CertTypeChanged_JKS_To_PEM() {
	namespace := "test_namespace"
	secretName := "test_secretname"
	serviceName := "test_servicename"
	secretFileNames := secretstypes.NewPEMConfig("", "", "")

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
					metadata.TLSSecretSVIDExpiryAnnotation:            time.Now().Add(2 * secretExpiryDelta).Format(time.RFC3339),
					metadata.SVIDFileNameAnnotation:                   secretFileNames.SVIDFileName,
					metadata.BundleFileNameAnnotation:                 secretFileNames.BundleFileName,
					metadata.KeyFileNameAnnotation:                    secretFileNames.KeyFileName,
					metadata.TLSSecretRegisteredServiceNameAnnotation: serviceName,
					metadata.TLSSecretEntryHashAnnotation:             "",
					metadata.CertTypeAnnotation:                       "jks",
				},
			},
		}
	})

	testData, err := testdata.LoadTestData()
	s.Require().NoError(err)

	entryId := "/test"

	newCertType := "pem"
	certType, err := secretstypes.StrToCertType(newCertType)
	s.Require().NoError(err)
	certConfig := secretstypes.CertConfig{CertType: certType, PEMConfig: secretstypes.NewPEMConfig("", "", "")}
	secretConf := secretstypes.NewSecretConfig(entryId, "", secretName, namespace, serviceName, certConfig)

	certData := secretstypes.CertificateData{Files: map[string][]byte{
		certConfig.PEMConfig.BundleFileName: testData.BundlePEM,
		certConfig.PEMConfig.KeyFileName:    testData.KeyPEM,
		certConfig.PEMConfig.SVIDFileName:   testData.SVIDPEM},
	}
	pem := secretstypes.PEMCert{Key: testData.KeyPEM, Bundle: testData.BundlePEM, SVID: testData.SVIDPEM}
	s.mockCertGen.EXPECT().GeneratePEM(gomock.Any(), secretConf.EntryID).Return(pem, nil)

	s.client.EXPECT().Update(
		gomock.Any(),
		&TLSSecretMatcher{
			namespace: namespace,
			name:      secretName,
			tlsData:   &certData.Files,
		},
	).Return(nil)

	err = s.manager.EnsureTLSSecret(context.Background(), secretConf, nil)
	s.Require().NoError(err)
}

func (s *ManagerSuite) TestManager_EnsureTLSSecret_ExistingSecretFound_UpdateNeeded_CertTypeChanged_PEM_To_JKS() {
	namespace := "test_namespace"
	secretName := "test_secretname"
	serviceName := "test_servicename"
	secretFileNames := secretstypes.NewPEMConfig("", "", "")

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
					metadata.TLSSecretSVIDExpiryAnnotation:            time.Now().Add(2 * secretExpiryDelta).Format(time.RFC3339),
					metadata.SVIDFileNameAnnotation:                   secretFileNames.SVIDFileName,
					metadata.BundleFileNameAnnotation:                 secretFileNames.BundleFileName,
					metadata.KeyFileNameAnnotation:                    secretFileNames.KeyFileName,
					metadata.TLSSecretRegisteredServiceNameAnnotation: serviceName,
					metadata.TLSSecretEntryHashAnnotation:             "",
					metadata.CertTypeAnnotation:                       "pem",
				},
			},
		}
	})

	entryId := "/test"

	newCertType := "jks"
	certType, err := secretstypes.StrToCertType(newCertType)
	s.Require().NoError(err)
	certConfig := secretstypes.CertConfig{CertType: certType, JKSConfig: secretstypes.NewJKSConfig("", "", "")}
	secretConf := secretstypes.NewSecretConfig(entryId, "", secretName, namespace, serviceName, certConfig)

	jks := secretstypes.JKSCert{KeyStore: []byte("test1234"), TrustStore: []byte("testy-test")}
	certData := secretstypes.CertificateData{Files: map[string][]byte{
		certConfig.JKSConfig.KeyStoreFileName:   jks.KeyStore,
		certConfig.JKSConfig.TrustStoreFileName: jks.TrustStore},
	}
	s.mockCertGen.EXPECT().GenerateJKS(gomock.Any(), secretConf.EntryID, "password").Return(jks, nil)

	s.client.EXPECT().Update(
		gomock.Any(),
		&TLSSecretMatcher{
			namespace: namespace,
			name:      secretName,
			tlsData:   &certData.Files,
		},
	).Return(nil)

	err = s.manager.EnsureTLSSecret(context.Background(), secretConf, nil)
	s.Require().NoError(err)
}

func (s *ManagerSuite) TestManager_RefreshTLSSecrets_RefreshNeeded() {
	namespace := "test_namespace"
	secretName := "test_secretname"
	serviceName := "test_servicename"
	entryId := "/test"
	secretFileNames := secretstypes.NewPEMConfig("", "", "")
	certTypeStr := "pem"

	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Annotations: map[string]string{
				metadata.TLSSecretSVIDExpiryAnnotation:            time.Now().Format(time.RFC3339),
				metadata.TLSSecretRegisteredServiceNameAnnotation: serviceName,
				metadata.TLSSecretEntryIDAnnotation:               entryId,
				metadata.SVIDFileNameAnnotation:                   secretFileNames.SVIDFileName,
				metadata.BundleFileNameAnnotation:                 secretFileNames.BundleFileName,
				metadata.KeyFileNameAnnotation:                    secretFileNames.KeyFileName,
				metadata.CertTypeAnnotation:                       certTypeStr,
			},
		},
	}
	s.client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&corev1.SecretList{}),
		gomock.AssignableToTypeOf(&client.MatchingLabels{}),
	).Do(func(ctx context.Context, list *corev1.SecretList, opts ...client.ListOption) {
		*list = corev1.SecretList{
			Items: []corev1.Secret{secret},
		}
	})

	testData, err := testdata.LoadTestData()
	s.Require().NoError(err)
	certType, err := secretstypes.StrToCertType(certTypeStr)
	s.Require().NoError(err)
	certConfig := secretstypes.CertConfig{CertType: certType, PEMConfig: secretFileNames}
	secretConf := secretstypes.NewSecretConfig(entryId, "", secretName, namespace, serviceName, certConfig)

	pem := secretstypes.PEMCert{Key: testData.KeyPEM, Bundle: testData.BundlePEM, SVID: testData.SVIDPEM}
	s.mockCertGen.EXPECT().GeneratePEM(gomock.Any(), secretConf.EntryID).Return(pem, nil)

	s.client.EXPECT().Update(
		gomock.Any(),
		&TLSSecretMatcher{
			namespace: namespace,
			name:      secretName,
			tlsData: &map[string][]byte{
				secretFileNames.BundleFileName: testData.BundlePEM,
				secretFileNames.KeyFileName:    testData.KeyPEM,
				secretFileNames.SVIDFileName:   testData.SVIDPEM,
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
	entryId := "/test"
	secretFileNames := secretstypes.NewPEMConfig("", "", "")

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
							metadata.TLSSecretSVIDExpiryAnnotation:            time.Now().Add(2 * secretExpiryDelta).Format(time.RFC3339),
							metadata.TLSSecretRegisteredServiceNameAnnotation: serviceName,
							metadata.TLSSecretEntryIDAnnotation:               entryId,
							metadata.SVIDFileNameAnnotation:                   secretFileNames.SVIDFileName,
							metadata.BundleFileNameAnnotation:                 secretFileNames.BundleFileName,
							metadata.KeyFileNameAnnotation:                    secretFileNames.KeyFileName,
							metadata.CertTypeAnnotation:                       "pem",
						},
					},
				},
			},
		}
	})

	err := s.manager.RefreshTLSSecrets(context.Background())
	s.Require().NoError(err)
}

func TestRunManagerSuite(t *testing.T) {
	suite.Run(t, new(ManagerSuite))
}
