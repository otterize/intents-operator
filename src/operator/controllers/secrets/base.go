package secrets

import (
	"context"
	"fmt"
	certmanager "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/otterize/credentials-operator/src/controllers/secrets/types"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"time"
)

const (
	CertRenewReason = "CertificateRenewed"
)

func SecretConfigFromAnnotations(annotations map[string]string) secretstypes.SecretConfig {
	_, shouldRestartOnRenewalBool := annotations[metadata.ShouldRestartOnRenewalAnnotation]
	return secretstypes.SecretConfig{
		ServiceName:               annotations[metadata.TLSSecretRegisteredServiceNameAnnotation],
		EntryID:                   annotations[metadata.TLSSecretEntryIDAnnotation],
		EntryHash:                 annotations[metadata.TLSSecretEntryHashAnnotation],
		ShouldRestartPodOnRenewal: shouldRestartOnRenewalBool,
		CertConfig: secretstypes.CertConfig{
			CertType: secretstypes.CertType(annotations[metadata.CertTypeAnnotation]),
			PEMConfig: secretstypes.PEMConfig{
				CertFileName: annotations[metadata.CertFileNameAnnotation],
				CAFileName:   annotations[metadata.CAFileNameAnnotation],
				KeyFileName:  annotations[metadata.KeyFileNameAnnotation],
			},
			JKSConfig: secretstypes.JKSConfig{
				KeyStoreFileName:   annotations[metadata.KeyStoreFileNameAnnotation],
				TrustStoreFileName: annotations[metadata.TrustStoreFileNameAnnotation],
				Password:           annotations[metadata.JKSPasswordAnnotation],
			},
		},
	}
}

type K8sTlsSecretObject interface {
	*corev1.Secret | *certmanager.Certificate
	client.Object
}

// K8sSecretsManagerSubclass "abstract" methods that should be implemented by structs "inheriting" from K8sSecretsManagerBase
type K8sSecretsManagerSubclass[T K8sTlsSecretObject] interface {
	NewSecretObject() T
	IsRefreshNeeded(secretObj T) bool
	ExtractConfig(secretObj T) secretstypes.SecretConfig
	PopulateSecretObject(ctx context.Context, config secretstypes.SecretConfig, secretObj T) error
}

type K8sSecretsManagerBase[T K8sTlsSecretObject] struct {
	client.Client
	eventRecorder     record.EventRecorder
	serviceIdResolver secretstypes.ServiceIdResolver
	subclass          K8sSecretsManagerSubclass[T] // Achieves sort of method-overriding in inheriting classes
}

func NewK8sSecretsManagerBase[T K8sTlsSecretObject](
	c client.Client,
	serviceIdResolver secretstypes.ServiceIdResolver,
	eventRecorder record.EventRecorder,
	subclass K8sSecretsManagerSubclass[T]) *K8sSecretsManagerBase[T] {
	return &K8sSecretsManagerBase[T]{Client: c,
		serviceIdResolver: serviceIdResolver,
		eventRecorder:     eventRecorder,
		subclass:          subclass,
	}
}

func (m *K8sSecretsManagerBase[T]) getExistingSecretObject(ctx context.Context, namespace string, name string) (T, bool, error) {
	var found = m.subclass.NewSecretObject()
	if err := m.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, found); err != nil && apierrors.IsNotFound(err) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, errors.Wrap(err)
	}

	return found, true, nil
}

func (m *K8sSecretsManagerBase[T]) UpdateSecretObject(ctx context.Context, config secretstypes.SecretConfig, secretObj T) error {
	objLabels := map[string]string{
		metadata.SecretTypeLabel: string(secretstypes.TlsSecretType),
	}

	objAnnotations := map[string]string{
		metadata.TLSSecretRegisteredServiceNameAnnotation: config.ServiceName,
		metadata.TLSSecretEntryIDAnnotation:               config.EntryID,
		metadata.TLSSecretEntryHashAnnotation:             config.EntryHash,
		metadata.CertFileNameAnnotation:                   config.CertConfig.PEMConfig.CertFileName,
		metadata.CAFileNameAnnotation:                     config.CertConfig.PEMConfig.CAFileName,
		metadata.KeyFileNameAnnotation:                    config.CertConfig.PEMConfig.KeyFileName,
		metadata.KeyStoreFileNameAnnotation:               config.CertConfig.JKSConfig.KeyStoreFileName,
		metadata.TrustStoreFileNameAnnotation:             config.CertConfig.JKSConfig.TrustStoreFileName,
		metadata.JKSPasswordAnnotation:                    config.CertConfig.JKSConfig.Password,
		metadata.CertTypeAnnotation:                       string(config.CertConfig.CertType),
	}
	if config.ShouldRestartPodOnRenewal {
		// it only has to exist, we don't check the value
		objAnnotations[metadata.ShouldRestartOnRenewalAnnotation] = ""
	}

	secretObj.SetAnnotations(objAnnotations)
	secretObj.SetLabels(objLabels)
	return m.subclass.PopulateSecretObject(ctx, config, secretObj)
}

func (m *K8sSecretsManagerBase[T]) EnsureTLSSecret(ctx context.Context, config secretstypes.SecretConfig, pod *corev1.Pod) error {
	log := logrus.WithFields(logrus.Fields{"secret.namespace": config.Namespace, "secret.name": config.SecretName})

	existingObject, isExistingObject, err := m.getExistingSecretObject(ctx, config.Namespace, config.SecretName)
	if err != nil {
		log.WithError(err).Error("failed querying for certificate")
		return errors.Wrap(err)
	}

	var secretObj T
	shouldUpdate := false

	if isExistingObject {
		secretObj = existingObject
	} else {
		secretObj = m.subclass.NewSecretObject()
		secretObj.SetName(config.SecretName)
		secretObj.SetNamespace(config.Namespace)
	}

	if !isExistingObject ||
		m.subclass.IsRefreshNeeded(secretObj) ||
		m.isUpdateNeeded(m.subclass.ExtractConfig(secretObj), config) {
		if err := m.UpdateSecretObject(ctx, config, secretObj); err != nil {
			log.WithError(err).Error("failed updating TLS secret config")
			return errors.Wrap(err)
		}
		shouldUpdate = true
	}

	ownerCount := len(secretObj.GetOwnerReferences())
	if pod != nil {
		podOwner, err := m.serviceIdResolver.GetOwnerObject(ctx, pod)
		if err != nil {
			return errors.Wrap(err)
		}
		if err := controllerutil.SetOwnerReference(podOwner, secretObj, m.Scheme()); err != nil {
			log.WithError(err).Error("failed setting pod as owner reference")
			return errors.Wrap(err)
		}
		shouldUpdate = shouldUpdate || len(secretObj.GetOwnerReferences()) != ownerCount
	}

	if isExistingObject {
		if shouldUpdate {
			log.Info("Updating existing secret")
			if err := m.Update(ctx, secretObj); err != nil {
				logrus.WithError(err).Error("failed updating existing secret")
				return errors.Wrap(err)
			}
		}
	} else {
		log.Info("Creating a new secret")
		if err := m.Create(ctx, secretObj); err != nil {
			logrus.WithError(err).Error("failed creating new secret")
			return errors.Wrap(err)
		}
	}

	return nil
}

func (m *K8sSecretsManagerBase[T]) RefreshTLSSecrets(_ context.Context) error {
	// Not all resource implementations need refresh
	return nil
}

func (m *K8sSecretsManagerBase[T]) isUpdateNeeded(existingSecretConfig secretstypes.SecretConfig, newSecretConfig secretstypes.SecretConfig) bool {
	log := logrus.WithFields(logrus.Fields{"secret.namespace": existingSecretConfig.Namespace, "secret.name": existingSecretConfig.SecretName})
	needsUpdate := existingSecretConfig != newSecretConfig
	log.Infof("needs update: %v", needsUpdate)

	return needsUpdate
}

func (m *K8sSecretsManagerBase[T]) HandlePodRestarts(ctx context.Context, secret *corev1.Secret) error {
	podList := corev1.PodList{}
	labelSelector, err := labels.Parse(fmt.Sprintf("%s=%s", metadata.RegisteredServiceNameLabel, secret.Annotations[metadata.RegisteredServiceNameLabel]))
	if err != nil {
		return errors.Wrap(err)
	}

	err = m.List(ctx, &podList, &client.ListOptions{
		LabelSelector: labelSelector,
		Namespace:     secret.Namespace,
	})
	if err != nil {
		return errors.Wrap(err)
	}
	// create unique owner list
	owners := make(map[secretstypes.PodOwnerIdentifier]client.Object)
	for _, pod := range podList.Items {
		if ok := metadata.AnnotationExists(pod.Annotations, metadata.ShouldRestartOnRenewalAnnotation); ok {
			owner, err := m.serviceIdResolver.GetOwnerObject(ctx, &pod)
			if err != nil {
				return errors.Wrap(err)
			}
			owners[secretstypes.PodOwnerIdentifier{Name: owner.GetName(), GroupVersionKind: owner.GetObjectKind().GroupVersionKind()}] = owner
		}
	}
	for _, owner := range owners {
		logrus.Infof("Restarting pods for owner %s of type %s after certificate renewal",
			owner.GetName(), owner.GetObjectKind().GroupVersionKind().Kind)
		err = m.TriggerPodRestarts(ctx, owner, secret)
		if err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

// TriggerPodRestarts edits the pod owner's template spec with an annotation about the secret's expiry date
// If the secret is refreshed, its expiry will be updated in the pod owner's spec which will trigger the pods to restart
func (m *K8sSecretsManagerBase[T]) TriggerPodRestarts(ctx context.Context, owner client.Object, secret *corev1.Secret) error {
	kind := owner.GetObjectKind().GroupVersionKind().Kind
	switch kind {
	case "Deployment":
		deployment := v1.Deployment{}
		if err := m.Get(ctx, types.NamespacedName{Namespace: secret.Namespace, Name: owner.GetName()}, &deployment); err != nil {
			return errors.Wrap(err)
		}
		deployment.Spec.Template = m.updatePodTemplateSpec(deployment.Spec.Template)
		if err := m.Update(ctx, &deployment); err != nil {
			return errors.Wrap(err)
		}
		m.eventRecorder.Eventf(&deployment, corev1.EventTypeNormal, CertRenewReason, "Successfully restarted Deployment after secret '%s' renewal", secret.Name)
	case "ReplicaSet":
		replicaSet := v1.ReplicaSet{}
		if err := m.Get(ctx, types.NamespacedName{Namespace: secret.Namespace, Name: owner.GetName()}, &replicaSet); err != nil {
			return errors.Wrap(err)
		}
		replicaSet.Spec.Template = m.updatePodTemplateSpec(replicaSet.Spec.Template)
		if err := m.Update(ctx, &replicaSet); err != nil {
			return errors.Wrap(err)
		}
		m.eventRecorder.Eventf(&replicaSet, corev1.EventTypeNormal, CertRenewReason, "Successfully restarted ReplicaSet after secret '%s' renewal", secret.Name)
	case "StatefulSet":
		statefulSet := v1.StatefulSet{}
		if err := m.Get(ctx, types.NamespacedName{Namespace: secret.Namespace, Name: owner.GetName()}, &statefulSet); err != nil {
			return errors.Wrap(err)
		}
		statefulSet.Spec.Template = m.updatePodTemplateSpec(statefulSet.Spec.Template)
		if err := m.Update(ctx, &statefulSet); err != nil {
			return errors.Wrap(err)
		}
		m.eventRecorder.Eventf(&statefulSet, corev1.EventTypeNormal, CertRenewReason, "Successfully restarted StatefulSet after secret '%s' renewal", secret.Name)

	case "DaemonSet":
		daemonSet := v1.DaemonSet{}
		if err := m.Get(ctx, types.NamespacedName{Namespace: secret.Namespace, Name: owner.GetName()}, &daemonSet); err != nil {
			return errors.Wrap(err)
		}
		daemonSet.Spec.Template = m.updatePodTemplateSpec(daemonSet.Spec.Template)
		if err := m.Update(ctx, &daemonSet); err != nil {
			return errors.Wrap(err)
		}
		m.eventRecorder.Eventf(&daemonSet, corev1.EventTypeNormal, CertRenewReason, "Successfully restarted DaemonSet after secret '%s' renewal", secret.Name)

	default:
		return errors.Errorf("unsupported owner type: %s", kind)
	}
	return nil
}

func (m *K8sSecretsManagerBase[T]) updatePodTemplateSpec(podTemplateSpec corev1.PodTemplateSpec) corev1.PodTemplateSpec {
	if podTemplateSpec.Annotations == nil {
		podTemplateSpec.Annotations = map[string]string{}
	}
	podTemplateSpec.Annotations[metadata.TLSRestartTimeAfterRenewal] = time.Now().String()
	return podTemplateSpec
}
