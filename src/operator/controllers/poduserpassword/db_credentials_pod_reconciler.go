package poduserpassword

import (
	"context"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/otterize/credentials-operator/src/operatorconfig"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/clusterutils"
	"github.com/otterize/intents-operator/src/shared/databaseconfigurator"
	"github.com/otterize/intents-operator/src/shared/databaseconfigurator/mysql"
	"github.com/otterize/intents-operator/src/shared/databaseconfigurator/postgres"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"strings"
	"time"
)

const (
	ReasonEnsuredPodUserAndPassword        = "EnsuredPodUserAndPassword"
	ReasonGeneratePodDatabaseUserFailed    = "GeneratePodDatabaseUserFailed"
	ReasonEnsuringPodUserAndPasswordFailed = "EnsuringPodUserAndPasswordFailed"
	ReasonEnsuringDatabasePasswordFailed   = "EnsuringDatabasePasswordFailed"
	ReasonRotatingSecretFailed             = "RotatingSecretFailed"
)

const (
	RefreshSecretsLoopTick = time.Minute * 5
)

type Reconciler struct {
	client            client.Client
	scheme            *runtime.Scheme
	recorder          record.EventRecorder
	serviceIdResolver *serviceidresolver.Resolver
}

func NewReconciler(client client.Client, scheme *runtime.Scheme, eventRecorder record.EventRecorder, serviceIdResolver *serviceidresolver.Resolver) *Reconciler {
	return &Reconciler{
		client:            client,
		scheme:            scheme,
		serviceIdResolver: serviceIdResolver,
		recorder:          eventRecorder,
	}
}

func (e *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		For(&v1.Pod{}).
		Complete(e)
}

func (e *Reconciler) shouldHandleCredentialsForPod(pod v1.Pod) bool {
	return pod.Annotations != nil && hasUserAndPasswordSecretAnnotation(pod)
}

func hasUserAndPasswordSecretAnnotation(pod v1.Pod) bool {
	_, ok := pod.Annotations[metadata.UserAndPasswordSecretNameAnnotation]
	return ok
}

func hasDatabaseAccessAnnotation(pod v1.Pod) bool {
	_, ok := pod.Annotations[databaseconfigurator.DatabaseAccessAnnotation]
	return ok
}

func (e *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var pod v1.Pod
	err := e.client.Get(ctx, req.NamespacedName, &pod)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrap(err)
	}
	if !pod.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	if !e.shouldHandleCredentialsForPod(pod) {
		return ctrl.Result{}, nil
	}

	username, err := e.generateServiceDatabaseUsername(ctx, &pod)
	if err != nil {
		e.recorder.Eventf(&pod, v1.EventTypeWarning, ReasonGeneratePodDatabaseUserFailed, "Failed to generate database username: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	logrus.Debug("Ensuring user-password credentials secrets for pod")
	password, err := e.ensurePodUserAndPasswordSecret(ctx, &pod, pod.Annotations[metadata.UserAndPasswordSecretNameAnnotation], username)
	if err != nil {
		e.recorder.Eventf(&pod, v1.EventTypeWarning, ReasonEnsuringPodUserAndPasswordFailed, "Failed to ensure user-password credentials secret: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	if hasDatabaseAccessAnnotation(pod) {
		logrus.Debug("Validating password in all databases")
		err = e.ensurePasswordInDatabases(ctx, pod, username, password)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err)
		}
	}
	e.recorder.Event(&pod, v1.EventTypeNormal, ReasonEnsuredPodUserAndPassword, "Ensured user-password credentials in specified secret")
	return ctrl.Result{}, nil
}

func (e *Reconciler) generateServiceDatabaseUsername(ctx context.Context, pod *v1.Pod) (string, error) {
	serviceID, err := e.serviceIdResolver.ResolvePodToServiceIdentity(ctx, pod)
	if err != nil {
		return "", errors.Wrap(err)
	}

	clusterID, err := clusterutils.GetClusterUID(ctx)
	if err != nil {
		return "", errors.Wrap(err)
	}

	username := clusterutils.BuildHashedUsername(serviceID.Name, pod.Namespace, clusterID)
	return clusterutils.KubernetesToPostgresName(username), nil
}

func (e *Reconciler) ensurePodUserAndPasswordSecret(ctx context.Context, pod *v1.Pod, secretName string, username string) (string, error) {
	log := logrus.WithFields(logrus.Fields{"pod": pod.Name, "namespace": pod.Namespace})
	secret := v1.Secret{}
	err := e.client.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: secretName}, &secret)
	if apierrors.IsNotFound(err) {
		log.Debug("Creating user-password credentials secret for pod")
		password, err := databaseconfigurator.GenerateRandomPassword()
		if err != nil {
			return "", errors.Wrap(err)
		}

		secret := buildUserAndPasswordCredentialsSecret(secretName, pod.Namespace, username, password)
		log.WithField("secret", secretName).Debug("Creating new secret with user-password credentials")
		if err := e.client.Create(ctx, secret); err != nil {
			return "", errors.Wrap(err)
		}
		return password, nil
	}

	if err != nil {
		return "", errors.Wrap(err)
	}
	log.Debug("Secret exists, nothing to do")
	return string(secret.Data["password"]), nil
}

func (e *Reconciler) ensurePasswordInDatabases(ctx context.Context, pod v1.Pod, username string, password string) error {
	databases := strings.Split(pod.Annotations[databaseconfigurator.DatabaseAccessAnnotation], ",")
	pgServerConfigs := otterizev1alpha3.PostgreSQLServerConfigList{}
	err := e.client.List(ctx, &pgServerConfigs)
	if err != nil {
		return errors.Wrap(err)
	}

	mysqlServerConfigs := otterizev1alpha3.MySQLServerConfigList{}
	err = e.client.List(ctx, &mysqlServerConfigs)
	if err != nil {
		return errors.Wrap(err)
	}

	for _, database := range databases {
		configFound, err := e.ensurePasswordInDatabaseInstance(ctx, database, mysqlServerConfigs.Items, pgServerConfigs.Items, username, password)
		if err != nil {
			return errors.Wrap(err)
		}
		if !configFound {
			logrus.Warningf("Missing database server config for db: %s", database)
			e.recorder.Eventf(&pod, v1.EventTypeWarning, ReasonEnsuringDatabasePasswordFailed,
				"Failed to ensure database password in %s. Missing database server config", database)
		}
	}
	return nil
}

func (e *Reconciler) RotateSecretsLoop(ctx context.Context) {
	refreshSecretsTicker := time.NewTicker(RefreshSecretsLoopTick)
	for {
		select {
		case <-refreshSecretsTicker.C:
			go func() {
				err := e.RotateSecretsAndAlterPasswords(ctx)
				if err != nil {
					logrus.WithError(err).Error("failed rotating user-password secrets")
				}
			}()
		case <-ctx.Done():
			return
		}
	}
}

func (e *Reconciler) RotateSecretsAndAlterPasswords(ctx context.Context) error {
	otterizeSecrets := v1.SecretList{}
	if err := e.client.List(ctx, &otterizeSecrets, &client.MatchingLabels{metadata.SecretTypeLabel: string(v1.SecretTypeOpaque)}); err != nil {
		return errors.Wrap(err)
	}
	secretsNeedingRotation := lo.Filter(otterizeSecrets.Items, func(secret v1.Secret, _ int) bool {
		return shouldRotateSecret(secret)
	})
	rotatedSecrets := make([]v1.Secret, 0)

	for _, secret := range secretsNeedingRotation {
		updatedSecret, err := e.rotateSecret(ctx, secret)
		if err != nil {
			e.recorder.Eventf(&secret, v1.EventTypeWarning, ReasonRotatingSecretFailed, "Failed to rotate secret: %s", err.Error())
			continue
		}
		rotatedSecrets = append(rotatedSecrets, updatedSecret)
		logrus.Infof("Rotated secret: %s.%s", secret.Name, secret.Namespace)
	}

	err := e.runAlterPasswordForSecrets(ctx, rotatedSecrets)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (e *Reconciler) rotateSecret(ctx context.Context, secret v1.Secret) (v1.Secret, error) {
	updatedSecret := secret.DeepCopy()
	password, err := databaseconfigurator.GenerateRandomPassword()
	if err != nil {
		return v1.Secret{}, errors.Wrap(err)
	}
	updatedSecret.Data["password"] = []byte(password)
	if updatedSecret.Annotations == nil {
		updatedSecret.Annotations = map[string]string{metadata.SecretLastUpdatedTimestampAnnotation: time.Now().Format(time.RFC3339)}
	} else {
		updatedSecret.Annotations[metadata.SecretLastUpdatedTimestampAnnotation] = time.Now().Format(time.RFC3339)
	}
	if err := e.client.Patch(ctx, updatedSecret, client.MergeFrom(&secret)); err != nil {
		return v1.Secret{}, errors.Wrap(err)
	}

	return *updatedSecret, nil
}

func (e *Reconciler) runAlterPasswordForSecrets(ctx context.Context, secrets []v1.Secret) error {
	pgServerConfigs := otterizev1alpha3.PostgreSQLServerConfigList{}
	err := e.client.List(ctx, &pgServerConfigs)
	if err != nil {
		return errors.Wrap(err)
	}

	mysqlServerConfigs := otterizev1alpha3.MySQLServerConfigList{}
	err = e.client.List(ctx, &mysqlServerConfigs)
	if err != nil {
		return errors.Wrap(err)
	}

	allConfigurators := e.GetAllDBConfigurators(ctx, mysqlServerConfigs.Items, pgServerConfigs.Items)
	defer closeAllConnections(ctx, allConfigurators)

	for _, secret := range secrets {
		username := string(secret.Data["username"])
		password := string(secret.Data["password"])
		for _, dbConfigurator := range allConfigurators {
			exists, err := dbConfigurator.ValidateUserExists(ctx, username)
			if err != nil {
				return errors.Wrap(err)
			}
			if !exists {
				continue
			}
			if err := dbConfigurator.AlterUserPassword(ctx, username, password); err != nil {
				logrus.WithError(err).Errorf("Failed to alter user-password for secret: %s", secret.Name)
			}
		}
	}

	return nil
}

func (e *Reconciler) createDBConfigurator(
	ctx context.Context,
	database string,
	mysqlServerConfigs []otterizev1alpha3.MySQLServerConfig,
	pgServerConfigs []otterizev1alpha3.PostgreSQLServerConfig) (databaseconfigurator.DatabaseConfigurator, bool, error) {

	mysqlConf, found := lo.Find(mysqlServerConfigs, func(config otterizev1alpha3.MySQLServerConfig) bool {
		return config.Name == database
	})
	if found {
		dbconfigurator, err := mysql.NewMySQLConfigurator(ctx, mysqlConf.Spec)
		if err != nil {
			return nil, false, errors.Wrap(err)
		}
		return dbconfigurator, true, nil
	}

	pgServerConf, found := lo.Find(pgServerConfigs, func(config otterizev1alpha3.PostgreSQLServerConfig) bool {
		return config.Name == database
	})
	if found {
		dbconfigurator, err := postgres.NewPostgresConfigurator(ctx, pgServerConf.Spec)
		if err != nil {
			return nil, false, errors.Wrap(err)
		}
		return dbconfigurator, true, nil
	}

	return nil, false, nil
}

func (e *Reconciler) ensurePasswordInDatabaseInstance(
	ctx context.Context,
	database string,
	mysqlServerConfigs []otterizev1alpha3.MySQLServerConfig,
	pgServerConfigs []otterizev1alpha3.PostgreSQLServerConfig,
	username string,
	password string) (bool, error) {

	dbConfigurator, found, err := e.createDBConfigurator(ctx, database, mysqlServerConfigs, pgServerConfigs)
	if err != nil {
		return false, errors.Wrap(err)
	}
	if !found {
		return false, nil
	}

	defer dbConfigurator.CloseConnection(ctx)
	if err := dbConfigurator.AlterUserPassword(ctx, username, password); err != nil {
		return false, errors.Wrap(err)
	}

	return true, nil
}
func closeAllConnections(ctx context.Context, allConfigurators []databaseconfigurator.DatabaseConfigurator) {
	for _, dbConfigurator := range allConfigurators {
		dbConfigurator.CloseConnection(ctx)
	}
}

func (e *Reconciler) GetAllDBConfigurators(ctx context.Context, mysqlServerConfigs []otterizev1alpha3.MySQLServerConfig, pgServerConfigs []otterizev1alpha3.PostgreSQLServerConfig) []databaseconfigurator.DatabaseConfigurator {
	configurators := make([]databaseconfigurator.DatabaseConfigurator, 0)
	for _, mysqlServerConfig := range mysqlServerConfigs {
		dbconfigurator, err := mysql.NewMySQLConfigurator(ctx, mysqlServerConfig.Spec)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to create configurator for MySQL server config: %s", mysqlServerConfig.Name)
			continue
		}
		configurators = append(configurators, dbconfigurator)
	}
	for _, pgServerConfig := range pgServerConfigs {
		dbconfigurator, err := postgres.NewPostgresConfigurator(ctx, pgServerConfig.Spec)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to create configurator for PostgreSQL server config: %s", pgServerConfig.Name)
			continue
		}
		configurators = append(configurators, dbconfigurator)
	}
	return configurators
}

func shouldRotateSecret(secret v1.Secret) bool {
	if secret.Annotations == nil {
		return true
	}
	lastUpdatedStr, ok := secret.Annotations[metadata.SecretLastUpdatedTimestampAnnotation]
	if !ok {
		return true
	}
	rotationInterval := viper.GetDuration(operatorconfig.DatabasePasswordRotationIntervalKey)
	lastUpdatedTime, err := time.Parse(time.RFC3339, lastUpdatedStr)
	if err != nil {
		logrus.WithError(err).Errorf("failed parsing last updated time from secret: %s.%s, will re-create it", secret.Name, secret.Namespace)
		return true
	}
	if lastUpdatedTime.Add(rotationInterval).Before(time.Now()) {
		return true
	}

	return false
}

func buildUserAndPasswordCredentialsSecret(name, namespace, pgUsername, password string) *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{metadata.SecretTypeLabel: string(v1.SecretTypeOpaque)},
			Annotations: map[string]string{
				metadata.SecretLastUpdatedTimestampAnnotation: time.Now().Format(time.RFC3339),
			},
		},
		Data: map[string][]byte{
			"username": []byte(pgUsername),
			"password": []byte(password),
		},
		Type: v1.SecretTypeOpaque,
	}
}
