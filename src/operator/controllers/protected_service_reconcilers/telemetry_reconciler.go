package protected_service_reconcilers

import (
	"context"
	"fmt"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/prometheus"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetrysender"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type TelemetryReconciler struct {
	client.Client
	injectablerecorder.InjectableRecorder
	protectedServicesCounter sets.Set[string]
}

func NewTelemetryReconciler(client client.Client) *TelemetryReconciler {
	return &TelemetryReconciler{
		Client:                   client,
		protectedServicesCounter: sets.New[string](),
	}
}

func (r *TelemetryReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	protectedService := &otterizev2alpha1.ProtectedService{}
	err := r.Get(ctx, req.NamespacedName, protectedService)
	if k8serrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	anonymizedServerName := telemetrysender.Anonymize(fmt.Sprintf("%s/%s",
		protectedService.Namespace,
		protectedService.Spec.Name,
	))

	if !protectedService.DeletionTimestamp.IsZero() {
		delete(r.protectedServicesCounter, anonymizedServerName)
		return ctrl.Result{}, nil
	}

	r.protectedServicesCounter.Insert(anonymizedServerName)

	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeProtectedServiceApplied, r.protectedServicesCounter.Len())
	prometheus.SetProtectedServicesApplied(r.protectedServicesCounter.Len())

	return ctrl.Result{}, nil
}
