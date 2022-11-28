package spirecertgen

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/otterize/spire-integration-operator/src/controllers/spireclient/bundles"
	"github.com/otterize/spire-integration-operator/src/controllers/spireclient/svids"
	"github.com/pavlo-v-chernykh/keystore-go/v4"
	"github.com/samber/lo"
	"time"
)

const certificateEnd = "-----END CERTIFICATE-----\n"

func svidToKeyStore(svid svids.EncodedX509SVID, password string) ([]byte, error) {
	notEmptyCertPEMs := lo.Filter(bytes.SplitAfter(svid.SVIDPEM, []byte(certificateEnd)), func(pem []byte, _ int) bool { return len(pem) != 0 })
	certChain := lo.Map(notEmptyCertPEMs, func(certPEM []byte, _ int) keystore.Certificate {
		return keystore.Certificate{
			Type:    "X509",
			Content: certPEM,
		}
	})

	keyStore := keystore.New()
	block, _ := pem.Decode(svid.KeyPEM)
	if block == nil {
		return nil, errors.New("error decoding private key PEM block")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}

	pk := keystore.PrivateKeyEntry{
		CreationTime:     time.Now(),
		PrivateKey:       keyDER,
		CertificateChain: certChain,
	}
	err = keyStore.SetPrivateKeyEntry("pkey", pk, []byte(password))
	if err != nil {
		return nil, err
	}
	keyStoreBytesBuffer := new(bytes.Buffer)
	err = keyStore.Store(keyStoreBytesBuffer, []byte(password))
	if err != nil {
		return nil, err
	}
	return keyStoreBytesBuffer.Bytes(), nil
}

func trustBundleToTrustStore(trustBundle bundles.EncodedTrustBundle, password string) ([]byte, error) {
	trustStore := keystore.New()
	notEmptyCaPEMs := lo.Filter(bytes.SplitAfter(trustBundle.BundlePEM, []byte(certificateEnd)), func(pem []byte, _ int) bool { return len(pem) != 0 })
	for i, caPEM := range notEmptyCaPEMs {
		ca := keystore.TrustedCertificateEntry{
			CreationTime: time.Now(),
			Certificate: keystore.Certificate{
				Type:    "X509",
				Content: caPEM,
			},
		}
		err := trustStore.SetTrustedCertificateEntry(fmt.Sprintf("ca-%d", i), ca)
		if err != nil {
			return nil, err
		}
	}
	trustStoreBytesBuffer := new(bytes.Buffer)
	err := trustStore.Store(trustStoreBytesBuffer, []byte(password))
	if err != nil {
		return nil, err
	}
	return trustStoreBytesBuffer.Bytes(), nil
}
