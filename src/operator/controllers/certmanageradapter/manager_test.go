package certmanageradapter

import (
	"context"
	"fmt"
	certmanager "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/otterize/credentials-operator/src/controllers/secrets/types"
	mock_client "github.com/otterize/credentials-operator/src/mocks/controller-runtime/client"
	mock_record "github.com/otterize/credentials-operator/src/mocks/eventrecorder"
	mock_serviceidresolver "github.com/otterize/credentials-operator/src/mocks/serviceidresolver"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"testing"
)

type CertificateMatcher struct {
	name             string
	namespace        string
	dnsNames         []string
	serviceName      string
	issuerName       string
	useClusterIssuer bool
	jks              bool
}

func (m *CertificateMatcher) Matches(x interface{}) bool {
	cert, ok := x.(*certmanager.Certificate)
	if !ok {
		fmt.Println("object is not Certificate")
		return false
	}

	if cert.Name != m.name || cert.Namespace != m.namespace {
		fmt.Println("name doesn't match")
		return false
	}

	if cert.Labels == nil || cert.Labels[metadata.SecretTypeLabel] != string(secretstypes.TlsSecretType) {
		fmt.Println("labels doesn't match")
		return false
	}

	if cert.Spec.SecretName != m.name || cert.Spec.CommonName != fmt.Sprintf("%s.%s", m.serviceName, m.namespace) {
		fmt.Println("secret name doesn't match")
		return false
	}

	if cert.Spec.IssuerRef.Name != m.issuerName ||
		(m.useClusterIssuer && cert.Spec.IssuerRef.Kind != "ClusterIssuer") ||
		(!m.useClusterIssuer && cert.Spec.IssuerRef.Kind != "Issuer") {
		fmt.Println("issuer doesn't match")
		return false
	}

	if m.dnsNames != nil && !reflect.DeepEqual(cert.Spec.DNSNames, m.dnsNames) {
		fmt.Println("dns names doesn't match")
		return false
	}

	if m.jks {
		expectedKeystores := &certmanager.CertificateKeystores{
			JKS: &certmanager.JKSKeystore{
				Create: true,
				PasswordSecretRef: cmmeta.SecretKeySelector{
					LocalObjectReference: cmmeta.LocalObjectReference{
						Name: JKSPasswordsSecretName,
					},
					Key: m.name,
				},
			},
		}
		if !reflect.DeepEqual(cert.Spec.Keystores, expectedKeystores) {
			fmt.Println("secret has no JKS")
			return false
		}
	} else {
		if cert.Spec.Keystores != nil {
			fmt.Println("secret has JKS: ")
			return false
		}
	}

	return true
}

func (m *CertificateMatcher) String() string {
	return fmt.Sprintf("CertificateMatcher(name=%s, namespace=%s)", m.name, m.namespace)
}

type ManagerSuite struct {
	suite.Suite
	controller        *gomock.Controller
	client            *mock_client.MockClient
	eventRecorder     *mock_record.MockEventRecorder
	serviceIdResolver *mock_serviceidresolver.MockServiceIdResolver
	manager           *CertManagerSecretsManager
	registry          *CertManagerWorkloadRegistry
}

func (s *ManagerSuite) SetupTest() {
	s.controller = gomock.NewController(s.T())
	s.client = mock_client.NewMockClient(s.controller)
	s.eventRecorder = mock_record.NewMockEventRecorder(s.controller)
	s.serviceIdResolver = mock_serviceidresolver.NewMockServiceIdResolver(s.controller)
	s.manager, s.registry = NewCertManagerSecretsManager(s.client, s.serviceIdResolver, s.eventRecorder, "issuer", false)

	s.client.EXPECT().Scheme().AnyTimes()
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

	entryId, err := s.registry.RegisterK8SPod(context.Background(), namespace, "", serviceName, 0, []string{"dns1", "dns2"})
	s.Require().NoError(err)

	certType, err := secretstypes.StrToCertType("pem")
	s.Require().NoError(err)
	certConfig := secretstypes.CertConfig{CertType: certType, PEMConfig: secretstypes.NewPEMConfig("", "", "")}
	secretConf := secretstypes.NewSecretConfig(entryId, "", secretName, namespace, serviceName, certConfig, false)

	s.client.EXPECT().Create(
		gomock.Any(),
		&CertificateMatcher{
			namespace:        namespace,
			name:             secretName,
			dnsNames:         []string{"dns1", "dns2", strings.Join([]string{serviceName, namespace}, ".")},
			serviceName:      serviceName,
			issuerName:       "issuer",
			useClusterIssuer: false,
			jks:              false,
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
	entryId, err := s.registry.RegisterK8SPod(context.Background(), namespace, "", serviceName, 0, []string{"dns1", "dns2"})
	s.Require().NoError(err)

	s.client.EXPECT().Get(
		gomock.Any(),
		types.NamespacedName{Name: secretName, Namespace: namespace},
		gomock.Any(),
	).Return(nil).Do(func(ctx context.Context, key client.ObjectKey, found *certmanager.Certificate, opt ...any) {
		*found = certmanager.Certificate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Annotations: map[string]string{
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

	s.client.EXPECT().Update(
		gomock.Any(),
		&CertificateMatcher{
			namespace:        namespace,
			name:             secretName,
			dnsNames:         []string{"dns1", "dns2", strings.Join([]string{serviceName, namespace}, ".")},
			serviceName:      serviceName,
			issuerName:       "issuer",
			useClusterIssuer: false,
			jks:              false,
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
	entryId, err := s.registry.RegisterK8SPod(context.Background(), namespace, "", serviceName, 0, []string{"dns1", "dns2"})
	s.Require().NoError(err)

	s.client.EXPECT().Get(
		gomock.Any(),
		types.NamespacedName{Name: secretName, Namespace: namespace},
		gomock.Any(),
	).Return(nil).Do(func(ctx context.Context, key client.ObjectKey, found *certmanager.Certificate, opt ...any) {
		*found = certmanager.Certificate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Labels: map[string]string{
					metadata.SecretTypeLabel: string(secretstypes.TlsSecretType),
				},
				Annotations: map[string]string{
					metadata.CertFileNameAnnotation:                   secretFileNames.CertFileName,
					metadata.CAFileNameAnnotation:                     secretFileNames.CAFileName,
					metadata.KeyFileNameAnnotation:                    secretFileNames.KeyFileName,
					metadata.TLSSecretRegisteredServiceNameAnnotation: serviceName,
					metadata.TLSSecretEntryIDAnnotation:               entryId,
					metadata.CertTypeAnnotation:                       "pem",
				},
			},
			Spec: certmanager.CertificateSpec{
				SecretName: secretName,
				CommonName: fmt.Sprintf("%s.%s", serviceName, namespace),
				DNSNames:   []string{"dns1", "dns2", strings.Join([]string{serviceName, namespace}, ".")},
				IssuerRef: cmmeta.ObjectReference{
					Name: "issuer",
					Kind: "Issuer",
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

func (s *ManagerSuite) TestManager_EnsureTLSSecret_ExistingSecretFound_UpdateNeeded_NewServiceName() {
	namespace := "test_namespace"
	secretName := "test_secretname"
	serviceName := "test_servicename"
	secretFileNames := secretstypes.NewPEMConfig("", "", "")

	entryId, err := s.registry.RegisterK8SPod(context.Background(), namespace, "", serviceName, 0, []string{"dns1", "dns2"})
	s.Require().NoError(err)

	s.client.EXPECT().Get(
		gomock.Any(),
		types.NamespacedName{Name: secretName, Namespace: namespace},
		gomock.Any(),
	).Return(nil).Do(func(ctx context.Context, key client.ObjectKey, found *certmanager.Certificate, opt ...any) {
		*found = certmanager.Certificate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Labels: map[string]string{
					metadata.SecretTypeLabel: string(secretstypes.TlsSecretType),
				},
				Annotations: map[string]string{
					metadata.CertFileNameAnnotation:                   secretFileNames.CertFileName,
					metadata.CAFileNameAnnotation:                     secretFileNames.CAFileName,
					metadata.KeyFileNameAnnotation:                    secretFileNames.KeyFileName,
					metadata.TLSSecretRegisteredServiceNameAnnotation: serviceName,
					metadata.TLSSecretEntryIDAnnotation:               entryId,
					metadata.CertTypeAnnotation:                       "pem",
				},
			},
			Spec: certmanager.CertificateSpec{
				SecretName: secretName,
				CommonName: fmt.Sprintf("%s.%s", serviceName, namespace),
				DNSNames:   []string{"dns1", "dns2", strings.Join([]string{serviceName, namespace}, ".")},
				IssuerRef: cmmeta.ObjectReference{
					Name: "issuer",
					Kind: "Issuer",
				},
			},
		}
	})

	// Change service name
	newServiceName := serviceName + "_new"
	newEntryId, err := s.registry.RegisterK8SPod(context.Background(), namespace, "", newServiceName, 0, []string{"dns1", "dns2"})
	s.Require().NoError(err)

	certType, err := secretstypes.StrToCertType("pem")
	s.Require().NoError(err)
	certConfig := secretstypes.CertConfig{CertType: certType, PEMConfig: secretFileNames}

	secretConf := secretstypes.NewSecretConfig(newEntryId, "", secretName, namespace, newServiceName, certConfig, false)

	s.client.EXPECT().Update(
		gomock.Any(),
		&CertificateMatcher{
			namespace:        namespace,
			name:             secretName,
			dnsNames:         []string{"dns1", "dns2", strings.Join([]string{newServiceName, namespace}, ".")},
			serviceName:      newServiceName,
			issuerName:       "issuer",
			useClusterIssuer: false,
			jks:              false,
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

	entryId, err := s.registry.RegisterK8SPod(context.Background(), namespace, "", serviceName, 0, []string{"dns1", "dns2"})
	s.Require().NoError(err)

	s.client.EXPECT().Get(
		gomock.Any(),
		types.NamespacedName{Name: secretName, Namespace: namespace},
		gomock.Any(),
	).Return(nil).Do(func(ctx context.Context, key client.ObjectKey, found *certmanager.Certificate, opt ...any) {
		*found = certmanager.Certificate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Annotations: map[string]string{
					metadata.CertFileNameAnnotation:                   secretFileNames.CertFileName,
					metadata.CAFileNameAnnotation:                     secretFileNames.CAFileName,
					metadata.KeyFileNameAnnotation:                    secretFileNames.KeyFileName,
					metadata.TLSSecretRegisteredServiceNameAnnotation: serviceName,
					metadata.TLSSecretEntryHashAnnotation:             "",
					metadata.TLSSecretEntryIDAnnotation:               entryId,
					metadata.CertTypeAnnotation:                       "pem",
				},
			},
			Spec: certmanager.CertificateSpec{
				SecretName: secretName,
				CommonName: fmt.Sprintf("%s.%s", serviceName, namespace),
				DNSNames:   []string{"dns1", "dns2", strings.Join([]string{serviceName, namespace}, ".")},
				IssuerRef: cmmeta.ObjectReference{
					Name: "issuer",
					Kind: "Issuer",
				},
			},
		}
	})

	certType, err := secretstypes.StrToCertType("pem")
	s.Require().NoError(err)
	certConfig := secretstypes.CertConfig{CertType: certType, PEMConfig: secretFileNames}

	newEntryHash := "New-Hash"
	secretConf := secretstypes.NewSecretConfig(entryId, newEntryHash, secretName, namespace, serviceName, certConfig, false)

	s.client.EXPECT().Update(
		gomock.Any(),
		&CertificateMatcher{
			namespace:        namespace,
			name:             secretName,
			dnsNames:         []string{"dns1", "dns2", strings.Join([]string{serviceName, namespace}, ".")},
			serviceName:      serviceName,
			issuerName:       "issuer",
			useClusterIssuer: false,
			jks:              false,
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

	entryId, err := s.registry.RegisterK8SPod(context.Background(), namespace, "", serviceName, 0, []string{"dns1", "dns2"})
	s.Require().NoError(err)

	s.client.EXPECT().Get(
		gomock.Any(),
		types.NamespacedName{Name: secretName, Namespace: namespace},
		gomock.Any(),
	).Return(nil).Do(func(ctx context.Context, key client.ObjectKey, found *certmanager.Certificate, opt ...any) {
		*found = certmanager.Certificate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Annotations: map[string]string{
					metadata.CertFileNameAnnotation:                   secretFileNames.CertFileName,
					metadata.CAFileNameAnnotation:                     secretFileNames.CAFileName,
					metadata.KeyFileNameAnnotation:                    secretFileNames.KeyFileName,
					metadata.TLSSecretRegisteredServiceNameAnnotation: serviceName,
					metadata.TLSSecretEntryHashAnnotation:             "",
					metadata.TLSSecretEntryIDAnnotation:               entryId,
					metadata.CertTypeAnnotation:                       "jks",
				},
			},
			Spec: certmanager.CertificateSpec{
				SecretName: secretName,
				CommonName: fmt.Sprintf("%s.%s", serviceName, namespace),
				DNSNames:   []string{"dns1", "dns2", strings.Join([]string{serviceName, namespace}, ".")},
				IssuerRef: cmmeta.ObjectReference{
					Name: "issuer",
					Kind: "Issuer",
				},
				Keystores: &certmanager.CertificateKeystores{
					JKS: &certmanager.JKSKeystore{
						Create: true,
						PasswordSecretRef: cmmeta.SecretKeySelector{
							LocalObjectReference: cmmeta.LocalObjectReference{
								Name: JKSPasswordsSecretName,
							},
							Key: secretName,
						},
					},
				},
			},
		}
	})

	newCertType := "pem"
	certType, err := secretstypes.StrToCertType(newCertType)
	s.Require().NoError(err)
	certConfig := secretstypes.CertConfig{CertType: certType, PEMConfig: secretstypes.NewPEMConfig("", "", "")}
	secretConf := secretstypes.NewSecretConfig(entryId, "", secretName, namespace, serviceName, certConfig, false)

	s.client.EXPECT().Update(
		gomock.Any(),
		&CertificateMatcher{
			namespace:        namespace,
			name:             secretName,
			dnsNames:         []string{"dns1", "dns2", strings.Join([]string{serviceName, namespace}, ".")},
			serviceName:      serviceName,
			issuerName:       "issuer",
			useClusterIssuer: false,
			jks:              false,
		},
	).Return(nil)

	err = s.manager.EnsureTLSSecret(context.Background(), secretConf, nil)
	s.Require().NoError(err)
}

type JKSPasswordsSecretMatcher struct {
	namespace  string
	secretName string
	password   string
}

func (m *JKSPasswordsSecretMatcher) Matches(x interface{}) bool {
	secret, ok := x.(*corev1.Secret)
	if !ok {
		return false
	}

	if secret.Name != JKSPasswordsSecretName || secret.Namespace != m.namespace {
		return false
	}

	password := m.password
	if password == "" {
		password = "password"
	}
	if string(secret.Data[m.secretName]) != password {
		return false
	}
	return true
}

func (m *JKSPasswordsSecretMatcher) String() string {
	return fmt.Sprintf("JKSPasswordsSecretMatcher(name=%s, namespace=%s, field=%s)", JKSPasswordsSecretName, m.namespace, m.secretName)
}

func (s *ManagerSuite) TestManager_EnsureTLSSecret_ExistingSecretFound_UpdateNeeded_CertTypeChanged_PEM_To_JKS() {
	namespace := "test_namespace"
	secretName := "test_secretname"
	serviceName := "test_servicename"
	secretFileNames := secretstypes.NewPEMConfig("", "", "")

	entryId, err := s.registry.RegisterK8SPod(context.Background(), namespace, "", serviceName, 0, []string{"dns1", "dns2"})
	s.Require().NoError(err)

	s.client.EXPECT().Get(
		gomock.Any(),
		types.NamespacedName{Name: secretName, Namespace: namespace},
		gomock.Any(),
	).Return(nil).Do(func(ctx context.Context, key client.ObjectKey, found *certmanager.Certificate, opt ...any) {
		*found = certmanager.Certificate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Annotations: map[string]string{
					metadata.CertFileNameAnnotation:                   secretFileNames.CertFileName,
					metadata.CAFileNameAnnotation:                     secretFileNames.CAFileName,
					metadata.KeyFileNameAnnotation:                    secretFileNames.KeyFileName,
					metadata.TLSSecretRegisteredServiceNameAnnotation: serviceName,
					metadata.TLSSecretEntryHashAnnotation:             "",
					metadata.TLSSecretEntryIDAnnotation:               entryId,
					metadata.CertTypeAnnotation:                       "pem",
				},
			},
			Spec: certmanager.CertificateSpec{
				SecretName: secretName,
				CommonName: fmt.Sprintf("%s.%s", serviceName, namespace),
				DNSNames:   []string{"dns1", "dns2", strings.Join([]string{serviceName, namespace}, ".")},
				IssuerRef: cmmeta.ObjectReference{
					Name: "issuer",
					Kind: "Issuer",
				},
			},
		}
	})

	// Should get and update the jks passwords secret
	s.client.EXPECT().Get(
		gomock.Any(),
		types.NamespacedName{Name: JKSPasswordsSecretName, Namespace: namespace},
		gomock.Any(),
	).Return(nil).Do(func(ctx context.Context, key client.ObjectKey, found *corev1.Secret, opt ...any) {
		*found = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      JKSPasswordsSecretName,
				Namespace: namespace,
			},
			Data: map[string][]byte{},
		}
	})

	s.client.EXPECT().Update(
		gomock.Any(),
		&JKSPasswordsSecretMatcher{
			namespace:  namespace,
			secretName: secretName,
			password:   "",
		},
	).Return(nil)

	newCertType := "jks"
	certType, err := secretstypes.StrToCertType(newCertType)
	s.Require().NoError(err)
	certConfig := secretstypes.CertConfig{CertType: certType, JKSConfig: secretstypes.NewJKSConfig("", "", "")}
	secretConf := secretstypes.NewSecretConfig(entryId, "", secretName, namespace, serviceName, certConfig, false)

	s.client.EXPECT().Update(
		gomock.Any(),
		&CertificateMatcher{
			namespace:        namespace,
			name:             secretName,
			dnsNames:         []string{"dns1", "dns2", strings.Join([]string{serviceName, namespace}, ".")},
			serviceName:      serviceName,
			issuerName:       "issuer",
			useClusterIssuer: false,
			jks:              true,
		},
	).Return(nil)

	err = s.manager.EnsureTLSSecret(context.Background(), secretConf, nil)
	s.Require().NoError(err)
}

// TODO: Test RefreshTLSSecrets once its implemented for CertManagerSecretsManager

func TestRunManagerSuite(t *testing.T) {
	suite.Run(t, new(ManagerSuite))
}
