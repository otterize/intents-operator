package webhooks

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"github.com/otterize/intents-operator/src/shared/errors"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

const (
	Year               = 365 * 24 * time.Hour
	CertDirPath        = "/tmp/k8s-webhook-server/serving-certs"
	CertFilename       = "tls.crt"
	PrivateKeyFilename = "tls.key"
)

type CertificateBundle struct {
	CertPem       []byte
	PrivateKeyPem []byte
}

func GenerateSelfSignedCertificate(hostname string, namespace string) (CertificateBundle, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return CertificateBundle{}, errors.Wrap(err)
	}
	certificate := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject: pkix.Name{
			CommonName: hostname,
		},
		NotBefore: time.Now().Add(-10 * time.Minute),
		NotAfter:  time.Now().Add(10 * Year),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{hostname, fmt.Sprintf("%s.%s.svc", hostname, namespace), fmt.Sprintf("%s.%s.svc.cluster.local", hostname, namespace)},
	}
	derCert, err := x509.CreateCertificate(rand.Reader, &certificate, &certificate, &privateKey.PublicKey, privateKey)
	if err != nil {
		return CertificateBundle{}, errors.Wrap(err)
	}

	signedCert := &bytes.Buffer{}
	err = pem.Encode(signedCert, &pem.Block{Type: "CERTIFICATE", Bytes: derCert})
	if err != nil {
		return CertificateBundle{}, errors.Wrap(err)
	}

	privateKeyPem := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	return CertificateBundle{
		CertPem:       signedCert.Bytes(),
		PrivateKeyPem: privateKeyPem,
	}, nil
}

func WriteCertToFiles(bundle CertificateBundle) error {
	err := os.MkdirAll(CertDirPath, 0750)
	if err != nil {
		return errors.Wrap(err)
	}

	certFilePath := filepath.Join(CertDirPath, CertFilename)
	privateKeyFilePath := filepath.Join(CertDirPath, PrivateKeyFilename)

	err = os.WriteFile(certFilePath, bundle.CertPem, 0600)
	if err != nil {
		return errors.Wrap(err)
	}
	return os.WriteFile(privateKeyFilePath, bundle.PrivateKeyPem, 0600)
}
