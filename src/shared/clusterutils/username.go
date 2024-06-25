package clusterutils

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	HashedUsernameSectionMaxLength = 10
)

func BuildHashedUsername(workloadName, namespace, clusterUID string) string {
	// We're trying to achieve uniqueness while not completely trimming the clusterUID if username + namespace are long
	// so we trim each section separately leaving room for 6 char hash suffix and string formatting chars
	fullUsername := fmt.Sprintf("%s-%s-%s", workloadName, namespace, clusterUID)
	hash := md5.Sum([]byte(fullUsername))

	if len(workloadName) > HashedUsernameSectionMaxLength {
		workloadName = strings.TrimSuffix(workloadName[:HashedUsernameSectionMaxLength], "-")
	}

	if len(namespace) > HashedUsernameSectionMaxLength {
		namespace = strings.TrimSuffix(namespace[:HashedUsernameSectionMaxLength], "-")
	}

	hashSuffix := hex.EncodeToString(hash[:])[:6]
	return fmt.Sprintf("%s-%s-id-%s", workloadName, namespace, hashSuffix)
}

// KubernetesToPostgresName translates a name with Kubernetes conventions to Postgres conventions
func KubernetesToPostgresName(kubernetesName string) string {
	// '.' and '-' are replaced with single underscores '_'
	return strings.ReplaceAll(strings.ReplaceAll(kubernetesName, ".", "_"), "-", "_")
}
