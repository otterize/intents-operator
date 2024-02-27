package gcpagent

import (
	"crypto/sha256"
	"fmt"
	"github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/iam/v1beta1"
	"github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/k8s/v1alpha1"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const maxGCPNameLength = 30
const truncatedHashLength = 6
const maxTruncatedLength = maxGCPNameLength - truncatedHashLength - 1 // add another char for the hyphen

func (a *Agent) generateKSAPolicyName(ksaName string) string {
	return fmt.Sprintf("otr-%s-policy", ksaName)
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

func (a *Agent) generateIAMPartialPolicy(namespace string, ksaName string, intents []otterizev1alpha3.Intent) *v1beta1.IAMPartialPolicy {
	policyName := a.generateKSAPolicyName(ksaName)
	gsaFullName := a.GetGSAFullName(namespace, ksaName)
	saMember := fmt.Sprintf("serviceAccount:%s", gsaFullName)

	newIAMPolicyMember := &v1beta1.IAMPartialPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: namespace,
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
		// Name formats - https://cloud.google.com/iam/docs/conditions-resource-attributes#resource-name
		condition := v1beta1.PartialpolicyCondition{
			Title:      fmt.Sprintf("otr-%s", intent.Name),
			Expression: fmt.Sprintf("resource.name.startsWith(\"%s\")", intent.Name),
		}
		for _, permission := range intent.GCPPermissions {
			binding := v1beta1.PartialpolicyBindings{
				Role:      fmt.Sprintf("roles/%s", permission),
				Members:   []v1beta1.PartialpolicyMembers{{Member: &saMember}},
				Condition: condition.DeepCopy(),
			}

			newIAMPolicyMember.Spec.Bindings = append(newIAMPolicyMember.Spec.Bindings, binding)
		}
	}

	return newIAMPolicyMember
}
