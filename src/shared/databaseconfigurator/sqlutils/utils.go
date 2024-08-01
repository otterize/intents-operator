package sqlutils

import (
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"golang.org/x/exp/slices"
)

func GetGrantOperations(operations []otterizev2alpha1.DatabaseOperation) []otterizev2alpha1.DatabaseOperation {
	if len(operations) == 0 || slices.Contains(operations, otterizev2alpha1.DatabaseOperationAll) {
		// Omit everything else in case it's included to avoid Postgres errors, include just 'ALL'
		return []otterizev2alpha1.DatabaseOperation{otterizev2alpha1.DatabaseOperationAll}
	}

	return operations
}

func ContainsAllOperations(operations []otterizev2alpha1.DatabaseOperation) bool {
	return slices.Contains(operations, otterizev2alpha1.DatabaseOperationAll)
}
