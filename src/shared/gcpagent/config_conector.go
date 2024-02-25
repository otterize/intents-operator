package gcpagent

import (
	"context"
	gcpk8s "github.com/GoogleCloudPlatform/k8s-config-connector/operator/pkg/k8s"
	"github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/iam/v1beta1"
	"github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/k8s/v1alpha1"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"fmt"
)

func (a *Agent) AnnotateGKENamespace(ctx context.Context, namespaceName string) (requeue bool, err error) {
	logger := logrus.WithField("namespace", namespaceName)

	namespace := corev1.Namespace{}
	err = a.client.Get(ctx, client.ObjectKey{Name: namespaceName}, &namespace)
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
	err = a.client.Patch(ctx, updatedNamespace, client.MergeFrom(&namespace))
	if err != nil {
		if apierrors.IsConflict(err) {
			return true, nil
		}
		return false, errors.Wrap(err)
	}

	return false, nil
}

func (a *Agent) CreateAndConnectGSA(ctx context.Context, namespaceName string, ksaName string) error {
	err := a.createIAMServiceAccount(ctx, namespaceName, ksaName)
	if err != nil {
		return errors.Wrap(err)
	}

	err = a.createGSAToKSAPolicy(ctx, namespaceName, ksaName)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) DeleteGSA(ctx context.Context, namespaceName string, ksaName string) error {
	err := a.deleteGSAToKSAPolicy(ctx, namespaceName, ksaName)
	if err != nil {
		return errors.Wrap(err)
	}

	err = a.deleteIAMServiceAccount(ctx, namespaceName, ksaName)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) createIAMServiceAccount(ctx context.Context, namespaceName string, ksaName string) error {
	logger := logrus.WithField("namespace", namespaceName).WithField("account", ksaName)

	gsaName := a.generateGSAName(namespaceName, ksaName)
	gsaDisplayName := a.generateGSADisplayName(namespaceName, ksaName)

	// Skip if GSA already exists or an error occurred
	iamServiceAccount := v1beta1.IAMServiceAccount{}
	err := a.client.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: gsaName}, &iamServiceAccount)
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

	err = a.client.Create(ctx, newIAMServiceAccount)
	if err != nil {
		logger.WithError(err).Errorf("failed to create IAMServiceAccount")
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) createGSAToKSAPolicy(ctx context.Context, namespaceName string, ksaName string) error {
	logger := logrus.WithField("namespace", namespaceName).WithField("account", ksaName)

	gsaName := a.generateGSAName(namespaceName, ksaName)
	policyName := a.generateGSAToKSAPolicyName(ksaName)
	memberName := fmt.Sprintf("serviceAccount:%s.svc.id.goog[%s/%s]", a.projectID, namespaceName, ksaName)

	// Skip if policy already exists or an error occurred
	iamPolicyMember := v1beta1.IAMPolicyMember{}
	err := a.client.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: policyName}, &iamPolicyMember)
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

	err = a.client.Create(ctx, newIAMPolicyMember)
	if err != nil {
		logger.WithError(err).Errorf("failed to create IAMPolicyMember")
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) applyIAMPartialPolicy(ctx context.Context, namespaceName string, ksaName string, intents []otterizev1alpha3.Intent) error {
	// Name formats available - https://cloud.google.com/iam/docs/conditions-resource-attributes#resource-name
	logger := logrus.WithField("namespace", namespaceName).WithField("account", ksaName)

	policyName := a.generateKSAPolicyName(ksaName)

	gsaFullName := a.GetGSAFullName(namespaceName, ksaName)
	saMember := fmt.Sprintf("serviceAccount:%s", gsaFullName)

	// Find if there is an existing policy
	iamPolicyMember := v1beta1.IAMPartialPolicy{}
	err := a.client.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: policyName}, &iamPolicyMember)
	if !apierrors.IsNotFound(err) {
		return errors.Wrap(err)
	}

	var shouldCreate bool
	var newIAMPolicyMember *v1beta1.IAMPartialPolicy
	if err == nil {
		// Policy already exists
		shouldCreate = false
		newIAMPolicyMember = iamPolicyMember.DeepCopy()
	} else {
		// Create new policy
		shouldCreate = true
		newIAMPolicyMember = &v1beta1.IAMPartialPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      policyName,
				Namespace: namespaceName,
			},
			Spec: v1beta1.IAMPartialPolicySpec{
				ResourceRef: v1alpha1.IAMResourceRef{
					Kind:     "Project",
					External: a.projectID,
				},
				Bindings: []v1beta1.PartialpolicyBindings{},
			},
		}

		// Populate bindings
		for _, intent := range intents {
			// TODO: need to handle wildcards
			condition := v1beta1.PartialpolicyCondition{
				Title:      fmt.Sprintf("otr-%s", intent.Name),
				Expression: fmt.Sprintf("resource.name.startsWith(\"%s\")", intent.Name),
			}
			for _, permission := range intent.GCPPermissions {
				binding := v1beta1.PartialpolicyBindings{
					Role:      permission,
					Members:   []v1beta1.PartialpolicyMembers{{Member: &saMember}},
					Condition: condition.DeepCopy(),
				}

				newIAMPolicyMember.Spec.Bindings = append(newIAMPolicyMember.Spec.Bindings, binding)
			}
		}
	}

	if shouldCreate {
		err = a.client.Create(ctx, newIAMPolicyMember)
	} else {
		err = a.client.Update(ctx, newIAMPolicyMember)
	}
	if err != nil {
		logger.WithError(err).Errorf("failed to apply IAMPolicyMember")
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) deleteIAMServiceAccount(ctx context.Context, namespaceName string, ksaName string) error {
	logger := logrus.WithField("namespace", namespaceName).WithField("account", ksaName)

	gsaName := a.generateGSAName(namespaceName, ksaName)

	// Skip if IAMServiceAccount was already deleted
	iamServiceAccount := v1beta1.IAMServiceAccount{}
	err := a.client.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: gsaName}, &iamServiceAccount)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrap(err)
	}

	err = a.client.Delete(ctx, iamServiceAccount.DeepCopy())
	if err != nil {
		logger.WithError(err).Errorf("failed to delete IAMServiceAccount")
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) deleteGSAToKSAPolicy(ctx context.Context, namespaceName string, ksaName string) error {
	logger := logrus.WithField("namespace", namespaceName).WithField("account", ksaName)

	policyName := a.generateGSAToKSAPolicyName(ksaName)

	// Skip if IAMPolicyMember was already deleted
	iamPolicyMember := v1beta1.IAMPolicyMember{}
	err := a.client.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: policyName}, &iamPolicyMember)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrap(err)
	}

	err = a.client.Delete(ctx, iamPolicyMember.DeepCopy())
	if err != nil {
		logger.WithError(err).Errorf("failed to delete IAMPolicyMember")
		return errors.Wrap(err)
	}

	return nil
}
