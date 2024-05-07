package database

import (
	"context"
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/clusterutils"
	"github.com/otterize/intents-operator/src/shared/databaseconfigurator"
	"github.com/otterize/intents-operator/src/shared/databaseconfigurator/mysql"
	"github.com/otterize/intents-operator/src/shared/databaseconfigurator/postgres"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
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
	ReasonErrorFetchingPostgresServerConfig = "ErrorFetchingPostgreSQLServerConfig"
	ReasonErrorFetchingMySQLServerConfig    = "ErrorFetchingMySQLServerConfig"
	ReasonMissingDBServerConfig             = "MissingDBServerConfig"
	ReasonExcessPermissionsCleanupFailed    = "ExcessPermissionsCleanupFailed"
)

type DatabaseReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	injectablerecorder.InjectableRecorder
	clusterID *string
}

func NewDatabaseReconciler(
	client client.Client,
	scheme *runtime.Scheme,
) *DatabaseReconciler {
	return &DatabaseReconciler{
		client: client,
		scheme: scheme,
	}
}

func (r *DatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	clientIntents := &otterizev1alpha3.ClientIntents{}
	logger := logrus.WithField("namespacedName", req.String())
	err := r.client.Get(ctx, req.NamespacedName, clientIntents)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Info("No client intents found")
			return ctrl.Result{}, nil

		}
		return ctrl.Result{}, errors.Wrap(err)
	}

	if clientIntents.Spec == nil {
		logger.Info("No specs found")
		return ctrl.Result{}, nil
	}

	pgServerConfigs := otterizev1alpha3.PostgreSQLServerConfigList{}
	err = r.client.List(ctx, &pgServerConfigs)
	if err != nil {
		r.RecordWarningEventf(clientIntents, ReasonErrorFetchingPostgresServerConfig,
			"Error listing PostgreSQLServerConfings. Error: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	mySQLServerConfigs := otterizev1alpha3.PostgreSQLServerConfigList{} // TODO: read MySQLServerConfigs
	err = r.client.List(ctx, &mySQLServerConfigs)
	if err != nil {
		r.RecordWarningEventf(clientIntents, ReasonErrorFetchingPostgresServerConfig,
			"Error listing MySQLServerConfings. Error: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	clusterID, err := r.getClusterID(ctx)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}
	username := clusterutils.BuildHashedUsername(clientIntents.GetServiceName(), clientIntents.Namespace, clusterID)
	dbUsername := clusterutils.KubernetesToPostgresName(username)

	dbIntents := clientIntents.GetDatabaseIntents()
	dbInstanceToIntents := lo.GroupBy(dbIntents, func(intent otterizev1alpha3.Intent) string {
		return intent.Name // "Name" is the db instance name in our case.
	})

	existingPGInstances := lo.Map(pgServerConfigs.Items, func(config otterizev1alpha3.PostgreSQLServerConfig, _ int) string {
		return config.Name
	})
	existingMySQLInstances := lo.Map(mySQLServerConfigs.Items, func(config otterizev1alpha3.PostgreSQLServerConfig, _ int) string {
		return config.Name
	})

	existingDBInstances := append(existingPGInstances, existingMySQLInstances...)

	missingDBServerConfig := lo.Without(lo.Keys(dbInstanceToIntents), existingDBInstances...)
	for _, missingDBInstance := range missingDBServerConfig {
		// Not returning error on purpose, missing PGServerConf - record event and move on
		// When a new PGServerConf is created, the clientIntents will be reconciled again
		r.RecordWarningEventf(clientIntents, ReasonMissingDBServerConfig,
			"Missing database server config: did not find DB server config to match database '%s' in the cluster", missingDBInstance)
	}

	for _, config := range pgServerConfigs.Items {
		err := r.applyPGDBInstanceIntents(ctx, config, clientIntents, dbUsername, dbInstanceToIntents)
		if err != nil {
			r.RecordWarningEventf(clientIntents, ReasonApplyingDatabaseIntentsFailed,
				"Failed applying database clientIntents: %s", err.Error())
			return ctrl.Result{}, errors.Wrap(err)
		}
	}

	for _, config := range mySQLServerConfigs.Items {
		err := r.applyMySQLDBInstanceIntents(ctx, config, clientIntents, dbUsername, dbInstanceToIntents)
		if err != nil {
			r.RecordWarningEventf(clientIntents, ReasonApplyingDatabaseIntentsFailed,
				"Failed applying database clientIntents: %s", err.Error())
			return ctrl.Result{}, errors.Wrap(err)
		}
	}

	r.RecordNormalEventf(clientIntents, ReasonAppliedDatabaseIntents, "Database clientIntents reconcile complete, reconciled %d intent calls", len(dbIntents))

	return ctrl.Result{}, nil
}

func (r *DatabaseReconciler) applyPGDBInstanceIntents(ctx context.Context, config otterizev1alpha3.PostgreSQLServerConfig, clientIntents *otterizev1alpha3.ClientIntents, dbUsername string, dbInstanceToIntents map[string][]otterizev1alpha3.Intent) error {
	dbConfigurator, err := postgres.NewPostgresConfigurator(ctx, config.Spec)
	if err != nil {
		r.RecordWarningEventf(clientIntents, ReasonErrorFetchingPostgresServerConfig,
			"Error connecting to PostgreSQL server. Error: %s", err.Error())
		return errors.Wrap(err)
	}

	defer dbConfigurator.CloseConnection(ctx)

	return r.applyDBInstanceIntentsOnConfigurator(ctx, dbConfigurator, clientIntents, dbUsername, dbInstanceToIntents[config.Name])
}

func (r *DatabaseReconciler) applyMySQLDBInstanceIntents(ctx context.Context, config otterizev1alpha3.PostgreSQLServerConfig, clientIntents *otterizev1alpha3.ClientIntents, dbUsername string, dbInstanceToIntents map[string][]otterizev1alpha3.Intent) error {
	dbConfigurator, err := mysql.NewMySQLConfigurator(ctx, config.Spec)
	if err != nil {
		r.RecordWarningEventf(clientIntents, ReasonErrorFetchingPostgresServerConfig,
			"Error connecting to PostgreSQL server. Error: %s", err.Error())
		return errors.Wrap(err)
	}

	defer dbConfigurator.Close()

	return r.applyDBInstanceIntentsOnConfigurator(ctx, dbConfigurator, clientIntents, dbUsername, dbInstanceToIntents[config.Name])
}

func (r *DatabaseReconciler) applyDBInstanceIntentsOnConfigurator(ctx context.Context, dbConfigurator databaseconfigurator.DatabaseConfigurator, clientIntents *otterizev1alpha3.ClientIntents, dbUsername string, dbInstanceIntents []otterizev1alpha3.Intent) error {
	intentsDeleted := !clientIntents.DeletionTimestamp.IsZero()

	userExists, err := dbConfigurator.ValidateUserExists(ctx, dbUsername)
	if err != nil {
		r.RecordWarningEventf(clientIntents, ReasonApplyingDatabaseIntentsFailed,
			"Failed querying for database user: %s", err.Error())
		return errors.Wrap(err)
	}

	if intentsDeleted {
		if !userExists {
			// User was never in the db, nothing more to do
			return nil
		}
		err = dbConfigurator.RevokeAllDatabasePermissionsForUser(ctx, dbUsername)
		if err != nil {
			r.RecordWarningEventf(clientIntents, ReasonExcessPermissionsCleanupFailed,
				"Failed revoking all database permissions: %s", err.Error())
			return errors.Wrap(err)
		}

		logrus.Infof("ClientIntents deleted, dropping user %s from DB", dbUsername)
		if err := dbConfigurator.DropUser(ctx, dbUsername); err != nil {
			return errors.Wrap(err)
		}

		return nil
	} else {
		if !userExists {
			r.RecordWarningEventf(clientIntents, ReasonApplyingDatabaseIntentsFailed,
				"User %s does not exist, waiting for it to be created by credentials operator", dbUsername)
			return errors.New("user does not exist in the database")
		}

		dbnameToDatabaseResources := getDBNameToDatabaseResourcesFromIntents(dbInstanceIntents)
		err = dbConfigurator.ApplyDatabasePermissionsForUser(ctx, dbUsername, dbnameToDatabaseResources)
		if err != nil {
			r.RecordWarningEventf(clientIntents, ReasonApplyingDatabaseIntentsFailed,
				"Failed applying database clientIntents: %s", err.Error())
			return errors.Wrap(err)
		}
	}

	return nil
}

func (r *DatabaseReconciler) getClusterID(ctx context.Context) (string, error) {
	if r.clusterID != nil {
		return *r.clusterID, nil
	}
	clusterID, err := clusterutils.GetClusterUID(ctx)
	if err != nil {
		return "", errors.Wrap(err)
	}
	r.clusterID = &clusterID
	return clusterID, nil
}

func getDBNameToDatabaseResourcesFromIntents(intents []otterizev1alpha3.Intent) map[string][]otterizev1alpha3.DatabaseResource {
	dbnameToResources := make(map[string][]otterizev1alpha3.DatabaseResource)
	for _, intent := range intents {
		for _, dbResource := range intent.DatabaseResources {
			if _, ok := dbnameToResources[dbResource.DatabaseName]; !ok {
				dbnameToResources[dbResource.DatabaseName] = []otterizev1alpha3.DatabaseResource{dbResource}
				continue
			}
			// TODO: Smart merge instead of just adding
			resources := dbnameToResources[dbResource.DatabaseName]
			dbnameToResources[dbResource.DatabaseName] = append(resources, dbResource)
		}
	}
	return dbnameToResources
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
