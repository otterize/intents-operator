package spirecertgen

import (
	"bytes"
	"github.com/otterize/spire-integration-operator/src/controllers/certificates/jks"
	"github.com/otterize/spire-integration-operator/src/controllers/spireclient/bundles"
	"github.com/otterize/spire-integration-operator/src/controllers/spireclient/svids"
	"github.com/samber/lo"
)

const certificateEnd = "-----END CERTIFICATE-----\n"

func svidToKeyStore(svid svids.EncodedX509SVID, password string) ([]byte, error) {
	notEmptyCertPEMs := lo.Filter(bytes.SplitAfter(svid.SVIDPEM, []byte(certificateEnd)), func(pem []byte, _ int) bool { return len(pem) != 0 })
	keyPem := svid.KeyPEM
	keyStore, err := jks.PemToKeyStore(notEmptyCertPEMs, keyPem, password)
	if err != nil {
		return nil, err
	}
	return jks.ByteDumpKeyStore(keyStore, password)
}

func trustBundleToTrustStore(trustBundle bundles.EncodedTrustBundle, password string) ([]byte, error) {
	notEmptyCaPEMs := lo.Filter(bytes.SplitAfter(trustBundle.BundlePEM, []byte(certificateEnd)), func(pem []byte, _ int) bool { return len(pem) != 0 })
	trustStore, err := jks.CASliceToTrustStore(notEmptyCaPEMs)
	if err != nil {
		return nil, err
	}
	return jks.ByteDumpKeyStore(trustStore, password)
}
