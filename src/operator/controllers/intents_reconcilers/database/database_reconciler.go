package database

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/database/databaseconfigurator"
	"github.com/otterize/intents-operator/src/shared/clusterid"
	"github.com/otterize/intents-operator/src/shared/databaseutils"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ReasonApplyingDatabaseIntentsFailed     = "ApplyingDatabaseIntentsFailed"
	ReasonAppliedDatabaseIntents            = "AppliedDatabaseIntents"
	ReasonErrorFetchingPostgresServerConfig = "ErrorFetchingPostgresServerConfig"
	ReasonMissingPostgresServerConfig       = "MissingPostgresServerConfig"
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

	dbInstanceToIntents := lo.GroupBy(dbIntents, func(intent otterizev1alpha3.Intent) string {
		return intent.Name // "Name" is the db instance name in our case.
	})

	if err != nil {
		r.RecordWarningEventf(clientIntents, ReasonApplyingDatabaseIntentsFailed, "Failed applying database clientIntents: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	for databaseInstance, intents := range dbInstanceToIntents {
		pgServerConfigs := otterizev1alpha3.PostgreSQLServerConfigList{}
		err := r.client.List(ctx, &pgServerConfigs)
		if err != nil {
			r.RecordWarningEventf(clientIntents, ReasonErrorFetchingPostgresServerConfig,
				"Error trying to fetch '%s' PostgresServerConf for client '%s'. Error: %s",
				databaseInstance, clientIntents.GetServiceName(), err.Error())
			return ctrl.Result{}, errors.Wrap(err)
		}
		pgServerConf, err := findMatchingPGServerConfForDBInstance(databaseInstance, pgServerConfigs)
		if err != nil {
			r.RecordWarningEventf(clientIntents, ReasonMissingPostgresServerConfig,
				"Could not find matching PostgreSQLServerConfig. Error: %s", err.Error())
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

	if err := r.cleanExcessPermissions(ctx, clientIntents); err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	r.RecordNormalEventf(clientIntents, ReasonAppliedDatabaseIntents, "Database clientIntents reconcile complete, reconciled %d intent calls", len(dbIntents))

	return ctrl.Result{}, nil
}

// cleanExcessPermissions compensates for DB resources completely removed from client intents
// Permission edits are handled by the normal flow because we run "revoke all" before adding permissions
// This is only used when permissions are completely removed, then we
func (r *DatabaseReconciler) cleanExcessPermissions(ctx context.Context, intents *otterizev1alpha3.ClientIntents) error {
	allDBServerConfigs := otterizev1alpha3.PostgreSQLServerConfigList{}
	if err := r.client.List(ctx, &allDBServerConfigs); err != nil {
		return errors.Wrap(err)
	}
	clusterID, err := clusterid.GetClusterUID(ctx)
	if err != nil {
		return err
	}

	pgUsername := databaseutils.BuildPostgresUsername(clusterID, intents.GetServiceName(), intents.Namespace)
	for _, config := range allDBServerConfigs.Items {
		pgConfigurator := databaseconfigurator.NewPostgresConfigurator(config.Spec, r.client)
		connectionString := pgConfigurator.FormatConnectionString(config.Spec.DatabaseName)
		conn, err := pgx.Connect(ctx, connectionString)
		if err != nil {
			logrus.WithError(err).Errorf("Failed connecting to database instace '%s'", config.Name)
			continue
		}
		pgConfigurator.SetConnection(ctx, conn)
		exists, err := databaseutils.ValidateUserExists(ctx, pgUsername, conn)
		if err != nil {
			return errors.Wrap(err)
		}
		if !exists {
			// User was never in the db, nothing more to do
			continue
		}
		_, found := lo.Find(intents.Spec.Calls, func(intent otterizev1alpha3.Intent) bool {
			return intent.Name == config.Name
		})
		if !found {
			// Username exists in the database, but doesn't have any intents for it, run "revoke all" just in case
			revokeBatch := &pgx.Batch{}
			if err := pgConfigurator.QueueRevokePermissionsByDatabaseNameStatements(ctx, revokeBatch, pgUsername); err != nil {
				return errors.Wrap(err)
			}
			if err := pgConfigurator.SendBatch(ctx, revokeBatch); err != nil {
				return errors.Wrap(err)
			}
		}
	}
	return nil
}

func findMatchingPGServerConfForDBInstance(
	databaseInstanceName string,
	pgServerConfigList otterizev1alpha3.PostgreSQLServerConfigList) (*otterizev1alpha3.PostgreSQLServerConfig, error) {

	matchingConf, found := lo.Find(pgServerConfigList.Items, func(conf otterizev1alpha3.PostgreSQLServerConfig) bool {
		return databaseInstanceName == conf.Name
	})

	if !found {
		return nil, errors.Wrap(fmt.Errorf(
			"did not find Postgres server config to match database '%s' in the cluster", databaseInstanceName))
	}

	return &matchingConf, nil
}
