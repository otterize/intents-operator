package intents_reconcilers

import (
	"context"
	"github.com/amit7itz/goset"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/protected_services"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ReasonCouldNotConnectToKafkaServer         = "CouldNotConnectToKafkaServer"
	ReasonCouldNotApplyIntentsOnKafkaServer    = "CouldNotApplyIntentsOnKafkaServer"
	ReasonKafkaACLCreationDisabled             = "KafkaACLCreationDisabled"
	ReasonKafkaServerNotConfigured             = "KafkaServerNotConfigured"
	ReasonRemovingKafkaACLsFailed              = "RemovingKafkaACLsFailed"
	ReasonApplyingKafkaACLsFailed              = "ApplyingKafkaACLsFailed"
	ReasonAppliedKafkaACLs                     = "AppliedKafkaACLs"
	ReasonIntentsOperatorIdentityResolveFailed = "IntentsOperatorIdentityResolveFailed"
)

type KafkaACLReconciler struct {
	client                  client.Client
	scheme                  *runtime.Scheme
	KafkaServersStore       kafkaacls.ServersStore
	enforcementDefaultState bool
	enableKafkaACLCreation  bool
	getNewKafkaIntentsAdmin kafkaacls.IntentsAdminFactoryFunction
	operatorPodName         string
	operatorPodNamespace    string
	serviceResolver         serviceidresolver.ServiceResolver
	activeNamespaces        *goset.Set[string]
	injectablerecorder.InjectableRecorder
}

func NewKafkaACLReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	serversStore kafkaacls.ServersStore,
	enableKafkaACLCreation bool,
	factoryFunc kafkaacls.IntentsAdminFactoryFunction,
	enforcementDefaultState bool,
	operatorPodName string,
	operatorPodNamespace string,
	serviceResolver serviceidresolver.ServiceResolver,
	activeNamespaces *goset.Set[string],
) *KafkaACLReconciler {
	return &KafkaACLReconciler{
		client:                  client,
		scheme:                  scheme,
		KafkaServersStore:       serversStore,
		enableKafkaACLCreation:  enableKafkaACLCreation,
		getNewKafkaIntentsAdmin: factoryFunc,
		enforcementDefaultState: enforcementDefaultState,
		operatorPodName:         operatorPodName,
		operatorPodNamespace:    operatorPodNamespace,
		serviceResolver:         serviceResolver,
		activeNamespaces:        activeNamespaces,
	}
}

func getIntentsByServer(defaultNamespace string, intents []otterizev2alpha1.Target) map[types.NamespacedName][]otterizev2alpha1.Target {
	intentsByServer := map[types.NamespacedName][]otterizev2alpha1.Target{}
	for _, intent := range intents {
		if intent.Kafka == nil {
			continue
		}

		serverName := types.NamespacedName{
			Name:      intent.GetTargetServerName(),
			Namespace: intent.GetTargetServerNamespace(defaultNamespace),
		}

		intentsByServer[serverName] = append(intentsByServer[serverName], intent)
	}

	return intentsByServer
}

func (r *KafkaACLReconciler) applyACLs(ctx context.Context, intents *otterizev2alpha1.ClientIntents) (serverCount int, err error) {
	intentsByServer := getIntentsByServer(intents.Namespace, intents.Spec.Targets)

	if err := r.KafkaServersStore.MapErr(func(serverName types.NamespacedName, config *otterizev2alpha1.KafkaServerConfig, tls otterizev2alpha1.TLSSource) error {
		intentsForServer := intentsByServer[serverName]
		shouldCreatePolicy, err := protected_services.IsServerEnforcementEnabledDueToProtectionOrDefaultState(ctx, r.client, serviceidentity.ServiceIdentity{Name: serverName.Name, Namespace: serverName.Namespace}, r.enforcementDefaultState, r.activeNamespaces)
		if err != nil {
			return errors.Wrap(err)
		}

		if !shouldCreatePolicy {
			logrus.Debugf("Enforcement is disabled globally and server is not explicitly protected, skipping Kafka ACL creation for server %s in namespace %s", serverName.Name, serverName.Namespace)
			r.RecordNormalEventf(intents, consts.ReasonEnforcementDefaultOff, "Enforcement is disabled globally and called service '%s' is not explicitly protected using a ProtectedService resource, Kafka ACL creation skipped", serverName.Name)
			// Intentionally no return - KafkaIntentsAdminImpl skips the creation, but still needs to do deletion.
		}
		kafkaIntentsAdmin, err := r.getNewKafkaIntentsAdmin(*config, tls, r.enableKafkaACLCreation, shouldCreatePolicy)
		if err != nil {
			err = errors.Errorf("failed to connect to Kafka server %s: %w", serverName, err)
			r.RecordWarningEventf(intents, ReasonCouldNotConnectToKafkaServer, "Kafka ACL reconcile failed: %s", err.Error())
			return errors.Wrap(err)
		}
		defer kafkaIntentsAdmin.Close()
		if err := kafkaIntentsAdmin.ApplyClientIntents(intents.Spec.Workload.Name, intents.Namespace, intentsForServer); err != nil {
			r.RecordWarningEventf(intents, ReasonCouldNotApplyIntentsOnKafkaServer, "Kafka ACL reconcile failed: %s", err.Error())
			return errors.Errorf("failed applying intents on kafka server %s: %w", serverName, err)
		}
		return nil
	}); err != nil {
		return 0, errors.Wrap(err)
	}

	if !r.enableKafkaACLCreation {
		r.RecordNormalEvent(intents, ReasonKafkaACLCreationDisabled, "Kafka ACL creation is disabled, creation skipped")
	}

	for serverName := range intentsByServer {
		if !r.KafkaServersStore.Exists(serverName.Name, serverName.Namespace) {
			r.RecordWarningEventf(intents, ReasonKafkaServerNotConfigured, "broker %s not configured", serverName)
			logrus.WithField("server", serverName).Warning("Did not apply intents to server - no server configuration was defined")
		}
	}

	return len(intentsByServer), nil
}

func (r *KafkaACLReconciler) RemoveACLs(ctx context.Context, intents *otterizev2alpha1.ClientIntents) error {
	return r.KafkaServersStore.MapErr(func(serverName types.NamespacedName, config *otterizev2alpha1.KafkaServerConfig, tls otterizev2alpha1.TLSSource) error {
		shouldCreatePolicy, err := protected_services.IsServerEnforcementEnabledDueToProtectionOrDefaultState(ctx, r.client, serviceidentity.ServiceIdentity{Name: serverName.Name, Namespace: serverName.Namespace}, r.enforcementDefaultState, r.activeNamespaces)
		if err != nil {
			return errors.Wrap(err)
		}

		// We just pass shouldCreatePolicy to the KafkaIntentsAdmin - it determines whether to create or delete.
		kafkaIntentsAdmin, err := r.getNewKafkaIntentsAdmin(*config, tls, r.enableKafkaACLCreation, shouldCreatePolicy)
		if err != nil {
			return errors.Wrap(err)
		}
		defer kafkaIntentsAdmin.Close()

		if err := kafkaIntentsAdmin.RemoveClientIntents(intents.Spec.Workload.Name, intents.Namespace); err != nil {
			return errors.Errorf("failed removing intents from kafka server %s: %w", serverName, err)
		}
		return nil
	})
}

func (r *KafkaACLReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	intents := &otterizev2alpha1.ClientIntents{}
	logger := logrus.WithField("namespacedName", req.String())
	err := r.client.Get(ctx, req.NamespacedName, intents, &client.GetOptions{Raw: &metav1.GetOptions{ResourceVersion: "v2alpha1"}})
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

	if r.intentsObjectUnderDeletion(intents) {
		return r.handleIntentsDeletion(ctx, intents, logger)
	}

	clientIsOperator, err := r.isIntentsForTheIntentsOperator(ctx, intents)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	if clientIsOperator {
		logger.Debug("Skipping ACLs creation for the intents operator")
		return ctrl.Result{}, nil
	}

	var result ctrl.Result
	result, err = r.applyAcls(ctx, logger, intents)
	if err != nil {
		return result, errors.Wrap(err)
	}

	return ctrl.Result{}, nil

}

func (r *KafkaACLReconciler) isIntentsForTheIntentsOperator(ctx context.Context, intents *otterizev2alpha1.ClientIntents) (bool, error) {
	if r.operatorPodNamespace != intents.Namespace {
		return false, nil
	}

	var operatorPod v1.Pod
	err := r.client.Get(ctx, types.NamespacedName{Name: r.operatorPodName, Namespace: r.operatorPodNamespace}, &operatorPod)
	if err != nil {
		return false, errors.Wrap(err)
	}
	annotatedServiceName, err := r.serviceResolver.ResolvePodToServiceIdentity(ctx, &operatorPod)
	if err != nil {
		return false, errors.Wrap(err)
	}

	return annotatedServiceName.Name == intents.Spec.Workload.Name, nil
}

func (r *KafkaACLReconciler) applyAcls(ctx context.Context, logger *logrus.Entry, intents *otterizev2alpha1.ClientIntents) (ctrl.Result, error) {
	logger.Debug("Applying new ACLs")
	serverCount, err := r.applyACLs(ctx, intents)
	if err != nil {
		r.RecordWarningEventf(intents, ReasonApplyingKafkaACLsFailed, "could not apply Kafka ACLs: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	if serverCount > 0 {
		r.RecordNormalEventf(intents, ReasonAppliedKafkaACLs, "Kafka ACL reconcile complete, reconciled %d Kafka brokers", serverCount)
	}
	return ctrl.Result{}, nil
}

func (r *KafkaACLReconciler) intentsObjectUnderDeletion(intents *otterizev2alpha1.ClientIntents) bool {
	return !intents.ObjectMeta.DeletionTimestamp.IsZero()
}

func (r *KafkaACLReconciler) handleIntentsDeletion(ctx context.Context, intents *otterizev2alpha1.ClientIntents, logger *logrus.Entry) (ctrl.Result, error) {
	logger.Debugf("Removing associated Acls")
	if err := r.RemoveACLs(ctx, intents); err != nil {
		r.RecordWarningEventf(intents, ReasonRemovingKafkaACLsFailed, "Could not remove Kafka ACLs: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	return ctrl.Result{}, nil
}
