package intents_reconcilers

import (
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetrysender"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func removeIntentFinalizers(intents *otterizev1alpha2.ClientIntents, finalizerName string) {
	if len(intents.GetFinalizers()) <= 1 {
		telemetrysender.Send(telemetriesgql.EventTypeIntentsDeleted, map[string]string{"ns": telemetrysender.Anonymize(intents.Namespace)})
	}
	controllerutil.RemoveFinalizer(intents, finalizerName)
}
