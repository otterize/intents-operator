package intents_reconcilers

import (
	"context"
	"crypto/sha1"
	"fmt"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/prometheus"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetrysender"
	"github.com/samber/lo"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type TelemetryReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	injectablerecorder.InjectableRecorder
	intentsCounter      map[string]int
	typedIntentsCounter map[string]map[otterizev2alpha1.IntentType]int
}

func NewTelemetryReconciler(client client.Client, scheme *runtime.Scheme) *TelemetryReconciler {
	return &TelemetryReconciler{
		Client:              client,
		Scheme:              scheme,
		intentsCounter:      make(map[string]int),
		typedIntentsCounter: make(map[string]map[otterizev2alpha1.IntentType]int),
	}
}

func (r *TelemetryReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	intents := &otterizev2alpha1.ApprovedClientIntents{}
	err := r.Get(ctx, req.NamespacedName, intents)
	if k8serrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	hasher := sha1.New()
	resourceName := fmt.Sprintf("%s/%s", intents.Namespace, intents.Name)
	hashedName := string(hasher.Sum([]byte(resourceName)))

	if !intents.DeletionTimestamp.IsZero() {
		delete(r.intentsCounter, hashedName)
		delete(r.typedIntentsCounter, hashedName)
		return ctrl.Result{}, nil
	}

	r.intentsCounter[hashedName] = len(intents.Spec.Targets)
	r.typedIntentsCounter[hashedName] = make(map[otterizev2alpha1.IntentType]int)

	for _, call := range intents.Spec.Targets {
		r.typedIntentsCounter[hashedName][call.GetIntentType()]++
	}

	kafkaCount := 0
	httpCount := 0
	databaseCount := 0
	internetCount := 0
	for _, value := range r.typedIntentsCounter {
		kafkaCount += value[otterizev2alpha1.IntentTypeKafka]
		httpCount += value[otterizev2alpha1.IntentTypeHTTP]
		databaseCount += value[otterizev2alpha1.IntentTypeDatabase]
		internetCount += value[otterizev2alpha1.IntentTypeInternet]
	}

	intentsCount := lo.Sum(lo.Values(r.intentsCounter))
	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeIntentsApplied, intentsCount)
	prometheus.IncrementIntentsApplied(intentsCount)
	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeIntentsAppliedKafka, kafkaCount)
	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeIntentsAppliedHttp, httpCount)
	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeIntentsAppliedDatabase, databaseCount)
	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeIntentsAppliedInternet, internetCount)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TelemetryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&otterizev2alpha1.ApprovedClientIntents{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}
