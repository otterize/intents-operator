package gcpagent

import (
	"crypto/sha256"
	"fmt"
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
