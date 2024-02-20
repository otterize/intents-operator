package gcp_agent

import (
	"context"
	"crypto/sha256"
	gcpk8s "github.com/GoogleCloudPlatform/k8s-config-connector/operator/pkg/k8s"
	"github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/iam/v1beta1"
	"github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/k8s/v1alpha1"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"fmt"
)

func (a *Agent) AnnotateGKENamespace(ctx context.Context, c client.Client, namespaceName string) (requeue bool, err error) {
	logger := logrus.WithField("namespace", namespaceName)

	namespace := corev1.Namespace{}
	err = c.Get(ctx, client.ObjectKey{Name: namespaceName}, &namespace)
	if err != nil {
		return false, errors.Wrap(err)
	}

	// Annotate the namespace object
	updatedNamespace := namespace.DeepCopy()
	if updatedNamespace.Annotations == nil {
		updatedNamespace.Annotations = make(map[string]string)
	} else {
		// Check if the namespace is already annotated
		annotationValue, hasAnnotation := namespace.Annotations[gcpk8s.ProjectIdAnnotation]
		if hasAnnotation {
			if annotationValue == a.projectID {
				logger.Debugf("skipping namespace annotation: %s", namespaceName)
				return false, nil
			} else {
				return false, errors.Errorf("namespace %s already annotated with a different project ID: %s", namespaceName, annotationValue)
			}
		}
	}
	updatedNamespace.Annotations[gcpk8s.ProjectIdAnnotation] = a.projectID

	logger.Debugf("annotating namespace %s with gcp workload identity tag", namespaceName)
	err = c.Patch(ctx, updatedNamespace, client.MergeFrom(&namespace))
	if err != nil {
		if apierrors.IsConflict(err) {
			return true, nil
		}
		return false, errors.Wrap(err)
	}

	return false, nil
}

func (a *Agent) CreateAndConnectGSA(ctx context.Context, c client.Client, namespaceName string, ksaName string) error {
	err := a.createIAMServiceAccount(ctx, c, namespaceName, ksaName)
	if err != nil {
		return errors.Wrap(err)
	}

	err = a.createGSAToKSAPolicy(ctx, c, namespaceName, ksaName)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) DeleteGSA(ctx context.Context, c client.Client, namespaceName string, ksaName string) error {
	err := a.deleteGSAToKSAPolicy(ctx, c, namespaceName, ksaName)
	if err != nil {
		return errors.Wrap(err)
	}

	err = a.deleteIAMServiceAccount(ctx, c, namespaceName, ksaName)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) createIAMServiceAccount(ctx context.Context, c client.Client, namespaceName string, ksaName string) error {
	logger := logrus.WithField("namespace", namespaceName).WithField("account", ksaName)

	gsaName := a.generateGSAName(namespaceName, ksaName)
	gsaDisplayName := a.generateGSADisplayName(namespaceName, ksaName)

	// Skip if GSA already exists or an error occurred
	iamServiceAccount := v1beta1.IAMServiceAccount{}
	err := c.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: gsaName}, &iamServiceAccount)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return errors.Wrap(err)
	}

	// Currently we are only supporting project-level IAM roles
	annotations := map[string]string{
		gcpk8s.ProjectIdAnnotation: a.projectID,
	}

	newIAMServiceAccount := &v1beta1.IAMServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        gsaName,
			Namespace:   namespaceName,
			Annotations: annotations,
		},
		Spec: v1beta1.IAMServiceAccountSpec{
			DisplayName: &gsaDisplayName,
		},
	}

	err = c.Create(ctx, newIAMServiceAccount)
	if err != nil {
		logger.WithError(err).Errorf("failed to create IAMServiceAccount")
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) createGSAToKSAPolicy(ctx context.Context, c client.Client, namespaceName string, ksaName string) error {
	logger := logrus.WithField("namespace", namespaceName).WithField("account", ksaName)

	gsaName := a.generateGSAName(namespaceName, ksaName)
	policyName := a.generateGSAToKSAPolicyName(ksaName)
	memberName := fmt.Sprintf("serviceAccount:%s.svc.id.goog[%s/%s]", a.projectID, namespaceName, ksaName)

	// Skip if policy already exists or an error occurred
	iamPolicyMember := v1beta1.IAMPolicyMember{}
	err := c.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: policyName}, &iamPolicyMember)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return errors.Wrap(err)
	}

	newIAMPolicyMember := &v1beta1.IAMPolicyMember{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: namespaceName,
		},
		Spec: v1beta1.IAMPolicyMemberSpec{
			ResourceRef: v1alpha1.IAMResourceRef{
				APIVersion: "iam.cnrm.cloud.google.com/v1beta1",
				Kind:       "IAMServiceAccount",
				Name:       gsaName,
			},
			Role:   "roles/iam.workloadIdentityUser",
			Member: &memberName,
		},
	}

	err = c.Create(ctx, newIAMPolicyMember)
	if err != nil {
		logger.WithError(err).Errorf("failed to create IAMPolicyMember")
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) deleteIAMServiceAccount(ctx context.Context, c client.Client, namespaceName string, ksaName string) error {
	logger := logrus.WithField("namespace", namespaceName).WithField("account", ksaName)

	gsaName := a.generateGSAName(namespaceName, ksaName)

	// Skip if IAMServiceAccount was already deleted
	iamServiceAccount := v1beta1.IAMServiceAccount{}
	err := c.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: gsaName}, &iamServiceAccount)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrap(err)
	}

	err = c.Delete(ctx, iamServiceAccount.DeepCopy())
	if err != nil {
		logger.WithError(err).Errorf("failed to delete IAMServiceAccount")
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) deleteGSAToKSAPolicy(ctx context.Context, c client.Client, namespaceName string, ksaName string) error {
	logger := logrus.WithField("namespace", namespaceName).WithField("account", ksaName)

	policyName := a.generateGSAToKSAPolicyName(ksaName)

	// Skip if IAMPolicyMember was already deleted
	iamPolicyMember := v1beta1.IAMPolicyMember{}
	err := c.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: policyName}, &iamPolicyMember)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrap(err)
	}

	err = c.Delete(ctx, iamPolicyMember.DeepCopy())
	if err != nil {
		logger.WithError(err).Errorf("failed to delete IAMPolicyMember")
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) generateGSAToKSAPolicyName(ksaName string) string {
	return fmt.Sprintf("otr-%s-gcp-identity", ksaName)
}

func (a *Agent) generateGSADisplayName(namespace string, accountName string) string {
	return fmt.Sprintf("otr-%s-%s-%s", a.clusterName, namespace, accountName)
}

func (a *Agent) generateGSAName(namespace string, accountName string) string {
	// TODO: the max length is quite short, we should consider a different approach
	fullName := a.generateGSADisplayName(namespace, accountName)

	var truncatedName string
	if len(fullName) >= maxTruncatedLength {
		truncatedName = fullName[:maxTruncatedLength]
	} else {
		truncatedName = fullName
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(fullName)))
	hash = hash[:truncatedHashLength]

	return fmt.Sprintf("%s-%s", truncatedName, hash)
}

func (a *Agent) GetGSAFullName(namespace string, accountName string) string {
	gsaName := a.generateGSAName(namespace, accountName)
	return fmt.Sprintf("%s@%s.iam.gserviceaccount.com", gsaName, a.projectID)
}
