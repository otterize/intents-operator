package intents_reconcilers

import (
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetrysender"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func RemoveIntentFinalizers(intents *otterizev1alpha3.ClientIntents, finalizerName string) {
	if len(intents.GetFinalizers()) <= 1 {
		telemetrysender.SendIntentOperator(telemetriesgql.EventTypeIntentsDeleted, 1)
	}
	controllerutil.RemoveFinalizer(intents, finalizerName)
}
