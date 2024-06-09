package database

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/clusterutils"
	"github.com/otterize/intents-operator/src/shared/databaseconfigurator"
	"github.com/otterize/intents-operator/src/shared/databaseconfigurator/mysql"
	"github.com/otterize/intents-operator/src/shared/databaseconfigurator/postgres"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"slices"
	"strings"
	"time"
)

const (
	ReasonApplyingDatabaseIntentsFailed         = "ApplyingDatabaseIntentsFailed"
	ReasonAppliedDatabaseIntents                = "AppliedDatabaseIntents"
	ReasonErrorFetchingPostgresServerConfig     = "ErrorFetchingPostgreSQLServerConfig"
	ReasonErrorFetchingMySQLServerConfig        = "ErrorFetchingMySQLServerConfig"
	ReasonErrorConnectingToDatabase             = "ErrorConnectingToDatabase"
	ReasonMissingDBServerConfig                 = "MissingDBServerConfig"
	ReasonExcessPermissionsCleanupFailed        = "ExcessPermissionsCleanupFailed"
	ReasonCreatingDatabaseUserFailed            = "CreatingDatabaseUserFailed"
	ReasonAnnotatingPodFailedWithDBAccessFailed = "AnnotatingPodFailedWithDBAccessFailed"
)

type DatabaseReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	injectablerecorder.InjectableRecorder
	clusterID         *string
	serviceIdResolver serviceidresolver.ServiceResolver
}

func NewDatabaseReconciler(
	client client.Client,
	scheme *runtime.Scheme,
) *DatabaseReconciler {
	return &DatabaseReconciler{
		client:            client,
		scheme:            scheme,
		serviceIdResolver: serviceidresolver.NewResolver(client),
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
			"Error listing PostgreSQLServerConfigs. Error: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	mySQLServerConfigs := otterizev1alpha3.MySQLServerConfigList{}
	err = r.client.List(ctx, &mySQLServerConfigs)
	if err != nil {
		r.RecordWarningEventf(clientIntents, ReasonErrorFetchingMySQLServerConfig,
			"Error listing MySQLServerConfigs. Error: %s", err.Error())
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
	existingMySQLInstances := lo.Map(mySQLServerConfigs.Items, func(config otterizev1alpha3.MySQLServerConfig, _ int) string {
		return config.Name
	})

	existingDBInstances := append(existingPGInstances, existingMySQLInstances...)

	intentsMissingDBInstances := lo.Without(lo.Keys(dbInstanceToIntents), existingDBInstances...)
	for _, missingDBInstance := range intentsMissingDBInstances {
		// DB instances mentioned in intents but not found in any DB Server Config CRD.
		// We do not return an error on purpose, but rather record event and move on.
		// When a new DB ServerConf is created, the clientIntents will be reconciled again.
		r.RecordWarningEventf(clientIntents, ReasonMissingDBServerConfig,
			"Missing database server config: did not find DB server config to match database '%s' in the cluster", missingDBInstance)
	}

	for _, config := range pgServerConfigs.Items {
		err := r.applyPGDBInstanceIntents(ctx, config, clientIntents, dbUsername, dbInstanceToIntents[config.Name])
		if err != nil {
			r.RecordWarningEventf(clientIntents, ReasonApplyingDatabaseIntentsFailed,
				"Failed applying database clientIntents: %s", err.Error())
			return ctrl.Result{}, errors.Wrap(err)
		}
	}

	for _, config := range mySQLServerConfigs.Items {
		err := r.applyMySQLDBInstanceIntents(ctx, config, clientIntents, dbUsername, dbInstanceToIntents[config.Name])
		if err != nil {
			r.RecordWarningEventf(clientIntents, ReasonApplyingDatabaseIntentsFailed,
				"Failed applying database clientIntents: %s", err.Error())
			return ctrl.Result{}, errors.Wrap(err)
		}
	}

	r.RecordNormalEventf(clientIntents, ReasonAppliedDatabaseIntents, "Database clientIntents reconcile complete, reconciled %d intent calls", len(dbIntents))

	return ctrl.Result{}, nil
}

func (r *DatabaseReconciler) applyPGDBInstanceIntents(ctx context.Context, config otterizev1alpha3.PostgreSQLServerConfig, clientIntents *otterizev1alpha3.ClientIntents, dbUsername string, dbInstanceIntents []otterizev1alpha3.Intent) error {
	dbConfigurator, err := postgres.NewPostgresConfigurator(ctx, config.Spec)
	if err != nil {
		r.RecordWarningEventf(clientIntents, ReasonErrorConnectingToDatabase,
			"Error connecting to PostgreSQL server. Error: %s", err.Error())
		return errors.Wrap(err)
	}

	defer dbConfigurator.CloseConnection(ctx)

	return r.applyDBInstanceIntentsOnConfigurator(ctx, dbConfigurator, clientIntents, dbUsername, config.Name, dbInstanceIntents)
}

func (r *DatabaseReconciler) applyMySQLDBInstanceIntents(ctx context.Context, config otterizev1alpha3.MySQLServerConfig, clientIntents *otterizev1alpha3.ClientIntents, dbUsername string, dbInstanceIntents []otterizev1alpha3.Intent) error {
	dbConfigurator, err := mysql.NewMySQLConfigurator(ctx, config.Spec)
	if err != nil {
		r.RecordWarningEventf(clientIntents, ReasonErrorConnectingToDatabase,
			"Error connecting to PostgreSQL server. Error: %s", err.Error())
		return errors.Wrap(err)
	}

	defer dbConfigurator.CloseConnection(ctx)

	return r.applyDBInstanceIntentsOnConfigurator(ctx, dbConfigurator, clientIntents, dbUsername, config.Name, dbInstanceIntents)
}

func (r *DatabaseReconciler) applyDBInstanceIntentsOnConfigurator(
	ctx context.Context,
	dbConfigurator databaseconfigurator.DatabaseConfigurator,
	clientIntents *otterizev1alpha3.ClientIntents,
	dbUsername string,
	dbInstanceName string,
	dbInstanceIntents []otterizev1alpha3.Intent) error {

	intentsDeleted := !clientIntents.DeletionTimestamp.IsZero()

	userExists, err := dbConfigurator.ValidateUserExists(ctx, dbUsername)
	if err != nil {
		r.RecordWarningEventf(clientIntents, ReasonApplyingDatabaseIntentsFailed,
			"Failed querying for database user: %s", err.Error())
		return errors.Wrap(err)
	}

	if intentsDeleted || len(dbInstanceIntents) == 0 {
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

		if err := r.handleDatabaseAnnotationOnPod(ctx, *clientIntents, dbInstanceName); err != nil {
			return errors.Wrap(err)
		}

		return nil
	}

	// intents not deleted and there are intents to apply for this db instance
	if !userExists {
		err := r.createDBUser(ctx, dbConfigurator, dbUsername)
		if err != nil {
			r.RecordWarningEventf(clientIntents, ReasonCreatingDatabaseUserFailed,
				"Failed creating user %s: %s", dbUsername, err.Error())
			return errors.Wrap(err)
		}
	}

	if err := r.handleDatabaseAnnotationOnPod(ctx, *clientIntents, dbInstanceName); err != nil {
		r.RecordWarningEventf(clientIntents, ReasonAnnotatingPodFailedWithDBAccessFailed,
			"Failed annotating pod with databse: %s", err.Error())
		return errors.Wrap(err)
	}

	dbnameToDatabaseResources := getDBNameToDatabaseResourcesFromIntents(dbInstanceIntents)
	err = dbConfigurator.ApplyDatabasePermissionsForUser(ctx, dbUsername, dbnameToDatabaseResources)
	if err != nil {
		r.RecordWarningEventf(clientIntents, ReasonApplyingDatabaseIntentsFailed,
			"Failed applying database clientIntents: %s", err.Error())
		return errors.Wrap(err)
	}

	return nil
}

func (r *DatabaseReconciler) createDBUser(ctx context.Context, dbConfigurator databaseconfigurator.DatabaseConfigurator, dbUsername string) error {
	password, err := databaseconfigurator.GenerateRandomPassword()
	if err != nil {
		return errors.Wrap(err)
	}
	if err := dbConfigurator.CreateUser(ctx, dbUsername, password); err != nil {
		return errors.Wrap(err)
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

func (r *DatabaseReconciler) handleDatabaseAnnotationOnPod(ctx context.Context, intents otterizev1alpha3.ClientIntents, dbInstance string) error {
	// We annotate a pod here to trigger the credentials operator flow
	// It will create a user-password secret and modify the databases so those credentials could connect successfully
	// We only annotate one pod since we just need to trigger the credentials operator once, to create the secret
	// All pods replicas could then load the secret data and use it as login credentials
	pod, err := r.serviceIdResolver.ResolveClientIntentToPod(ctx, intents)
	if err != nil {
		if errors.Is(err, serviceidresolver.ErrPodNotFound) {
			// no matching pods are deployed yet, but that's OK.
			// the first pod to admit with matching database intents will trigger the intents reconcile loop
			// which will lead it to this code path again.
			return nil
		}
		return errors.Wrap(err)
	}

	if !pod.DeletionTimestamp.IsZero() {
		return nil
	}

	updatedPod := pod.DeepCopy()
	updatedPod.Annotations[databaseconfigurator.LatestAccessChangeAnnotation] = time.Now().Format(time.RFC3339)
	if !intents.DeletionTimestamp.IsZero() {
		// Clean all databases
		delete(updatedPod.Annotations, databaseconfigurator.DatabaseAccessAnnotation)
		delete(updatedPod.Annotations, databaseconfigurator.LatestAccessChangeAnnotation)
	} else {
		// We cannot simply add all DB instances mentioned in the client intents because we also depend on server configs
		// So we add one instance at a time, and only those which the operator successfully created a user for
		updatedPod.Annotations[databaseconfigurator.DatabaseAccessAnnotation] = strings.Join(getAllowedDatabasesSlice(pod, dbInstance), ",")
	}

	if err := r.client.Patch(ctx, updatedPod, client.MergeFrom(&pod)); err != nil {
		return errors.Wrap(err)
	}
	return nil
}

func getAllowedDatabasesSlice(pod corev1.Pod, dbInstance string) []string {
	allowedDatabases, ok := pod.Annotations[databaseconfigurator.DatabaseAccessAnnotation]
	if !ok {
		return []string{dbInstance}
	} else {
		databaseSlice := strings.Split(allowedDatabases, ",")
		if !slices.Contains(databaseSlice, dbInstance) {
			databaseSlice = append(databaseSlice, dbInstance)
		}
		return databaseSlice
	}
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
