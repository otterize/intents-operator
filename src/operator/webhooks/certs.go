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
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"math/big"
	"os"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func WriteCertToFiles(bundle CertificateBundle) error {
	err := os.MkdirAll(CertDirPath, 0600)
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

func UpdateValidationWebHookCA(ctx context.Context, webHookName string, ca []byte) error {
	kubeClient, err := getKubeClient()
	if err != nil {
		return errors.Wrap(err)
	}

	webhookConfig, err := kubeClient.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(ctx, webHookName, metav1.GetOptions{})
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
	_, err = kubeClient.AdmissionregistrationV1().ValidatingWebhookConfigurations().Patch(ctx, webHookName, types.JSONPatchType, newCAByte, metav1.PatchOptions{})
	return errors.Wrap(err)
}

func UpdateConversionWebhookCAs(ctx context.Context, k8sClient client.Client, ca []byte) error {
	if err := updateConversionWebHookCA(ctx, "clientintents.k8s.otterize.com", k8sClient, ca); err != nil {
		return fmt.Errorf("failed updating the CA of clientIntents conversion webhhok: %w", err)
	}
	if err := updateConversionWebHookCA(ctx, "protectedservices.k8s.otterize.com", k8sClient, ca); err != nil {
		return fmt.Errorf("failed updating the CA of protectedServices conversion webhhok: %w", err)
	}
	if err := updateConversionWebHookCA(ctx, "kafkaserverconfigs.k8s.otterize.com", k8sClient, ca); err != nil {
		return fmt.Errorf("failed updating the CA of kafkaServerConfigs conversion webhhok: %w", err)
	}
	return nil
}

func updateConversionWebHookCA(ctx context.Context, crdName string, k8sClient client.Client, ca []byte) error {
	crd := apiextensionsv1.CustomResourceDefinition{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: crdName}, &crd)
	if err != nil {
		return fmt.Errorf("could not get ClientIntents CRD: %w", err)
	}

	updatedCRD := crd.DeepCopy()

	if updatedCRD.Spec.Conversion == nil || updatedCRD.Spec.Conversion.Webhook == nil || updatedCRD.Spec.Conversion.Webhook.ClientConfig == nil {
		return fmt.Errorf("clientsIntents CRD does not contain a proper conversion webhook defenition")
	}
	updatedCRD.Spec.Conversion.Webhook.ClientConfig.CABundle = ca
	err = k8sClient.Patch(ctx, updatedCRD, client.MergeFrom(&crd))
	if err != nil {
		return fmt.Errorf("could not Patch ClientIntents CRD: %w", err)
	}

	return nil
}
