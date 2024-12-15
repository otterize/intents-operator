package azureagent

import (
	"github.com/google/go-cmp/cmp"
	"slices"
)

func IsEqualAzureActions(a, b []*string) bool {
	slices.SortFunc(a, func(a, b *string) int {
		return compareStrings(a, b)
	})
	slices.SortFunc(b, func(a, b *string) int {
		return compareStrings(a, b)
	})

	return cmp.Equal(a, b)
}

func compareStrings(a, b *string) int {
	if *a < *b {
		return -1
	} else if *a > *b {
		return 1
	}
	return 0
}
