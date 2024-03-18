package agentutils

import (
	"crypto/sha256"
	"fmt"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
)

const (
	truncatedHashLength = 6
)

// TruncateHashName truncates the given name to the given max length and appends a hash to it.
func TruncateHashName(fullName string, maxLen int) string {
	maxTruncatedLen := maxLen - truncatedHashLength - 1 // add another char for the hyphen

	var truncatedName string
	if len(fullName) >= maxTruncatedLen {
		truncatedName = fullName[:maxTruncatedLen]
	} else {
		truncatedName = fullName
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(fullName)))
	truncatedHash := hash[:truncatedHashLength]

	return fmt.Sprintf("%s-%s", truncatedName, truncatedHash)
}

// PtrSlicesEqual checks if two slices of pointers are equal, by comparing the dereferenced pointers
func PtrSlicesEqual[T comparable](a, b []*T) bool {
	return slices.EqualFunc(a, b, func(x, y *T) bool {
		return lo.FromPtr(x) == lo.FromPtr(y)
	})
}
