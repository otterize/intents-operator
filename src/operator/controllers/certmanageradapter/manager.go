package certmanageradapter

import (
	"context"
	certmanager "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/otterize/credentials-operator/src/controllers/secrets"
	secretstypes "github.com/otterize/credentials-operator/src/controllers/secrets/types"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
)

const (
	JKSPasswordsSecretName = "jks-passwords"
)

var _ secrets.K8sSecretsManagerSubclass[*certmanager.Certificate] = &CertManagerSecretsManager{}

type CertManagerSecretsManager struct {
	secrets.K8sSecretsManagerBase[*certmanager.Certificate]
	registry         *CertManagerWorkloadRegistry
	issuerName       string
	useClusterIssuer bool
}

func NewCertManagerSecretsManager(c client.Client,
	serviceIdResolver secretstypes.ServiceIdResolver,
	eventRecorder record.EventRecorder,
	issuerName string,
	useClusterIssuer bool,
) (*CertManagerSecretsManager, *CertManagerWorkloadRegistry) {
	cm := &CertManagerSecretsManager{
		registry:         NewCertManagerWorkloadRegistry(),
		issuerName:       issuerName,
		useClusterIssuer: useClusterIssuer,
	}
	cm.K8sSecretsManagerBase = *secrets.NewK8sSecretsManagerBase[*certmanager.Certificate](c, serviceIdResolver, eventRecorder, cm)
	return cm, cm.registry
}

func (cm *CertManagerSecretsManager) NewSecretObject() *certmanager.Certificate {
	return new(certmanager.Certificate)
}

func (cm *CertManagerSecretsManager) IsRefreshNeeded(*certmanager.Certificate) bool {
	return false
}

func (cm *CertManagerSecretsManager) ExtractConfig(cert *certmanager.Certificate) secretstypes.SecretConfig {
	config := secrets.SecretConfigFromAnnotations(cert.Annotations)
	config.SecretName = cert.Spec.SecretName
	config.Namespace = cert.Namespace
	return config
}

func (cm *CertManagerSecretsManager) PopulateSecretObject(ctx context.Context, config secretstypes.SecretConfig, cert *certmanager.Certificate) error {
	// File names are constant and determined by cert-manager itself - so override any configured value
	cert.Annotations[metadata.CAFileNameAnnotation] = "ca.crt"
	cert.Annotations[metadata.CertFileNameAnnotation] = "tls.crt"
	cert.Annotations[metadata.KeyFileNameAnnotation] = "tls.key"
	cert.Annotations[metadata.TrustStoreFileNameAnnotation] = "truststore.jks"
	cert.Annotations[metadata.KeyStoreFileNameAnnotation] = "keystore.jks"

	entry := cm.registry.getPodEntry(config.Namespace, config.ServiceName)
	if entry.Ttl != 0 {
		cert.Spec.Duration = &metav1.Duration{Duration: time.Duration(entry.Ttl) * time.Second}
	}

	cert.Spec.SecretName = config.SecretName
	cert.Spec.DNSNames = entry.DnsNames
	cert.Spec.CommonName = strings.Join([]string{entry.ServiceName, entry.Namespace}, ".")
	if cm.useClusterIssuer {
		cert.Spec.IssuerRef.Kind = "ClusterIssuer"
	} else {
		cert.Spec.IssuerRef.Kind = "Issuer"
	}
	cert.Spec.IssuerRef.Name = cm.issuerName

	if config.CertConfig.CertType == secretstypes.JKSCertType {
		jksPasswordRef, err := cm.getJKSPasswordSecretRef(ctx, config.Namespace, config.SecretName, config.CertConfig.JKSConfig.Password)
		if err != nil {
			return err
		}

		cert.Spec.Keystores = &certmanager.CertificateKeystores{
			JKS: &certmanager.JKSKeystore{
				Create:            true,
				PasswordSecretRef: *jksPasswordRef,
			},
		}
	} else {
		cert.Spec.Keystores = nil
	}

	return nil
}

func (cm *CertManagerSecretsManager) getJKSPasswordSecretRef(ctx context.Context, namespace, secretName, password string) (*cmmeta.SecretKeySelector, error) {
	jksPasswordsSecret := corev1.Secret{}
	newJksPasswordsSecret := false

	if err := cm.Client.Get(ctx, types.NamespacedName{Name: JKSPasswordsSecretName, Namespace: namespace}, &jksPasswordsSecret); err != nil {
		if apierrors.IsNotFound(err) {
			jksPasswordsSecret.Name = JKSPasswordsSecretName
			jksPasswordsSecret.Namespace = namespace
			jksPasswordsSecret.Data = make(map[string][]byte)
			newJksPasswordsSecret = true
		} else {
			return nil, err
		}
	}
	jksPasswordsSecret.Data[secretName] = []byte(password)

	if newJksPasswordsSecret {
		if err := cm.Client.Create(ctx, &jksPasswordsSecret); err != nil {
			logrus.WithError(err).Error("Can't create secret of jks passwords")
			return nil, err
		}
	} else {
		if err := cm.Client.Update(ctx, &jksPasswordsSecret); err != nil {
			logrus.WithError(err).Error("Can't update secret of jks passwords")
			return nil, err
		}
	}

	return &cmmeta.SecretKeySelector{
		LocalObjectReference: cmmeta.LocalObjectReference{
			Name: JKSPasswordsSecretName,
		},
		Key: secretName,
	}, nil
}
