package webhooks

import (
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
