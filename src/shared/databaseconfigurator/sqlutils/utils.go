package sqlutils

import (
	otterizev2 "github.com/otterize/intents-operator/src/operator/api/v2"
	"golang.org/x/exp/slices"
)

func GetGrantOperations(operations []otterizev2.DatabaseOperation) []otterizev2.DatabaseOperation {
	if len(operations) == 0 || slices.Contains(operations, otterizev2.DatabaseOperationAll) {
		// Omit everything else in case it's included to avoid Postgres errors, include just 'ALL'
		return []otterizev2.DatabaseOperation{otterizev2.DatabaseOperationAll}
	}

	return operations
}

func ContainsAllOperations(operations []otterizev2.DatabaseOperation) bool {
	return slices.Contains(operations, otterizev2.DatabaseOperationAll)
}
