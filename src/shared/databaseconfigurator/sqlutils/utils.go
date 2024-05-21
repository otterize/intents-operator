package sqlutils

import (
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"golang.org/x/exp/slices"
)

func GetGrantOperations(operations []otterizev1alpha3.DatabaseOperation) []otterizev1alpha3.DatabaseOperation {
	if len(operations) == 0 || slices.Contains(operations, otterizev1alpha3.DatabaseOperationAll) {
		// Omit everything else in case it's included to avoid Postgres errors, include just 'ALL'
		return []otterizev1alpha3.DatabaseOperation{otterizev1alpha3.DatabaseOperationAll}
	}

	return operations
}

func ContainsAllOperations(operations []otterizev1alpha3.DatabaseOperation) bool {
	return slices.Contains(operations, otterizev1alpha3.DatabaseOperationAll)
}
