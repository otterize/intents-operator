package otterizecertgen

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"github.com/otterize/credentials-operator/src/controllers/otterizeclient/otterizegraphql"
	secretstypes "github.com/otterize/credentials-operator/src/controllers/secrets/types"
	mock_otterizecertgen "github.com/otterize/credentials-operator/src/mocks/otteirzecertgen"
	"github.com/pavlo-v-chernykh/keystore-go/v4"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"sort"
	"testing"
	"time"
)

const ExpiryTimeTestStr = "06/24/94"
const ExpiryTimeTestLayout = "01/02/06"
const CertPEM = `-----BEGIN CERTIFICATE-----
MIIDaTCCAlGgAwIBAgIUM/JtrXsdK8eSPOz459c92UBlMyEwDQYJKoZIhvcNAQEL
BQAwMjEwMC4GA1UEAwwnZW52X3g2c29yNXpqZmIgT3R0ZXJpemUgSW50ZXJtZWRp
YXRlIENBMB4XDTIyMTIyNTE0MTQwM1oXDTIzMDEyNjE0MTQzM1owGjEYMBYGA1UE
AxMPb21yaS54NnNvcjV6amZiMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKC
AQEAyFL5TW2uofao/khK5GC/kBqXksUEjgwY2CHqIm0GlzB2QGGdQ+vNu6s8UdkZ
cHh3/VQ2zWwIE0gbMq1qOa2GrliAmO4mb4xzgUaOGFi2/PFZ/luSre4FcGIUYY8p
WpY2W+VUqhNO3DLddFeqp68jBWiQC1Lw0kl60LaoMQPg/x4PqmkLlGKVfnS9GlIT
N+QTGtz+nhI0/rSFyklIpaabsY5UruRlYPh5Acm+b5ubtuZi+yk4VQgvO5xduprF
+2KZk+dIjNpTTcn/ZP2Uy5jtt5VlY5Gm/GQR+JID5XSDH7hg0Rl7RL7U4X5TPmtD
qQN9qHgsasVMMlm2KKasqwNaTQIDAQABo4GOMIGLMA4GA1UdDwEB/wQEAwIDqDAd
BgNVHSUEFjAUBggrBgEFBQcDAQYIKwYBBQUHAwIwHQYDVR0OBBYEFJOwawAfuVV8
XzWjb4qJGcOm5BWuMB8GA1UdIwQYMBaAFIk+69UKd2/UXs64ceHdsHbVLAAGMBoG
A1UdEQQTMBGCD29tcmkueDZzb3I1empmYjANBgkqhkiG9w0BAQsFAAOCAQEAS9rx
yeWvhUPL3+YXYebE93bu91yjjbNwsLEN+bhqJV+scFui253SO7MeOwW3P+9N443R
Hv145KoLvHzGVF6MLG+layPHLOmM+leuDztzooXXzkOCn6BAk2EeMXiPi+WjZabX
cIJOVnbFDm5qlUNFSQtFWOFRlKnYKvWmlMpH8Z2WdHz/TDPsYLugHLz8wx0GQGyb
3WKSNiuVrXiOCOZsavWE8+VIO9OZSY93DPm/qqi2JpTlSLXUwR1q9ph6iDvSvUW0
D9DsQGiG+0XeE/YzQEQnJqo5n14UfHQZlTDSjQ1ZL+E0qtGm39a1z+VrYCMLD5R/
CnN7cxW6JlS+D3jRLg==
-----END CERTIFICATE-----`
const CAPEM = `-----BEGIN CERTIFICATE-----
MIIDPjCCAiagAwIBAgIUL8mw/WOTNMdbazNHetE0qOee+owwDQYJKoZIhvcNAQEL
BQAwGzEZMBcGA1UEAxMQT3R0ZXJpemUgUm9vdCBDQTAeFw0yMjEyMTkxODAzMzNa
Fw0yMzEyMTkxODA0MDNaMDIxMDAuBgNVBAMMJ2Vudl94NnNvcjV6amZiIE90dGVy
aXplIEludGVybWVkaWF0ZSBDQTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoC
ggEBALilbL5n0P3Lf7kw6Vsr1sX/vSJq804DNfl8r/efufQJiSNBo7cAf0Sc1gNi
+B0CXVKe0B80BdOH0w3Rx60524igI6wVzwzDfDFZ/yKqRDzC+OUDtJw6Y04USAJC
AFVhU+1Z+udBagcI/MUDm9PcU4YQu8hb3oLEnBBDT78uzIZcxBeaiZkRWBm1ONtN
x/latAIQJifkKYnBuQx1yE+32cMAL8lzhfHJ864Vyk633mEGqMerTP99NVK6qUKR
UB+zuLPzS3lIFIM1b+bAxtIi2UFtAQVAnPS+z4gjuc4OCpvit4HiFzk/lX6hEiMm
XmCQ1uJdVUngyixyLlvMDCGVqQ0CAwEAAaNjMGEwDgYDVR0PAQH/BAQDAgEGMA8G
A1UdEwEB/wQFMAMBAf8wHQYDVR0OBBYEFIk+69UKd2/UXs64ceHdsHbVLAAGMB8G
A1UdIwQYMBaAFFP7/BGbGDdM3XmCjb6RZzVggGukMA0GCSqGSIb3DQEBCwUAA4IB
AQADog3aT0/MhfjKU17emC059ttHLTo3WMXcwbHgAgEj7CAsW6ZnIYiuDHpRLpkQ
vvcEeHNITS2XdUGAtJ6LgQiBc49RFFvE5U11tQrZIq2+4V9wTaKqsj9utBbRJkOP
i3dJ0LHJA7BY6A+zbDy1xURpU8etepFQAZ5/BirGFIK9Bp2Vy68TYkjeHBv4IFmi
JOOQLsi+bBDSRZbsd7MT14EECAbssKxebs5HPiCxkUdNiYyUnG/AonEXO9zq4yvI
PAsoRGfsd4XqwO3w9jd+haF2di5csdEpBtfmDEJ9IWwM/Iex4cHOKsTtO+d0lEUr
AHdKR0EbLY/Z5cdWPF9pvzbf
-----END CERTIFICATE-----`
const RootCAPEM = `-----BEGIN CERTIFICATE-----
MIIDJzCCAg+gAwIBAgIUUhS5xfGUmqM7/icsXxqQca9n2hYwDQYJKoZIhvcNAQEL
BQAwGzEZMBcGA1UEAxMQT3R0ZXJpemUgUm9vdCBDQTAeFw0yMjEyMTkxODAzMzNa
Fw0yMzEyMTkxODA0MDJaMBsxGTAXBgNVBAMTEE90dGVyaXplIFJvb3QgQ0EwggEi
MA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDHcre+nqg0Esu5pFvsfou39F5y
1AQQz7XuGASIekNCHH2+WewoSWN91VeaOuSkrbkys8eyL7RyR5mE5zCX2W2OG+7f
IaLkXw2E2HC9HKQre2y0uqNFo0hJoeNFkpni11qaHLhAltZ3STKsKwLhVkVKgdQ8
gtGaaO0aI72KPZvjxNAmTH1xPt3izlhLQE50/47vhtdtF4TqCp5634PKgNo5RytS
zSpasiaeaDNrx/R2LyRgz4IXkadtFj5n6hdZtuFBEIPAPdRLv1RdlAfPtzMzjXah
nQ3ghSenA76+i+pekFNxyxklli9zcxlnsJGQvCWEX5nOUaP4a6+QHQR5nVPpAgMB
AAGjYzBhMA4GA1UdDwEB/wQEAwIBBjAPBgNVHRMBAf8EBTADAQH/MB0GA1UdDgQW
BBRT+/wRmxg3TN15go2+kWc1YIBrpDAfBgNVHSMEGDAWgBRT+/wRmxg3TN15go2+
kWc1YIBrpDANBgkqhkiG9w0BAQsFAAOCAQEAGLk39YhoYI6a3MinqK+0zOmSo/25
jTYI3O3n8EOV71ZHxHYlsukdzTbtikcvkrQ2eLP+OwS7JavP9qvLc08Z/i9W8Djy
cO93udeqcU7mCsyqsKYojns+buEJW8RIe4lurtvjWQmHfFfcGStqWaZD+nEnUkVB
cyvxLjRYBj78/xjmO5Q84cZRBeBBZ16HPY/P8k6CUuW0QrKnUTEAxwlgg3+22JI7
xTgZufItS3GF0F1q7lMG8z2YdMi6s4J3AEs/XW2viuiMI9asv7+lKJE5NKS7vy4e
M3xV5JVb/TF4bs9TElQm9ZRVcPXgxYhPWSpBfkzoKdhVWFVaXeO3pqiVpQ==
-----END CERTIFICATE-----`
const KeyPEM = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAyFL5TW2uofao/khK5GC/kBqXksUEjgwY2CHqIm0GlzB2QGGd
Q+vNu6s8UdkZcHh3/VQ2zWwIE0gbMq1qOa2GrliAmO4mb4xzgUaOGFi2/PFZ/luS
re4FcGIUYY8pWpY2W+VUqhNO3DLddFeqp68jBWiQC1Lw0kl60LaoMQPg/x4PqmkL
lGKVfnS9GlITN+QTGtz+nhI0/rSFyklIpaabsY5UruRlYPh5Acm+b5ubtuZi+yk4
VQgvO5xduprF+2KZk+dIjNpTTcn/ZP2Uy5jtt5VlY5Gm/GQR+JID5XSDH7hg0Rl7
RL7U4X5TPmtDqQN9qHgsasVMMlm2KKasqwNaTQIDAQABAoIBAFv8rfn+GajJ6UQK
0kkYnB6B94Qv8C2CJI5q1GbGhbY7TLG3oU2lJC2/Lc2v0VyyFPdBCoE90F96RvL4
asTdh/DbNwICqaejaQ695VYMtspj0Z1ZU3uGxvyaLR23bZfpTkDYiA4pG5dFzCc2
cmjZpU1AfJSWm3sUvs7EcWtAirratP4raPngsL0UbOMtVx30gZ5poG82UCzt5eiP
yQg0CgCCxCI9CYp0nBGkAK++iSvekDbc9TI49RqrAB5kTlKULsO65JkBvQ65SBDW
mDNIXUpvmCA+MYE7QR4wbYjb4z8lWCg5lL9dtUngrKlwbysoCJKFKI9M+EHPAfaS
lEgkqIECgYEA5KFhXE691H+NUKE4obz/WgJNUs8VtqGhwXb2B07BgKFzrNKgOvMU
3tCEwkDb5lYpNS4Ji1WCWTyG0DqNGSy6T7A28HUrUgXAmE4qm3eVbuPYaEmc5OFg
kFmAw7oURBAwZNV0fW/KdpcVZwsieXlnpmjrFr09rePWdgPB9xfh5v0CgYEA4E4e
KqRXvwYtAjYDhw8ZmqsR1UesNni4KeQt9yTvQs0qwwoKDMvaQezOntDBtVLKUADE
LWwEA0FQ+USa6fIb4gplq3EaW9bt1ZcDn5EWOOU4bs2UlXelHP34xVJxjEriwao0
Ku0cEmoj3GB/G7er3qTz+OeD6cnQlAYGy/mlKZECgYAFBga9oH1LTgIs4137L8vs
jmBkkWhIuwRy28pMHs7hpKqGAZrDsNOkkbBZFFPAm+QL5xcOmLJkg4/yw1aWVwVA
+v46ClkJVFcHAbCt+dKuvRLkN7nazZjxkwXhRxVq6XAmxwnoN6ybLnap7PS09pXw
ch24QjA4wejUbwC0DTJJgQKBgQDEgcuN4hJ5aOivgjCO9xygUvS5nCP0SMhW8u+O
EE6IgIPRAQ+S7FiW3uaZXhwGRbS2aCV2AaZ2T5en+YGaKSBiZGdzzg+gm+ga8kUb
WxlT2QUalYJxe7MsdhemjzapCMYlkn5HiRjJzTEDlYpl9wBceri+u9zmSYcw1yLH
OjuG8QKBgFwzDAAwgQHslUkTSDGSssi8WaXu8oDbvzW5gGszNi+97e5LESRs3VJt
A5dLkq44R1B3jxlI314zv8MNIw7+1wE/dqBYf7FJvNvAADERLcwVSmCEXRXrrATC
KXvnWbl1K899XqzQ2b8u2OvbveY17g+K6N9D/u9Op0iMWsdpWnBu
-----END RSA PRIVATE KEY-----`

type ManagerSuite struct {
	suite.Suite
	controller    *gomock.Controller
	otterizeCloud *mock_otterizecertgen.MockOtterizeCloudClient
	certGenerator *OtterizeCertificateDataGenerator
}

func (s *ManagerSuite) SetupTest() {
	s.controller = gomock.NewController(s.T())
	s.otterizeCloud = mock_otterizecertgen.NewMockOtterizeCloudClient(s.controller)
	s.certGenerator = NewOtterizeCertificateGenerator(s.otterizeCloud)

}

func (s *ManagerSuite) mockGetTLSKeyPair(entryId string) {

	expiry, err := time.Parse(ExpiryTimeTestLayout, ExpiryTimeTestStr)
	s.Require().NoError(err)
	s.otterizeCloud.EXPECT().GetTLSKeyPair(gomock.Any(), entryId).
		Return(otterizegraphql.TLSKeyPair{
			ExpiresAt: int(expiry.Unix()),
			CertPEM:   CertPEM,
			CaPEM:     CAPEM,
			RootCAPEM: RootCAPEM,
			KeyPEM:    KeyPEM,
		}, nil)
}

func (s *ManagerSuite) mockGetTLSKeyPair_NoIntCAPEM(entryId string) {

	expiry, err := time.Parse(ExpiryTimeTestLayout, ExpiryTimeTestStr)
	s.Require().NoError(err)
	s.otterizeCloud.EXPECT().GetTLSKeyPair(gomock.Any(), entryId).
		Return(otterizegraphql.TLSKeyPair{
			ExpiresAt: int(expiry.Unix()),
			CertPEM:   CertPEM,
			RootCAPEM: RootCAPEM,
			KeyPEM:    KeyPEM,
		}, nil)
}

func (s *ManagerSuite) TestCertGenerator_GeneratePEM() {

	entryId := "/test"

	s.mockGetTLSKeyPair(entryId)

	certPEM, err := s.certGenerator.GeneratePEM(context.Background(), entryId)
	s.Require().NoError(err)
	expiry, err := time.Parse(ExpiryTimeTestLayout, ExpiryTimeTestStr)
	s.Require().NoError(err)
	expiryUnix := time.Unix(expiry.Unix(), 0)
	expectedCertData := secretstypes.PEMCert{
		CA:          bytes.Join([][]byte{[]byte(CAPEM), []byte(RootCAPEM)}, []byte("\n")),
		Key:         []byte(KeyPEM),
		Certificate: []byte(CertPEM),
		Expiry:      expiryUnix.Format(time.RFC3339),
	}
	s.Equal(expectedCertData, certPEM)
}

func (s *ManagerSuite) TestCertGenerator_GeneratePEM_EmptyIntCAPEM() {

	entryId := "/test"

	s.mockGetTLSKeyPair_NoIntCAPEM(entryId)

	certPEM, err := s.certGenerator.GeneratePEM(context.Background(), entryId)
	s.Require().NoError(err)
	expiry, err := time.Parse(ExpiryTimeTestLayout, ExpiryTimeTestStr)
	s.Require().NoError(err)
	expiryUnix := time.Unix(expiry.Unix(), 0)
	expectedCertData := secretstypes.PEMCert{
		CA:          []byte(RootCAPEM),
		Key:         []byte(KeyPEM),
		Certificate: []byte(CertPEM),
		Expiry:      expiryUnix.Format(time.RFC3339),
	}
	s.Equal(expectedCertData, certPEM)
}

func (s *ManagerSuite) TestCertGenerator_GenerateJKS() {
	entryId := "/test"
	password := []byte("password")
	s.mockGetTLSKeyPair(entryId)
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
	s.Require().Equal(len(ts.Aliases()), 2)
	caAliases := ts.Aliases()
	sort.Strings(caAliases)
	caAlias := caAliases[0]
	ca, err := ts.GetTrustedCertificateEntry(caAlias)
	s.Require().NoError(err)
	s.Require().Equal([]byte(CAPEM), ca.Certificate.Content)

	caAlias = caAliases[1]
	ca, err = ts.GetTrustedCertificateEntry(caAlias)
	s.Require().NoError(err)
	s.Require().Equal([]byte(RootCAPEM), ca.Certificate.Content)

	// test keystore is as expected
	ks := keystore.New()
	keyStoreReader := bytes.NewReader(certJKS.KeyStore)
	err = ks.Load(keyStoreReader, password)
	s.Require().NoError(err)
	s.Require().Equal(len(ks.Aliases()), 1)
	pkey, err := ks.GetPrivateKeyEntry("pkey", password)
	s.Require().NoError(err)
	// compare pkey
	decodedPkey, _ := pem.Decode([]byte(KeyPEM))
	pkc1Pk, err := x509.ParsePKCS1PrivateKey(decodedPkey.Bytes)
	s.Require().NoError(err)
	pkBytes, err := x509.MarshalPKCS8PrivateKey(pkc1Pk)
	s.Require().NoError(err)
	s.Require().Equal(pkBytes, pkey.PrivateKey)
	// compare certificate chain
	certChain := []string{CertPEM, CAPEM, RootCAPEM}
	// remove padding in order to compare
	s.Require().Equal([]byte(certChain[0]), pkey.CertificateChain[0].Content)
	s.Require().Equal([]byte(certChain[1]), pkey.CertificateChain[1].Content)
	s.Require().Equal([]byte(certChain[2]), pkey.CertificateChain[2].Content)

}

func (s *ManagerSuite) TestCertGenerator_GenerateJKS_EmptyIntCAPEM() {
	entryId := "/test"
	password := []byte("password")
	s.mockGetTLSKeyPair_NoIntCAPEM(entryId)
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
	caAliases := ts.Aliases()
	sort.Strings(caAliases)
	caAlias := caAliases[0]
	ca, err := ts.GetTrustedCertificateEntry(caAlias)
	s.Require().NoError(err)
	s.Require().Equal([]byte(RootCAPEM), ca.Certificate.Content)

	// test keystore is as expected
	ks := keystore.New()
	keyStoreReader := bytes.NewReader(certJKS.KeyStore)
	err = ks.Load(keyStoreReader, password)
	s.Require().NoError(err)
	s.Require().Equal(len(ks.Aliases()), 1)
	pkey, err := ks.GetPrivateKeyEntry("pkey", password)
	s.Require().NoError(err)
	// compare pkey
	decodedPkey, _ := pem.Decode([]byte(KeyPEM))
	pkc1Pk, err := x509.ParsePKCS1PrivateKey(decodedPkey.Bytes)
	s.Require().NoError(err)
	pkBytes, err := x509.MarshalPKCS8PrivateKey(pkc1Pk)
	s.Require().NoError(err)
	s.Require().Equal(pkBytes, pkey.PrivateKey)
	// compare certificate chain
	certChain := []string{CertPEM, RootCAPEM}
	// remove padding in order to compare
	s.Require().Equal([]byte(certChain[0]), pkey.CertificateChain[0].Content)
	s.Require().Equal([]byte(certChain[1]), pkey.CertificateChain[1].Content)

}

func TestRunManagerSuite(t *testing.T) {
	suite.Run(t, new(ManagerSuite))
}
