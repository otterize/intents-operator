package database

import (
	"context"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
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
	approvedClientIntents := &otterizev2alpha1.ApprovedClientIntents{}
	logger := logrus.WithField("namespacedName", req.String())
	err := r.client.Get(ctx, req.NamespacedName, approvedClientIntents)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Info("No client intents found")
			return ctrl.Result{}, nil

		}
		return ctrl.Result{}, errors.Wrap(err)
	}

	if approvedClientIntents.Spec == nil {
		logger.Info("No specs found")
		return ctrl.Result{}, nil
	}

	pgServerConfigs := otterizev2alpha1.PostgreSQLServerConfigList{}
	err = r.client.List(ctx, &pgServerConfigs)
	if err != nil {
		r.RecordWarningEventf(approvedClientIntents, ReasonErrorFetchingPostgresServerConfig,
			"Error listing PostgreSQLServerConfigs. Error: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	mySQLServerConfigs := otterizev2alpha1.MySQLServerConfigList{}
	err = r.client.List(ctx, &mySQLServerConfigs)
	if err != nil {
		r.RecordWarningEventf(approvedClientIntents, ReasonErrorFetchingMySQLServerConfig,
			"Error listing MySQLServerConfigs. Error: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	if !lo.SomeBy(approvedClientIntents.GetDatabaseIntents(), func(intent otterizev2alpha1.Target) bool {
		return true
	}) {
		return ctrl.Result{}, nil
	}

	clusterID, err := r.getClusterID(ctx)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}
	username := clusterutils.BuildHashedUsername(approvedClientIntents.GetWorkloadName(), approvedClientIntents.Namespace, clusterID)
	dbUsername := clusterutils.KubernetesToPostgresName(username)

	dbIntents := approvedClientIntents.GetDatabaseIntents()
	dbInstanceToIntents := lo.GroupBy(dbIntents, func(intent otterizev2alpha1.Target) string {
		return intent.GetTargetServerName() // "Name" is the db instance name in our case.
	})

	existingPGInstances := lo.Map(pgServerConfigs.Items, func(config otterizev2alpha1.PostgreSQLServerConfig, _ int) string {
		return config.Name
	})
	existingMySQLInstances := lo.Map(mySQLServerConfigs.Items, func(config otterizev2alpha1.MySQLServerConfig, _ int) string {
		return config.Name
	})

	existingDBInstances := append(existingPGInstances, existingMySQLInstances...)

	intentsMissingDBInstances := lo.Without(lo.Keys(dbInstanceToIntents), existingDBInstances...)
	for _, missingDBInstance := range intentsMissingDBInstances {
		// DB instances mentioned in intents but not found in any DB Server Config CRD.
		// We do not return an error on purpose, but rather record event and move on.
		// When a new DB ServerConf is created, the clientIntents will be reconciled again.
		r.RecordWarningEventf(approvedClientIntents, ReasonMissingDBServerConfig,
			"Missing database server config: did not find DB server config to match database '%s' in the cluster", missingDBInstance)
	}

	for _, config := range pgServerConfigs.Items {
		err := r.applyPGDBInstanceIntents(ctx, config, approvedClientIntents, dbUsername, dbInstanceToIntents[config.Name])
		if err != nil {
			r.RecordWarningEventf(approvedClientIntents, ReasonApplyingDatabaseIntentsFailed,
				"Failed applying database clientIntents: %s", err.Error())
			return ctrl.Result{}, errors.Wrap(err)
		}
	}

	for _, config := range mySQLServerConfigs.Items {
		err := r.applyMySQLDBInstanceIntents(ctx, config, approvedClientIntents, dbUsername, dbInstanceToIntents[config.Name])
		if err != nil {
			r.RecordWarningEventf(approvedClientIntents, ReasonApplyingDatabaseIntentsFailed,
				"Failed applying database clientIntents: %s", err.Error())
			return ctrl.Result{}, errors.Wrap(err)
		}
	}

	r.RecordNormalEventf(approvedClientIntents, ReasonAppliedDatabaseIntents, "Database clientIntents reconcile complete, reconciled %d intent calls", len(dbIntents))

	return ctrl.Result{}, nil
}

func (r *DatabaseReconciler) extractDBCredentials(ctx context.Context, namespace string, credentialsSpec otterizev2alpha1.DatabaseCredentials) (databaseconfigurator.DatabaseCredentials, error) {
	creds := databaseconfigurator.DatabaseCredentials{}
	if credentialsSpec.Username != "" {
		creds.Username = credentialsSpec.Username
	}
	if credentialsSpec.Password != "" {
		creds.Password = credentialsSpec.Password
	}
	if credentialsSpec.SecretRef != nil {
		secret := corev1.Secret{}
		name := credentialsSpec.SecretRef.Name
		if credentialsSpec.SecretRef.Namespace != "" {
			namespace = credentialsSpec.SecretRef.Namespace
		}
		err := r.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &secret)
		if err != nil {
			return creds, errors.Wrap(err)
		}
		if username, ok := secret.Data[credentialsSpec.SecretRef.UsernameKey]; ok {
			creds.Username = string(username)
		}
		if password, ok := secret.Data[credentialsSpec.SecretRef.PasswordKey]; ok {
			creds.Password = string(password)
		}
	}

	if creds.Username == "" || creds.Password == "" {
		return creds, errors.New("credentials missing either username or password")
	}

	return creds, nil
}

func (r *DatabaseReconciler) createPostgresDBConfigurator(ctx context.Context, pgServerConfig otterizev2alpha1.PostgreSQLServerConfig) (databaseconfigurator.DatabaseConfigurator, error) {
	credentials, err := r.extractDBCredentials(ctx, pgServerConfig.Namespace, pgServerConfig.Spec.Credentials)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	dbInfo := postgres.PostgresDatabaseInfo{
		Credentials: credentials,
		Address:     pgServerConfig.Spec.Address,
	}

	dbConfigurator, err := postgres.NewPostgresConfigurator(ctx, dbInfo)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	return dbConfigurator, nil
}

func (r *DatabaseReconciler) applyPGDBInstanceIntents(ctx context.Context, config otterizev2alpha1.PostgreSQLServerConfig, clientIntents *otterizev2alpha1.ApprovedClientIntents, dbUsername string, dbInstanceIntents []otterizev2alpha1.Target) error {
	dbConfigurator, err := r.createPostgresDBConfigurator(ctx, config)
	if err != nil {
		r.RecordWarningEventf(clientIntents, ReasonErrorConnectingToDatabase,
			"Error connecting to PostgreSQL server. Error: %s", err.Error())
		return errors.Wrap(err)
	}

	defer dbConfigurator.CloseConnection(ctx)

	return r.applyDBInstanceIntentsOnConfigurator(ctx, dbConfigurator, clientIntents, dbUsername, config.Name, dbInstanceIntents)
}

func (r *DatabaseReconciler) createMySQLDBConfigurator(ctx context.Context, mySqlServerConfig otterizev2alpha1.MySQLServerConfig) (databaseconfigurator.DatabaseConfigurator, error) {
	credentials, err := r.extractDBCredentials(ctx, mySqlServerConfig.Namespace, mySqlServerConfig.Spec.Credentials)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	dbInfo := mysql.MySQLDatabaseInfo{
		Credentials: credentials,
		Address:     mySqlServerConfig.Spec.Address,
	}

	dbConfigurator, err := mysql.NewMySQLConfigurator(ctx, dbInfo)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	return dbConfigurator, nil
}

func (r *DatabaseReconciler) applyMySQLDBInstanceIntents(ctx context.Context, config otterizev2alpha1.MySQLServerConfig, clientIntents *otterizev2alpha1.ApprovedClientIntents, dbUsername string, dbInstanceIntents []otterizev2alpha1.Target) error {
	dbConfigurator, err := r.createMySQLDBConfigurator(ctx, config)
	if err != nil {
		r.RecordWarningEventf(clientIntents, ReasonErrorConnectingToDatabase,
			"Error connecting to MySQL server. Error: %s", err.Error())
		return errors.Wrap(err)
	}

	defer dbConfigurator.CloseConnection(ctx)

	return r.applyDBInstanceIntentsOnConfigurator(ctx, dbConfigurator, clientIntents, dbUsername, config.Name, dbInstanceIntents)
}

func (r *DatabaseReconciler) applyDBInstanceIntentsOnConfigurator(
	ctx context.Context,
	dbConfigurator databaseconfigurator.DatabaseConfigurator,
	clientIntents *otterizev2alpha1.ApprovedClientIntents,
	dbUsername string,
	dbInstanceName string,
	dbInstanceIntents []otterizev2alpha1.Target) error {

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

func (r *DatabaseReconciler) handleDatabaseAnnotationOnPod(ctx context.Context, intents otterizev2alpha1.ApprovedClientIntents, dbInstance string) error {
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

func getDBNameToDatabaseResourcesFromIntents(intents []otterizev2alpha1.Target) map[string][]otterizev2alpha1.SQLPrivileges {
	dbnameToResources := make(map[string][]otterizev2alpha1.SQLPrivileges)
	for _, intent := range intents {
		for _, dbResource := range intent.SQL.Privileges {
			if _, ok := dbnameToResources[dbResource.DatabaseName]; !ok {
				dbnameToResources[dbResource.DatabaseName] = []otterizev2alpha1.SQLPrivileges{dbResource}
				continue
			}
			// TODO: Smart merge instead of just adding
			resources := dbnameToResources[dbResource.DatabaseName]
			dbnameToResources[dbResource.DatabaseName] = append(resources, dbResource)
		}
	}
	return dbnameToResources
}
