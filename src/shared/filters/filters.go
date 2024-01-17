package filters

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const labelKey = "app.kubernetes.io/part-of"
const labelValue = "otterize"

var filterByOtterizeLabel = func(obj client.Object) bool {
	existingValue, ok := obj.GetLabels()[labelKey]
	return ok && existingValue == labelValue
}

// FilterByOtterizeLabelPredicate Filter only resources that are part of Otterize
var FilterByOtterizeLabelPredicate = predicate.NewPredicateFuncs(filterByOtterizeLabel)
