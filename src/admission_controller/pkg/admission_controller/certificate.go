package admission_controller

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
	"github.com/sirupsen/logrus"
	"io/ioutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"math/big"
	"time"
)

const year = 365 * 24 * time.Hour

type certificateBundle struct {
	certPem       []byte
	privateKeyPem []byte
}

func generateSelfSignedCertificate(hostname string, namespace string) (certificateBundle, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return certificateBundle{}, err
	}
	certificate := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject: pkix.Name{
			CommonName: hostname,
		},
		NotBefore: time.Now().Add(-10 * time.Minute),
		NotAfter:  time.Now().Add(10 * year),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{hostname, fmt.Sprintf("%s.%s.svc", hostname, namespace), fmt.Sprintf("%s.%s.svc.cluster.local", hostname, namespace)},
	}
	derCert, err := x509.CreateCertificate(rand.Reader, &certificate, &certificate, &privateKey.PublicKey, privateKey)
	if err != nil {
		return certificateBundle{}, err
	}

	signedCert := &bytes.Buffer{}
	err = pem.Encode(signedCert, &pem.Block{Type: "CERTIFICATE", Bytes: derCert})
	if err != nil {
		return certificateBundle{}, err
	}

	privateKeyPem := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	return certificateBundle{
		certPem:       signedCert.Bytes(),
		privateKeyPem: privateKeyPem,
	}, nil
}

func writeCertToFiles(bundle certificateBundle, certPath string, keyPath string) error {
	err := ioutil.WriteFile(certPath, bundle.certPem, 0600)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(keyPath, bundle.privateKeyPem, 0600)
	return err
}

type patchValue struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

func getKubeClient() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientSet, nil
}

func updateWebHookCA(ctx context.Context, kubeClient *kubernetes.Clientset, webHookName string, ca []byte) error {
	// Make sure the webhook exists
	_, err := kubeClient.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(ctx, webHookName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	newCA := []patchValue{{
		Op:    "replace",
		Path:  "/webhooks/0/clientConfig/caBundle",
		Value: base64.StdEncoding.EncodeToString(ca),
	},
	}
	newCAByte, err := json.Marshal(newCA)
	if err != nil {
		return err
	}

	logrus.Infof("Installing new certificate in %s mutating webhook configuration", webHookName)
	_, err = kubeClient.AdmissionregistrationV1().MutatingWebhookConfigurations().Patch(ctx, webHookName, types.JSONPatchType, newCAByte, metav1.PatchOptions{})
	return err
}
