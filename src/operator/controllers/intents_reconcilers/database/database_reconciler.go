package database

import (
	"context"
	"fmt"
	"github.com/amit7itz/goset"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/clusterutils"
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
	ReasonErrorFetchingPostgresServerConfig = "ErrorFetchingPostgresServerConfig"
	ReasonMissingPostgresServerConfig       = "MissingPostgresServerConfig"
	ReasonExcessPermissionsCleanupFailed    = "ExcessPermissionsCleanupFailed"
)

type DatabaseReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	injectablerecorder.InjectableRecorder
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

	dbIntents := clientIntents.GetDatabaseIntents()
	dbInstanceToIntents := lo.GroupBy(dbIntents, func(intent otterizev1alpha3.Intent) string {
		return intent.Name // "Name" is the db instance name in our case.
	})
	pgServerConfigs := otterizev1alpha3.PostgreSQLServerConfigList{}
	err = r.client.List(ctx, &pgServerConfigs)
	if err != nil {
		r.RecordWarningEventf(clientIntents, ReasonErrorFetchingPostgresServerConfig,
			"Error listing PostgresServerConfings. Error: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	for databaseInstance, intents := range dbInstanceToIntents {
		pgServerConf, err := findMatchingPGServerConfForDBInstance(databaseInstance, pgServerConfigs)
		if err != nil {
			r.RecordWarningEventf(clientIntents, ReasonMissingPostgresServerConfig,
				"Could not find matching PostgreSQLServerConfig. Error: %s", err.Error())
			return ctrl.Result{}, nil // Not returning error on purpose, missing PGServerConf - record event and move on
		}
		pgConfigurator := postgres.NewPostgresConfigurator(pgServerConf.Spec)
		dbnameToDatabaseResources := getDBNameToDatabaseResourcesFromIntents(intents)

		err = pgConfigurator.ConfigureDatabasePermissions(ctx, clientIntents.GetServiceName(), clientIntents.Namespace, action, dbnameToDatabaseResources)
		if err != nil {
			r.RecordWarningEventf(clientIntents, ReasonApplyingDatabaseIntentsFailed,
				"Failed applying database clientIntents: %s", err.Error())
			return ctrl.Result{}, errors.Wrap(err)
		}
	}

	r.RecordNormalEventf(clientIntents, ReasonAppliedDatabaseIntents, "Database clientIntents reconcile complete, reconciled %d intent calls", len(dbIntents))

	if err := r.cleanExcessPermissions(ctx, clientIntents, pgServerConfigs); err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	return ctrl.Result{}, nil
}

// getDBInstanceToDBNames maps all different database names from the client intents to a database instance
func getDBInstanceToDBNames(dbInstanceToIntents map[string][]otterizev1alpha3.Intent) map[string]*goset.Set[string] {
	instanceToDatabaseNames := make(map[string]*goset.Set[string])
	for instance, intents := range dbInstanceToIntents {
		instanceToDatabaseNames[instance] = &goset.Set[string]{}
		for _, intent := range intents {
			for _, resource := range intent.DatabaseResources {
				instanceToDatabaseNames[instance].Add(resource.DatabaseName)
			}
		}
	}
	return instanceToDatabaseNames
}

// cleanExcessPermissions compensates for DB resources completely removed from client intents
// Permission edits are handled by the normal flow because we run "revoke all" before adding permissions
// This is only used when permissions might have been completely removed in a ClientIntents edit operation
func (r *DatabaseReconciler) cleanExcessPermissions(
	ctx context.Context,
	clientIntents *otterizev1alpha3.ClientIntents,
	pgServerConfigs otterizev1alpha3.PostgreSQLServerConfigList) error {

	clusterID, err := clusterutils.GetClusterUID(ctx)
	if err != nil {
		return err
	}

	username := clusterutils.BuildHashedUsername(clientIntents.GetServiceName(), clientIntents.Namespace, clusterID)
	pgUsername := clusterutils.KubernetesToPostgresName(username)
	for _, config := range pgServerConfigs.Items {
		pgConfigurator := postgres.NewPostgresConfigurator(config.Spec)
		if err := pgConfigurator.SetConnection(ctx, postgres.PGDefaultDatabase); err != nil {
			pgErr, ok := postgres.TranslatePostgresConnectionError(err)
			if ok {
				return errors.Wrap(fmt.Errorf(pgErr))
			}
			return errors.Wrap(err)
		}
		exists, err := pgConfigurator.ValidateUserExists(ctx, pgUsername)
		if err != nil {
			return errors.Wrap(err)
		}
		if !exists {
			// User was never in the db, nothing more to do
			continue
		}
		err = pgConfigurator.RevokePermissionsFromInstance(ctx, pgUsername, config.Name, clientIntents)
		if err != nil {
			r.RecordWarningEventf(clientIntents, ReasonExcessPermissionsCleanupFailed,
				"Failed cleaning excess permissions from instance %s: %s", config.Name, err.Error())
			return errors.Wrap(err)
		}
	}

	return nil
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
