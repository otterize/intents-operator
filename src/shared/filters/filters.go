package filters

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const LabelKey = "app.kubernetes.io/part-of"
const LabelValue = "otterize"

var filterByOtterizeLabel = func(obj client.Object) bool {
	existingValue, ok := obj.GetLabels()[LabelKey]
	return ok && existingValue == LabelValue
}

// FilterByOtterizeLabelPredicate Filter only resources that are part of Otterize
var FilterByOtterizeLabelPredicate = predicate.NewPredicateFuncs(filterByOtterizeLabel)
