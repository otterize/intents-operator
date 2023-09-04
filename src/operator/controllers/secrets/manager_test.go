package secrets

import (
	"context"
	"fmt"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/otterize/credentials-operator/src/controllers/secrets/types"
	mock_certificates "github.com/otterize/credentials-operator/src/mocks/certificates"
	mock_client "github.com/otterize/credentials-operator/src/mocks/controller-runtime/client"
	mock_record "github.com/otterize/credentials-operator/src/mocks/eventrecorder"
	mock_serviceidresolver "github.com/otterize/credentials-operator/src/mocks/serviceidresolver"
	"github.com/otterize/credentials-operator/src/testdata"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"
)

type ManagerSuite struct {
	suite.Suite
	controller        *gomock.Controller
	client            *mock_client.MockClient
	mockCertGen       *mock_certificates.MockCertificateDataGenerator
	eventRecorder     *mock_record.MockEventRecorder
	serviceIdResolver *mock_serviceidresolver.MockServiceIdResolver
	manager           *DirectSecretsManager
}

func (s *ManagerSuite) SetupTest() {
	s.controller = gomock.NewController(s.T())
	s.client = mock_client.NewMockClient(s.controller)
	s.mockCertGen = mock_certificates.NewMockCertificateDataGenerator(s.controller)
	s.eventRecorder = mock_record.NewMockEventRecorder(s.controller)
	s.serviceIdResolver = mock_serviceidresolver.NewMockServiceIdResolver(s.controller)
	s.manager = NewDirectSecretsManager(s.client, s.serviceIdResolver, s.eventRecorder, s.mockCertGen)

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
	secretConf := secretstypes.NewSecretConfig(entryId, "", secretName, namespace, serviceName, certConfig, false)

	certData := secretstypes.CertificateData{Files: map[string][]byte{
		certConfig.PEMConfig.CAFileName:   testData.BundlePEM,
		certConfig.PEMConfig.KeyFileName:  testData.KeyPEM,
		certConfig.PEMConfig.CertFileName: testData.SVIDPEM},
	}
	pem := secretstypes.PEMCert{Key: testData.KeyPEM, CA: testData.BundlePEM, Certificate: testData.SVIDPEM}
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
	).Return(nil).Do(func(ctx context.Context, key client.ObjectKey, found *corev1.Secret, opt ...any) {
		*found = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Annotations: map[string]string{
					metadata.TLSSecretExpiryAnnotation:                time.Now().Format(time.RFC3339),
					metadata.CertFileNameAnnotation:                   secretFileNames.CertFileName,
					metadata.CAFileNameAnnotation:                     secretFileNames.CAFileName,
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
	secretConf := secretstypes.NewSecretConfig(entryId, "", secretName, namespace, serviceName, certConfig, false)

	certData := secretstypes.CertificateData{Files: map[string][]byte{
		certConfig.PEMConfig.CAFileName:   testData.BundlePEM,
		certConfig.PEMConfig.KeyFileName:  testData.KeyPEM,
		certConfig.PEMConfig.CertFileName: testData.SVIDPEM},
	}

	pem := secretstypes.PEMCert{Key: testData.KeyPEM, CA: testData.BundlePEM, Certificate: testData.SVIDPEM}
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
	).Return(nil).Do(func(ctx context.Context, key client.ObjectKey, found *corev1.Secret, opt ...any) {
		*found = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Labels: map[string]string{
					metadata.SecretTypeLabel: string(secretstypes.TlsSecretType),
				},
				Annotations: map[string]string{
					metadata.TLSSecretExpiryAnnotation:                time.Now().Add(2 * secretExpiryDelta).Format(time.RFC3339),
					metadata.CertFileNameAnnotation:                   secretFileNames.CertFileName,
					metadata.CAFileNameAnnotation:                     secretFileNames.CAFileName,
					metadata.KeyFileNameAnnotation:                    secretFileNames.KeyFileName,
					metadata.TLSSecretRegisteredServiceNameAnnotation: serviceName,
					metadata.TLSSecretEntryIDAnnotation:               entryId,
					metadata.CertTypeAnnotation:                       "pem",
				},
			},
		}
	})

	certType, err := secretstypes.StrToCertType("pem")
	s.Require().NoError(err)
	certConfig := secretstypes.CertConfig{CertType: certType, PEMConfig: secretstypes.NewPEMConfig("", "", "")}
	secretConf := secretstypes.NewSecretConfig(entryId, "", secretName, namespace, serviceName, certConfig, false)

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
	).Return(nil).Do(func(ctx context.Context, key client.ObjectKey, found *corev1.Secret, opt ...any) {
		*found = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Annotations: map[string]string{
					metadata.TLSSecretExpiryAnnotation:                time.Now().Add(2 * secretExpiryDelta).Format(time.RFC3339),
					metadata.CertFileNameAnnotation:                   secretFileNames.CertFileName,
					metadata.CAFileNameAnnotation:                     secretFileNames.CAFileName,
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

	secretConf := secretstypes.NewSecretConfig(entryId, "", secretName, namespace, serviceName, certConfig, false)

	pem := secretstypes.PEMCert{Key: testData.KeyPEM, CA: testData.BundlePEM, Certificate: testData.SVIDPEM}
	s.mockCertGen.EXPECT().GeneratePEM(gomock.Any(), secretConf.EntryID).Return(pem, nil)

	s.client.EXPECT().Update(
		gomock.Any(),
		&TLSSecretMatcher{
			namespace: namespace,
			name:      secretName,
			tlsData: &map[string][]byte{
				newSecrets.CAFileName:   testData.BundlePEM,
				newSecrets.KeyFileName:  testData.KeyPEM,
				newSecrets.CertFileName: testData.SVIDPEM,
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
	).Return(nil).Do(func(ctx context.Context, key client.ObjectKey, found *corev1.Secret, opt ...any) {
		*found = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Annotations: map[string]string{
					metadata.TLSSecretExpiryAnnotation:                time.Now().Add(2 * secretExpiryDelta).Format(time.RFC3339),
					metadata.CertFileNameAnnotation:                   secretFileNames.CertFileName,
					metadata.CAFileNameAnnotation:                     secretFileNames.CAFileName,
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
	secretConf := secretstypes.NewSecretConfig(entryId, newEntryHash, secretName, namespace, serviceName, certConfig, false)

	pem := secretstypes.PEMCert{Key: testData.KeyPEM, CA: testData.BundlePEM, Certificate: testData.SVIDPEM}
	s.mockCertGen.EXPECT().GeneratePEM(gomock.Any(), secretConf.EntryID).Return(pem, nil)

	s.client.EXPECT().Update(
		gomock.Any(),
		&TLSSecretMatcher{
			namespace: namespace,
			name:      secretName,
			tlsData: &map[string][]byte{
				secretFileNames.CAFileName:   testData.BundlePEM,
				secretFileNames.KeyFileName:  testData.KeyPEM,
				secretFileNames.CertFileName: testData.SVIDPEM,
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
	).Return(nil).Do(func(ctx context.Context, key client.ObjectKey, found *corev1.Secret, opt ...any) {
		*found = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Annotations: map[string]string{
					metadata.TLSSecretExpiryAnnotation:                time.Now().Add(2 * secretExpiryDelta).Format(time.RFC3339),
					metadata.CertFileNameAnnotation:                   secretFileNames.CertFileName,
					metadata.CAFileNameAnnotation:                     secretFileNames.CAFileName,
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
	secretConf := secretstypes.NewSecretConfig(entryId, "", secretName, namespace, serviceName, certConfig, false)

	certData := secretstypes.CertificateData{Files: map[string][]byte{
		certConfig.PEMConfig.CAFileName:   testData.BundlePEM,
		certConfig.PEMConfig.KeyFileName:  testData.KeyPEM,
		certConfig.PEMConfig.CertFileName: testData.SVIDPEM},
	}
	pem := secretstypes.PEMCert{Key: testData.KeyPEM, CA: testData.BundlePEM, Certificate: testData.SVIDPEM}
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
	).Return(nil).Do(func(ctx context.Context, key client.ObjectKey, found *corev1.Secret, opt ...any) {
		*found = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Annotations: map[string]string{
					metadata.TLSSecretExpiryAnnotation:                time.Now().Add(2 * secretExpiryDelta).Format(time.RFC3339),
					metadata.CertFileNameAnnotation:                   secretFileNames.CertFileName,
					metadata.CAFileNameAnnotation:                     secretFileNames.CAFileName,
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
	secretConf := secretstypes.NewSecretConfig(entryId, "", secretName, namespace, serviceName, certConfig, false)

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

func (s *ManagerSuite) TestManager_RefreshTLSSecrets_RefreshNeeded_NOT_ShouldRestartPod() {
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
				metadata.TLSSecretExpiryAnnotation:                time.Now().Format(time.RFC3339),
				metadata.TLSSecretRegisteredServiceNameAnnotation: serviceName,
				metadata.TLSSecretEntryIDAnnotation:               entryId,
				metadata.CertFileNameAnnotation:                   secretFileNames.CertFileName,
				metadata.CAFileNameAnnotation:                     secretFileNames.CAFileName,
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
	secretConf := secretstypes.NewSecretConfig(entryId, "", secretName, namespace, serviceName, certConfig, false)

	pem := secretstypes.PEMCert{Key: testData.KeyPEM, CA: testData.BundlePEM, Certificate: testData.SVIDPEM}
	s.mockCertGen.EXPECT().GeneratePEM(gomock.Any(), secretConf.EntryID).Return(pem, nil)

	s.client.EXPECT().Update(
		gomock.Any(),
		&TLSSecretMatcher{
			namespace: namespace,
			name:      secretName,
			tlsData: &map[string][]byte{
				secretFileNames.CAFileName:   testData.BundlePEM,
				secretFileNames.KeyFileName:  testData.KeyPEM,
				secretFileNames.CertFileName: testData.SVIDPEM,
			},
		},
	).Return(nil)

	pod := corev1.Pod{}

	s.client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&corev1.PodList{}),
		gomock.AssignableToTypeOf(&client.ListOptions{}),
	).Do(func(ctx context.Context, list *corev1.PodList, opts ...client.ListOption) {
		*list = corev1.PodList{
			Items: []corev1.Pod{pod},
		}
	})

	err = s.manager.RefreshTLSSecrets(context.Background())
	s.Require().NoError(err)
}

func (s *ManagerSuite) TestManager_RefreshTLSSecrets_RefreshNeeded_ShouldRestartPod() {
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
				metadata.TLSSecretExpiryAnnotation:                time.Now().Format(time.RFC3339),
				metadata.TLSSecretRegisteredServiceNameAnnotation: serviceName,
				metadata.TLSSecretEntryIDAnnotation:               entryId,
				metadata.CertFileNameAnnotation:                   secretFileNames.CertFileName,
				metadata.CAFileNameAnnotation:                     secretFileNames.CAFileName,
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
	secretConf := secretstypes.NewSecretConfig(entryId, "", secretName, namespace, serviceName, certConfig, false)

	pem := secretstypes.PEMCert{Key: testData.KeyPEM, CA: testData.BundlePEM, Certificate: testData.SVIDPEM}
	s.mockCertGen.EXPECT().GeneratePEM(gomock.Any(), secretConf.EntryID).Return(pem, nil)

	s.client.EXPECT().Update(
		gomock.Any(),
		&TLSSecretMatcher{
			namespace: namespace,
			name:      secretName,
			tlsData: &map[string][]byte{
				secretFileNames.CAFileName:   testData.BundlePEM,
				secretFileNames.KeyFileName:  testData.KeyPEM,
				secretFileNames.CertFileName: testData.SVIDPEM,
			},
		},
	).Return(nil)

	pod := corev1.Pod{}
	pod.Annotations = map[string]string{metadata.ShouldRestartOnRenewalAnnotation: "Yap"}

	s.client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&corev1.PodList{}),
		gomock.AssignableToTypeOf(&client.ListOptions{}),
	).Do(func(ctx context.Context, list *corev1.PodList, opts ...client.ListOption) {
		*list = corev1.PodList{
			Items: []corev1.Pod{pod},
		}
	})
	uu := unstructured.Unstructured{}
	uu.SetKind("Deployment")
	uu.SetName("name")
	uu.SetNamespace(namespace)
	s.serviceIdResolver.EXPECT().GetOwnerObject(
		gomock.Any(),
		gomock.AssignableToTypeOf(&corev1.Pod{}),
	).Return(&uu, nil)

	// expect get according to 'uu' name, namespace and kind
	s.client.EXPECT().Get(
		gomock.Any(),
		gomock.Eq(types.NamespacedName{Name: "name", Namespace: namespace}),
		gomock.AssignableToTypeOf(&v1.Deployment{}),
	)

	// expect update deployment
	s.client.EXPECT().Update(
		gomock.Any(),
		gomock.AssignableToTypeOf(&v1.Deployment{}),
	).Do(func(_ any, d *v1.Deployment, options ...any) {
		// Check that the Owner's pod template annotation was updated
		_, ok := d.Spec.Template.Annotations[metadata.TLSRestartTimeAfterRenewal]
		s.Require().True(ok)
	})

	// expect event to be sent
	s.eventRecorder.EXPECT().Eventf(
		gomock.AssignableToTypeOf(&v1.Deployment{}),
		gomock.Eq(corev1.EventTypeNormal),
		gomock.Eq(CertRenewReason),
		gomock.Any(),
		gomock.Eq(secretName),
	)

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
							metadata.TLSSecretExpiryAnnotation:                time.Now().Add(2 * secretExpiryDelta).Format(time.RFC3339),
							metadata.TLSSecretRegisteredServiceNameAnnotation: serviceName,
							metadata.TLSSecretEntryIDAnnotation:               entryId,
							metadata.CertFileNameAnnotation:                   secretFileNames.CertFileName,
							metadata.CAFileNameAnnotation:                     secretFileNames.CAFileName,
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
