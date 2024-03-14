package database

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/databaseconfigurator"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ReasonApplyingDatabaseIntentsFailed     = "ApplyingDatabaseIntentsFailed"
	ReasonAppliedDatabaseIntents            = "AppliedDatabaseIntents"
	ReasonErrorFetchingPostgresServerConfig = "ErrorFetchingPostgresServerConfig"
)

type DatabaseReconciler struct {
	client         client.Client
	scheme         *runtime.Scheme
	otterizeClient operator_cloud_client.CloudClient
	injectablerecorder.InjectableRecorder
}

func NewDatabaseReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	otterizeClient operator_cloud_client.CloudClient,
) *DatabaseReconciler {
	return &DatabaseReconciler{
		client:         client,
		scheme:         scheme,
		otterizeClient: otterizeClient,
	}
}

func (r *DatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	clientIntents := &otterizev1alpha3.ClientIntents{}
	logger := logrus.WithField("namespacedName", req.String())
	err := r.client.Get(ctx, req.NamespacedName, clientIntents)
	if err != nil && k8serrors.IsNotFound(err) {
		logger.Info("No client intents found")
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	if clientIntents.Spec == nil {
		logger.Info("No specs found")
		return ctrl.Result{}, nil
	}

	action := otterizev1alpha3.DBPermissionChangeApply
	if !clientIntents.ObjectMeta.DeletionTimestamp.IsZero() {
		action = otterizev1alpha3.DBPermissionChangeDelete
	}

	var dbIntents []otterizev1alpha3.Intent
	for _, intent := range clientIntents.GetCallsList() {
		if intent.Type != otterizev1alpha3.IntentTypeDatabase {
			continue
		}
		dbIntents = append(dbIntents, intent)
	}

	if len(dbIntents) == 0 {
		return ctrl.Result{}, nil
	}

	databaseToIntents, err := r.MapDBInstanceToIntents(dbIntents)
	if err != nil {
		r.RecordWarningEventf(clientIntents, ReasonApplyingDatabaseIntentsFailed, "Failed applying database clientIntents: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	for databaseInstance, intents := range databaseToIntents {
		pgServerConf := otterizev1alpha3.PostgreSQLServerConfig{}
		err := r.client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: databaseInstance}, &pgServerConf)
		if err != nil {
			r.RecordWarningEventf(clientIntents, ReasonErrorFetchingPostgresServerConfig,
				"Error trying to fetch '%s' PostgresServerConf for client '%s'. Error: %s",
				databaseInstance, clientIntents.GetServiceName(), err.Error())
			return ctrl.Result{}, nil
		}
		pgConfigurator := databaseconfigurator.NewPostgresConfigurator(pgServerConf.Spec, r.client)
		err = pgConfigurator.ConfigureDBFromIntents(ctx, clientIntents.GetServiceName(), clientIntents.Namespace, intents, action)
		if err != nil {
			r.RecordWarningEventf(clientIntents, ReasonApplyingDatabaseIntentsFailed,
				"Failed applying database clientIntents: %s", err.Error())
			return ctrl.Result{}, errors.Wrap(err)
		}
	}

	r.RecordNormalEventf(clientIntents, ReasonAppliedDatabaseIntents, "Database clientIntents reconcile complete, reconciled %d intent calls", len(dbIntents))

	return ctrl.Result{}, nil
}

func (r *DatabaseReconciler) MapDBInstanceToIntents(intents []otterizev1alpha3.Intent) (map[string][]otterizev1alpha3.Intent, error) {
	dbInstanceToIntents := map[string][]otterizev1alpha3.Intent{}
	for _, intent := range intents {
		// Name represents a database instance which is represented by a matching server config CRD
		instanceIntents, ok := dbInstanceToIntents[intent.Name]
		if !ok {
			dbInstanceToIntents[intent.Name] = []otterizev1alpha3.Intent{intent}
			continue
		}
		dbInstanceToIntents[intent.Name] = append(instanceIntents, intent)
	}

	return dbInstanceToIntents, nil
}
