package webhooks

import (
	otterizev2beta1 "github.com/otterize/intents-operator/src/operator/api/v2beta1"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"strings"
)

// hasWildcardDomain checks if a list of FQDNs contains one with a '*' character.
func hasWildcardDomain(fqdns []string) bool {
	for _, fqdn := range fqdns {
		if strings.Contains(fqdn, "*") {
			return true
		}
	}
	return false
}

func nonServiceTarget(target otterizev2beta1.Target) bool {
	if target.Service == nil && (target.Kubernetes == nil || target.Kubernetes.Kind != serviceidentity.KindService) {
		return true
	}

	return false
}
