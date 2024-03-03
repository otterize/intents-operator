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
	ReasonApplyingDatabaseIntentsFailed = "ApplyingDatabaseIntentsFailed"
	ReasonAppliedDatabaseIntents        = "AppliedDatabaseIntents"
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
	intents := &otterizev1alpha3.ClientIntents{}
	logger := logrus.WithField("namespacedName", req.String())
	err := r.client.Get(ctx, req.NamespacedName, intents)
	if err != nil && k8serrors.IsNotFound(err) {
		logger.Info("No intents found")
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	if intents.Spec == nil {
		logger.Info("No specs found")
		return ctrl.Result{}, nil
	}

	action := otterizev1alpha3.DBPermissionChangeApply
	if !intents.ObjectMeta.DeletionTimestamp.IsZero() {
		action = otterizev1alpha3.DBPermissionChangeDelete
	}

	var dbIntents []otterizev1alpha3.Intent
	for _, intent := range intents.GetCallsList() {
		if intent.Type != otterizev1alpha3.IntentTypeDatabase {
			continue
		}
		dbIntents = append(dbIntents, intent)
	}

	if len(dbIntents) == 0 {
		return ctrl.Result{}, nil
	}

	databaseToIntents, err := r.MapDBNameToIntents(ctx, dbIntents, req.NamespacedName)
	if err != nil {
		r.RecordWarningEventf(intents, ReasonApplyingDatabaseIntentsFailed, "Failed applying database intents: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	for databaseName, dbIntents := range databaseToIntents {
		pgServerConf := otterizev1alpha3.PostgreSQLServerConfig{}
		err := r.client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: databaseName}, &pgServerConf)
		if err != nil {
			r.RecordWarningEventf(intents, ReasonApplyingDatabaseIntentsFailed, "Failed applying database intents: %s", err.Error())
			return ctrl.Result{}, errors.Wrap(err)
		}
		pgConfigurator := databaseconfigurator.NewPostgresConfigurator(pgServerConf.Spec)
		pgConfigurator.ConfigureDBFromIntents(ctx, types.NamespacedName{
			Namespace: intents.Namespace,
			Name:      intents.GetServiceName(),
		}, dbIntents, action)
	}

	//if err := r.otterizeClient.ApplyDatabaseIntent(ctx, dbIntents, action); err != nil {
	//	errType, errMsg, ok := graphqlclient.GetGraphQLUserError(err)
	//	if !ok || errType != graphqlclient.UserErrorTypeAppliedIntentsError {
	//		r.RecordWarningEventf(intents, ReasonApplyingDatabaseIntentsFailed, "Failed applying database intents: %s", err.Error())
	//		return ctrl.Result{}, errors.Wrap(err)
	//	}
	//	r.RecordWarningEventf(intents, ReasonApplyingDatabaseIntentsFailed, "Failed applying database intents: %s", errMsg)
	//	return ctrl.Result{}, errors.Wrap(err)
	//}
	//

	r.RecordNormalEventf(intents, ReasonAppliedDatabaseIntents, "Database intents reconcile complete, reconciled %d intent calls", len(dbIntents))

	return ctrl.Result{}, nil
}

func (r *DatabaseReconciler) MapDBNameToIntents(ctx context.Context, intents []otterizev1alpha3.Intent, namespacedName types.NamespacedName) (map[string][]otterizev1alpha3.Intent, error) {
	dbNamesToIntents := map[string][]otterizev1alpha3.Intent{}
	for _, intent := range intents {
		for _, dbResource := range intent.DatabaseResources {
			dbName := dbResource.DatabaseName
			if _, ok := dbNamesToIntents[dbName]; !ok {
				dbNamesToIntents[dbName] = []otterizev1alpha3.Intent{intent}
			} else {
				dbNamesToIntents[dbName] = append(dbNamesToIntents[dbName], intent)
			}
		}
	}

	//for dbname := range dbNamesToIntents {
	//	pgServerConfig := otterizev1alpha3.PostgreSQLServerConfig{}
	//	err := r.client.Get(ctx, types.NamespacedName{}, &pgServerConfig)
	//	if err != nil {
	//
	//	}
	//	}
	//}
	return dbNamesToIntents, nil
}
