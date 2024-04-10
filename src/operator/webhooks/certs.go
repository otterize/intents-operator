package webhooks

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/otterize/intents-operator/src/shared"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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

const ngrokWebhookModeKey = "ngrok-webhook-mode"
const ngrokWebhookModeDefault = false
const ngrokTunnelLookupPortKey = "ngrok-tunnel-lookup-port"
const ngrokTunnelLookupPortDefault = 4040

func init() {
	viper.SetDefault(ngrokWebhookModeKey, ngrokWebhookModeDefault)
	viper.SetDefault(ngrokTunnelLookupPortKey, ngrokTunnelLookupPortDefault)
}

func GetAdditionalDebuggingWebhookDNSNames() []string {
	if !viper.GetBool(ngrokWebhookModeKey) {
		return make([]string, 0)
	}

	// Get public URL from ngrok local API.
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	req := shared.MustRet(http.NewRequestWithContext(timeoutCtx, http.MethodGet,
		fmt.Sprintf("http://localhost:%d/api/tunnels", viper.GetInt(ngrokTunnelLookupPortKey)), nil))
	resp := shared.MustRet(http.DefaultClient.Do(req))
	if resp.StatusCode != http.StatusOK {
		logrus.Panic("failed to get response from ngrok")
	}
	defer resp.Body.Close()

	var response struct {
		Tunnels []struct {
			PublicURL string `json:"public_url"`
		}
	}

	dec := json.NewDecoder(resp.Body)
	shared.Must(dec.Decode(&response))

	if len(response.Tunnels) != 1 {
		logrus.Panic("can only work with 1 ngrok tunnel")
	}

	parsedURLWithPort := shared.MustRet(url.Parse(response.Tunnels[0].PublicURL))
	parsedURL := strings.Split(parsedURLWithPort.Host, ":")[0]

	return []string{parsedURL}
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
	certificate.DNSNames = append(certificate.DNSNames, GetAdditionalDebuggingWebhookDNSNames()...)
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

func UpdateMutationWebHookCA(ctx context.Context, webHookName string, ca []byte) error {
	kubeClient, err := getKubeClient()
	if err != nil {
		return errors.Wrap(err)
	}

	webhookConfig, err := kubeClient.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(ctx, webHookName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err)
	}

	var newCA []patchValue
	for i := range webhookConfig.Webhooks {
		newCA = append(newCA, patchValue{
			Op:    "replace",
			Path:  fmt.Sprintf("/webhooks/%d/clientConfig/caBundle", i),
			Value: base64.StdEncoding.EncodeToString(ca),
		})
	}

	newCAByte, err := json.Marshal(newCA)
	if err != nil {
		return errors.Wrap(err)
	}

	logrus.Infof("Installing new certificate in %s", webHookName)
	_, err = kubeClient.AdmissionregistrationV1().MutatingWebhookConfigurations().Patch(ctx, webHookName, types.JSONPatchType, newCAByte, metav1.PatchOptions{})
	return errors.Wrap(err)
}

func getKubeClient() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, errors.Wrap(err)
	}
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	return clientSet, nil
}

type patchValue struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}
