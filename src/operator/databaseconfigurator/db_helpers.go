package databaseconfigurator

import (
	"fmt"
	"strings"
)

const PGUsernameMaxLength = 63

func BuildPostgresUsername(clusterUID string, client, namespace string) string {
	username := fmt.Sprintf("%s_%s_%s", KubernetesToPostgresName(client), KubernetesToPostgresName(namespace), clusterUID)
	if len(username) > PGUsernameMaxLength {
		username = username[:PGUsernameMaxLength]
	}
	return username
}

// KubernetesToPostgresName translates a name with Kubernetes conventions to Postgres conventions
func KubernetesToPostgresName(kubernetesName string) string {
	// '.' are replaced with dunders '__'
	// '-' are replaced with single underscores '_'
	return strings.ReplaceAll(strings.ReplaceAll(kubernetesName, ".", "__"), "-", "_")
}
