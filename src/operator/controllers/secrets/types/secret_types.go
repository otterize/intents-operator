package secretstypes

import (
	"context"
	"fmt"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"strings"
)

type SecretType string
type CertType string

const (
	TlsSecretType = SecretType("TLS")
	JKSCertType   = CertType("jks")
	PEMCertType   = CertType("pem")
)

func StrToCertType(strCertType string) (CertType, error) {
	switch CertType(strings.ToLower(strCertType)) {
	case JKSCertType:
		return JKSCertType, nil
	case PEMCertType:
		return PEMCertType, nil
	default:
		return "", fmt.Errorf("certificate type: %s is not a valid certificate type. valid types: [jks pem]", strCertType)
	}
}

type CertificateData struct {
	Files     map[string][]byte
	ExpiryStr string
}

type PEMConfig struct {
	CertFileName string
	CAFileName   string
	KeyFileName  string
}

func NewPEMConfig(certFileName string, caFileName string, keyFileName string) PEMConfig {
	newFileNames := PEMConfig{}
	newFileNames.CertFileName, _ = lo.Coalesce(certFileName, "cert.pem")
	newFileNames.KeyFileName, _ = lo.Coalesce(keyFileName, "key.pem")
	newFileNames.CAFileName, _ = lo.Coalesce(caFileName, "ca.pem")
	return newFileNames
}

type JKSConfig struct {
	KeyStoreFileName   string
	TrustStoreFileName string
	Password           string
}

func NewJKSConfig(keystoreFileName string, truststoreFileName string, password string) JKSConfig {
	newFileNames := JKSConfig{}
	newFileNames.KeyStoreFileName, _ = lo.Coalesce(keystoreFileName, "keystore.jks")
	newFileNames.TrustStoreFileName, _ = lo.Coalesce(truststoreFileName, "truststore.jks")
	newFileNames.Password, _ = lo.Coalesce(password, "password")
	return newFileNames
}

type JKSCert struct {
	TrustStore []byte
	KeyStore   []byte
	Expiry     string
}

type PEMCert struct {
	Certificate []byte
	CA          []byte
	Key         []byte
	Expiry      string
}

type CertConfig struct {
	CertType  CertType
	JKSConfig JKSConfig
	PEMConfig PEMConfig
}

type SecretConfig struct {
	EntryID                   string
	EntryHash                 string
	SecretName                string
	Namespace                 string
	ServiceName               string
	ShouldRestartPodOnRenewal bool
	CertConfig                CertConfig
}

func NewSecretConfig(entryID string, entryHash string, secretName string, namespace string, serviceName string, certConfig CertConfig, ShouldRestartPodOnRenewal bool) SecretConfig {
	return SecretConfig{
		EntryID:                   entryID,
		EntryHash:                 entryHash,
		SecretName:                secretName,
		Namespace:                 namespace,
		ServiceName:               serviceName,
		CertConfig:                certConfig,
		ShouldRestartPodOnRenewal: ShouldRestartPodOnRenewal,
	}
}

type CertificateDataGenerator interface {
	GenerateJKS(ctx context.Context, entryID string, password string) (JKSCert, error)
	GeneratePEM(ctx context.Context, entryID string) (PEMCert, error)
}

type PodOwnerIdentifier struct {
	Name             string
	GroupVersionKind schema.GroupVersionKind
}
