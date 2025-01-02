package kafka_server_config_reconcilers

import (
	"context"
	"fmt"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
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
	kafkaServerCounter sets.Set[string]
}

func NewTelemetryReconciler(client client.Client) *TelemetryReconciler {
	return &TelemetryReconciler{
		Client:             client,
		kafkaServerCounter: sets.New[string](),
	}
}

func (r *TelemetryReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	kafkaServerConfig := &otterizev2alpha1.KafkaServerConfig{}
	err := r.Get(ctx, req.NamespacedName, kafkaServerConfig)
	if k8serrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	anonymizedServerName := telemetrysender.Anonymize(fmt.Sprintf("%s/%s",
		kafkaServerConfig.Namespace,
		kafkaServerConfig.Spec.Workload.Name,
	))

	if !kafkaServerConfig.DeletionTimestamp.IsZero() {
		delete(r.kafkaServerCounter, anonymizedServerName)
		return ctrl.Result{}, nil
	}

	r.kafkaServerCounter.Insert(anonymizedServerName)

	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeKafkaServerConfigApplied, r.kafkaServerCounter.Len())

	return ctrl.Result{}, nil
}
