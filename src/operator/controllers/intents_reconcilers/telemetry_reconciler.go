package intents_reconcilers

import (
	"context"
	"crypto/sha1"
	"fmt"

	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetrysender"
	"github.com/samber/lo"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type TelemetryReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	injectablerecorder.InjectableRecorder
	intentsCounter      map[string]int
	typedIntentsCounter map[string]map[otterizev1alpha2.IntentType]int
}

func NewTelemetryReconciler(client client.Client, scheme *runtime.Scheme) *TelemetryReconciler {
	return &TelemetryReconciler{
		Client:              client,
		Scheme:              scheme,
		intentsCounter:      make(map[string]int),
		typedIntentsCounter: make(map[string]map[otterizev1alpha2.IntentType]int),
	}
}

func (r *TelemetryReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	intents := &otterizev1alpha2.ClientIntents{}
	err := r.Get(ctx, req.NamespacedName, intents)
	if k8serrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	intents.Status.UpToDate = false

	defer func() {
		intents.Status.UpToDate = true
	}()

	hasher := sha1.New()
	resourceName := fmt.Sprintf("%s/%s", intents.Namespace, intents.Name)
	hashedName := string(hasher.Sum([]byte(resourceName)))

	if !intents.DeletionTimestamp.IsZero() {
		delete(r.intentsCounter, hashedName)
		delete(r.typedIntentsCounter, hashedName)
		return r.removeFinalizer(ctx, intents)
	}

	err = r.addFinalizer(ctx, intents)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.intentsCounter[hashedName] = len(intents.Spec.Calls)
	r.typedIntentsCounter[hashedName] = make(map[otterizev1alpha2.IntentType]int)

	for _, call := range intents.Spec.Calls {
		r.typedIntentsCounter[hashedName][call.Type]++
	}

	kafkaCount := 0
	httpCount := 0
	databaseCount := 0
	for _, value := range r.typedIntentsCounter {
		kafkaCount += value[otterizev1alpha2.IntentTypeKafka]
		httpCount += value[otterizev1alpha2.IntentTypeHTTP]
		databaseCount += value[otterizev1alpha2.IntentTypeDatabase]
	}

	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeIntentsApplied, lo.Sum(lo.Values(r.intentsCounter)))
	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeIntentsAppliedKafka, kafkaCount)
	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeIntentsAppliedHttp, httpCount)
	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeIntentsAppliedDatabase, databaseCount)

	return ctrl.Result{}, nil
}

func (r *TelemetryReconciler) removeFinalizer(ctx context.Context, intents *otterizev1alpha2.ClientIntents) (ctrl.Result, error) {
	RemoveIntentFinalizers(intents, otterizev1alpha2.OtterizeTelemetryReconcilerFinalizerName)
	err := r.Update(ctx, intents)
	if k8serrors.IsConflict(err) {
		return ctrl.Result{Requeue: true}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *TelemetryReconciler) addFinalizer(ctx context.Context, intents *otterizev1alpha2.ClientIntents) error {
	if controllerutil.ContainsFinalizer(intents, otterizev1alpha2.OtterizeTelemetryReconcilerFinalizerName) {
		return nil
	}

	controllerutil.AddFinalizer(intents, otterizev1alpha2.OtterizeTelemetryReconcilerFinalizerName)
	return r.Update(ctx, intents)
}
