package poduserpassword

import (
	"context"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/otterize/credentials-operator/src/operatorconfig"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/clusterutils"
	"github.com/otterize/intents-operator/src/shared/databaseconfigurator"
	"github.com/otterize/intents-operator/src/shared/databaseconfigurator/mysql"
	"github.com/otterize/intents-operator/src/shared/databaseconfigurator/postgres"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strings"
	"time"
)

const (
	ReasonEnsuredPodUserAndPassword              = "EnsuredPodUserAndPassword"
	ReasonGeneratePodDatabaseUserFailed          = "GeneratePodDatabaseUserFailed"
	ReasonEnsuringPodUserAndPasswordFailed       = "EnsuringPodUserAndPasswordFailed"
	ReasonEnsuringDatabasePasswordFailed         = "EnsuringDatabasePasswordFailed"
	ReasonEnsuredDatabasePassword                = "EnsuredPasswordInDatabases"
	ReasonRotatingSecretFailed                   = "RotatingSecretFailed"
	ReasonRestartingPodAfterSecretRotationFailed = "RestartingPodAfterSecretRotationFailed"
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
		Watches(&v1.Secret{}, handler.EnqueueRequestsFromMapFunc(e.watchSecretDeletion)).
		For(&v1.Pod{}).
		Complete(e)
}

func (e *Reconciler) shouldHandleCredentialsForPod(pod v1.Pod) bool {
	return pod.Annotations != nil && hasUserAndPasswordSecretAnnotation(pod)
}

func hasRestartAnnotation(pod v1.Pod) bool {
	_, ok := pod.Annotations[metadata.RestartOnSecretRotation]
	return ok
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
	// Fix for when the namespace is terminating but pods aren't getting the delete trigger just yet
	namespace := v1.Namespace{}
	if err := e.client.Get(ctx, types.NamespacedName{Name: req.Namespace}, &namespace); err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	if !namespace.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

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

	secretName := pod.Annotations[metadata.UserAndPasswordSecretNameAnnotation]
	logrus.Debug("Ensuring user-password credentials secrets for pod")
	result, created, password, err := e.ensurePodUserAndPasswordSecret(ctx, &pod, secretName, username)
	if err != nil {
		e.recorder.Eventf(&pod, v1.EventTypeWarning, ReasonEnsuringPodUserAndPasswordFailed, "Failed to ensure user-password credentials secret: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	if result.Requeue {
		return result, nil
	}

	e.recorder.Event(&pod, v1.EventTypeNormal, ReasonEnsuredPodUserAndPassword, "Ensured user-password credentials in specified secret")

	if hasDatabaseAccessAnnotation(pod) {
		logrus.Debug("Validating password in all databases")
		err = e.ensurePasswordInDatabases(ctx, pod, username, password)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err)
		}
		e.recorder.Event(&pod, v1.EventTypeNormal, ReasonEnsuredDatabasePassword, "Ensured password in databases")
	}

	if created && hasRestartAnnotation(pod) {
		logrus.Debug("Triggering pod restart for newly created secret")
		err := e.TriggerPodRestart(ctx, &pod)
		if err != nil {
			e.recorder.Eventf(&pod, v1.EventTypeWarning,
				ReasonRestartingPodAfterSecretRotationFailed, "Failed restarting pod after secret creation: %s", err.Error())
		}
	}

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

func (e *Reconciler) ensurePodUserAndPasswordSecret(ctx context.Context, pod *v1.Pod, secretName string, username string) (ctrl.Result, bool, string, error) {
	log := logrus.WithFields(logrus.Fields{"pod": pod.Name, "namespace": pod.Namespace})
	secret := v1.Secret{}
	err := e.client.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: secretName}, &secret)

	if apierrors.IsNotFound(err) {
		log.Debug("Creating user-password credentials secret for pod")
		password, err := databaseconfigurator.GenerateRandomPassword()
		if err != nil {
			return ctrl.Result{}, false, "", errors.Wrap(err)
		}

		secret := buildUserAndPasswordCredentialsSecret(secretName, pod.Namespace, username, password)
		log.WithField("secret", secretName).Debug("Creating new secret with user-password credentials")
		if err := e.client.Create(ctx, secret); err != nil {
			return ctrl.Result{}, false, "", errors.Wrap(err)
		}
		return ctrl.Result{}, true, password, nil
	}

	if err != nil {
		return ctrl.Result{}, false, "", errors.Wrap(err)
	}

	// If secret exists but is being deleted we need to requeue until it is deleted
	if !secret.DeletionTimestamp.IsZero() {
		logrus.Debug("Secret is being deleted, retriggering reconcile")
		return ctrl.Result{Requeue: true}, false, "", nil
	}

	log.Debug("Secret exists, nothing to do")
	return ctrl.Result{}, false, string(secret.Data["password"]), nil
}

func (e *Reconciler) ensurePasswordInDatabases(ctx context.Context, pod v1.Pod, username string, password string) error {
	databases := strings.Split(pod.Annotations[databaseconfigurator.DatabaseAccessAnnotation], ",")
	pgServerConfigs := otterizev2alpha1.PostgreSQLServerConfigList{}
	err := e.client.List(ctx, &pgServerConfigs)
	if err != nil {
		return errors.Wrap(err)
	}

	mysqlServerConfigs := otterizev2alpha1.MySQLServerConfigList{}
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

	if err := e.runAlterPasswordForSecrets(ctx, rotatedSecrets); err != nil {
		return errors.Wrap(err)
	}

	if err := e.handlePodRestartsForRotatedSecrets(ctx, rotatedSecrets); err != nil {
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
	if err := e.client.Patch(ctx, updatedSecret, client.StrategicMergeFrom(&secret)); err != nil {
		return v1.Secret{}, errors.Wrap(err)
	}

	return *updatedSecret, nil
}

func (e *Reconciler) runAlterPasswordForSecrets(ctx context.Context, secrets []v1.Secret) error {
	pgServerConfigs := otterizev2alpha1.PostgreSQLServerConfigList{}
	err := e.client.List(ctx, &pgServerConfigs)
	if err != nil {
		return errors.Wrap(err)
	}

	mysqlServerConfigs := otterizev2alpha1.MySQLServerConfigList{}
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

func (e *Reconciler) extractDBCredentials(ctx context.Context, namespace string, credentialsSpec otterizev2alpha1.DatabaseCredentials) (databaseconfigurator.DatabaseCredentials, error) {
	creds := databaseconfigurator.DatabaseCredentials{}
	if credentialsSpec.Username != "" {
		creds.Username = credentialsSpec.Username
	}
	if credentialsSpec.Password != "" {
		creds.Password = credentialsSpec.Password
	}
	if credentialsSpec.SecretRef != nil {
		secret := v1.Secret{}
		name := credentialsSpec.SecretRef.Name
		if credentialsSpec.SecretRef.Namespace != "" {
			namespace = credentialsSpec.SecretRef.Namespace
		}
		err := e.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &secret)
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

func (e *Reconciler) createPostgresDBConfigurator(ctx context.Context, pgServerConfig otterizev2alpha1.PostgreSQLServerConfig) (databaseconfigurator.DatabaseConfigurator, error) {
	credentials, err := e.extractDBCredentials(ctx, pgServerConfig.Namespace, pgServerConfig.Spec.Credentials)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	dbInfo := postgres.PostgresDatabaseInfo{
		Credentials: credentials,
		Address:     pgServerConfig.Spec.Address,
	}

	dbconfigurator, err := postgres.NewPostgresConfigurator(ctx, dbInfo)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	return dbconfigurator, nil
}

func (e *Reconciler) createMySQLDBConfigurator(ctx context.Context, mySQLServerConfig otterizev2alpha1.MySQLServerConfig) (databaseconfigurator.DatabaseConfigurator, error) {
	credentials, err := e.extractDBCredentials(ctx, mySQLServerConfig.Namespace, mySQLServerConfig.Spec.Credentials)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	dbInfo := mysql.MySQLDatabaseInfo{
		Credentials: credentials,
		Address:     mySQLServerConfig.Spec.Address,
	}

	dbconfigurator, err := mysql.NewMySQLConfigurator(ctx, dbInfo)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	return dbconfigurator, nil
}

func (e *Reconciler) createDBConfigurator(
	ctx context.Context,
	database string,
	mysqlServerConfigs []otterizev2alpha1.MySQLServerConfig,
	pgServerConfigs []otterizev2alpha1.PostgreSQLServerConfig) (databaseconfigurator.DatabaseConfigurator, bool, error) {

	mysqlServerConf, found := lo.Find(mysqlServerConfigs, func(config otterizev2alpha1.MySQLServerConfig) bool {
		return config.Name == database
	})
	if found {
		dbconfigurator, err := e.createMySQLDBConfigurator(ctx, mysqlServerConf)
		if err != nil {
			return nil, false, errors.Wrap(err)
		}
		return dbconfigurator, true, nil
	}

	pgServerConf, found := lo.Find(pgServerConfigs, func(config otterizev2alpha1.PostgreSQLServerConfig) bool {
		return config.Name == database
	})
	if found {
		dbconfigurator, err := e.createPostgresDBConfigurator(ctx, pgServerConf)
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
	mysqlServerConfigs []otterizev2alpha1.MySQLServerConfig,
	pgServerConfigs []otterizev2alpha1.PostgreSQLServerConfig,
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

func (e *Reconciler) GetAllDBConfigurators(ctx context.Context, mysqlServerConfigs []otterizev2alpha1.MySQLServerConfig, pgServerConfigs []otterizev2alpha1.PostgreSQLServerConfig) []databaseconfigurator.DatabaseConfigurator {
	configurators := make([]databaseconfigurator.DatabaseConfigurator, 0)
	for _, mysqlServerConfig := range mysqlServerConfigs {
		dbconfigurator, err := e.createMySQLDBConfigurator(ctx, mysqlServerConfig)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to create configurator for MySQL server config: %s", mysqlServerConfig.Name)
			continue
		}
		configurators = append(configurators, dbconfigurator)
	}

	for _, pgServerConfig := range pgServerConfigs {
		dbconfigurator, err := e.createPostgresDBConfigurator(ctx, pgServerConfig)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to create configurator for PostgreSQL server config: %s", pgServerConfig.Name)
			continue
		}
		configurators = append(configurators, dbconfigurator)
	}
	return configurators
}

func (e *Reconciler) TriggerPodRestart(ctx context.Context, pod *v1.Pod) error {
	owner, err := e.serviceIdResolver.GetOwnerObject(ctx, pod)
	if err != nil {
		return errors.Wrap(err)
	}
	kind := owner.GetObjectKind().GroupVersionKind().Kind
	logrus.Infof("Trying to trigger restart for workload '%s' of kind %s", owner.GetName(), kind)
	switch kind {
	case "Deployment":
		deployment := appsv1.Deployment{}
		if err := e.client.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: owner.GetName()}, &deployment); err != nil {
			return errors.Wrap(err)
		}
		deployment.Spec.Template.Annotations[metadata.TLSRestartTimeAfterRenewal] = time.Now().Format(time.RFC3339)
		if err := e.client.Update(ctx, &deployment); err != nil {
			return errors.Wrap(err)
		}
	case "ReplicaSet":
		replicaSet := appsv1.ReplicaSet{}
		if err := e.client.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: owner.GetName()}, &replicaSet); err != nil {
			return errors.Wrap(err)
		}
		replicaSet.Spec.Template.Annotations[metadata.TLSRestartTimeAfterRenewal] = time.Now().Format(time.RFC3339)
		if err := e.client.Update(ctx, &replicaSet); err != nil {
			return errors.Wrap(err)
		}
	case "StatefulSet":
		statefulSet := appsv1.StatefulSet{}
		if err := e.client.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: owner.GetName()}, &statefulSet); err != nil {
			return errors.Wrap(err)
		}
		statefulSet.Spec.Template.Annotations[metadata.TLSRestartTimeAfterRenewal] = time.Now().Format(time.RFC3339)
		if err := e.client.Update(ctx, &statefulSet); err != nil {
			return errors.Wrap(err)
		}
	case "DaemonSet":
		daemonSet := appsv1.DaemonSet{}
		if err := e.client.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: owner.GetName()}, &daemonSet); err != nil {
			return errors.Wrap(err)
		}
		daemonSet.Spec.Template.Annotations[metadata.TLSRestartTimeAfterRenewal] = time.Now().Format(time.RFC3339)
		if err := e.client.Update(ctx, &daemonSet); err != nil {
			return errors.Wrap(err)
		}
	default:
		// Rougher way - trigger delete on all pods in the namespace with the same labels - will restart just the owner's pods
		err := e.client.DeleteAllOf(ctx, &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{Namespace: pod.Namespace},
		}, client.MatchingLabels(pod.Labels))
		if err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (e *Reconciler) handlePodRestartsForRotatedSecrets(ctx context.Context, rotatedSecrets []v1.Secret) error {
	for _, secret := range rotatedSecrets {
		podList := &v1.PodList{}
		if err := e.client.List(ctx, podList, &client.ListOptions{Namespace: secret.Namespace}); err != nil {
			return errors.Wrap(err)
		}
		for _, pod := range podList.Items {
			secretName, ok := pod.Annotations[metadata.UserAndPasswordSecretNameAnnotation]
			if ok && secretName == secret.Name {
				if hasRestartAnnotation(pod) {
					logrus.Info("Triggering pod restart after secret rotation")
					err := e.TriggerPodRestart(ctx, &pod)
					if err != nil {
						e.recorder.Eventf(&pod, v1.EventTypeWarning,
							ReasonRestartingPodAfterSecretRotationFailed, "Failed restarting pod after secret rotation: %s", err.Error())
					}
					// A single restart trigger should handle all pod replicas, we can break
					// If we failed, we continue to try and restart pods for other rotated secrets
					break
				}
			}
		}
	}

	return nil
}

func (e *Reconciler) watchSecretDeletion(ctx context.Context, object client.Object) []reconcile.Request {
	namespace := v1.Namespace{}
	err := e.client.Get(ctx, types.NamespacedName{Name: object.GetNamespace()}, &namespace)
	if err != nil {
		logrus.WithError(err).Error("Failed to get namespace")
		return nil
	}
	if !namespace.DeletionTimestamp.IsZero() {
		// Skip deleting namespaces
		return nil
	}

	secret := v1.Secret{}
	err = e.client.Get(ctx, types.NamespacedName{Namespace: object.GetNamespace(), Name: object.GetName()}, &secret)
	if err != nil && !apierrors.IsNotFound(err) {
		logrus.WithError(err).Error("Failed to get secret")
		return nil
	}

	// Only call reconcile if the secret is being deleted or is not found anymore
	if !secret.DeletionTimestamp.IsZero() || apierrors.IsNotFound(err) {
		podList := v1.PodList{}
		if err := e.client.List(ctx, &podList, &client.ListOptions{Namespace: object.GetNamespace()}); err != nil {
			logrus.WithError(err).Error("Failed to list pods")
			return nil
		}

		if len(podList.Items) == 0 {
			return nil
		}

		result := lo.Filter(podList.Items, func(pod v1.Pod, _ int) bool {
			secretName, ok := pod.Annotations[metadata.UserAndPasswordSecretNameAnnotation]
			return ok && secretName == object.GetName()
		})

		if len(result) > 0 {
			// Even if more than 1 pods match the criteria, we just need to enqueue reconciliation for one
			pod := result[0]
			logrus.Infof("Enqueueing pod reconciliation: %s for deleted secret: %s", pod.Name, object.GetName())
			return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}}}
		}
	}

	return nil
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
