package jks

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/pavlo-v-chernykh/keystore-go/v4"
	"github.com/samber/lo"
	"time"
)

func ByteDumpKeyStore(trustStore keystore.KeyStore, password string) ([]byte, error) {
	trustStoreBytesBuffer := new(bytes.Buffer)
	err := trustStore.Store(trustStoreBytesBuffer, []byte(password))
	if err != nil {
		return nil, errors.Wrap(err)
	}
	return trustStoreBytesBuffer.Bytes(), nil
}

func CASliceToTrustStore(CAPEMSlice [][]byte) (keystore.KeyStore, error) {
	trustStore := keystore.New()
	for i, caPEM := range CAPEMSlice {
		ca := keystore.TrustedCertificateEntry{
			CreationTime: time.Now(),
			Certificate: keystore.Certificate{
				Type:    "X509",
				Content: caPEM,
			},
		}
		err := trustStore.SetTrustedCertificateEntry(fmt.Sprintf("ca-%d", i), ca)
		if err != nil {
			return keystore.KeyStore{}, errors.Wrap(err)
		}
	}
	return trustStore, nil
}

func PemToKeyStore(certificatesChainPEM [][]byte, keyPem []byte, password string) (keystore.KeyStore, error) {
	certChain := lo.Map(certificatesChainPEM, func(certPEM []byte, _ int) keystore.Certificate {
		return keystore.Certificate{
			Type:    "X509",
			Content: certPEM,
		}
	})

	block, _ := pem.Decode(keyPem)
	keyStore := keystore.New()
	if block == nil {
		return keystore.KeyStore{}, errors.New("error decoding private key PEM block")
	}
	var key any
	var err error
	switch block.Type {
	case "PRIVATE KEY":
		key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return keystore.KeyStore{}, errors.Wrap(err)
		}
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return keystore.KeyStore{}, errors.Wrap(err)
		}
	default:
		return keystore.KeyStore{}, errors.Errorf("unsupprted block type for private key: %s", block.Type)

	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return keystore.KeyStore{}, errors.Wrap(err)
	}

	pk := keystore.PrivateKeyEntry{
		CreationTime:     time.Now(),
		PrivateKey:       keyDER,
		CertificateChain: certChain,
	}
	err = keyStore.SetPrivateKeyEntry("pkey", pk, []byte(password))
	if err != nil {
		return keystore.KeyStore{}, errors.Wrap(err)
	}
	return keyStore, nil
}
