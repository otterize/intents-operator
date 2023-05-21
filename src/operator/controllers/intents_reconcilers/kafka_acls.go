package intents_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	KafkaACLsFinalizerName                     = "intents.otterize.com/kafka-finalizer"
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
	client                     client.Client
	scheme                     *runtime.Scheme
	KafkaServersStore          kafkaacls.ServersStore
	enforcementEnabledGlobally bool
	enableKafkaACLCreation     bool
	getNewKafkaIntentsAdmin    kafkaacls.IntentsAdminFactoryFunction
	operatorPodName            string
	operatorPodNamespace       string
	serviceResolver            serviceidresolver.ServiceResolver
	injectablerecorder.InjectableRecorder
}

func NewKafkaACLReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	serversStore kafkaacls.ServersStore,
	enableKafkaACLCreation bool,
	factoryFunc kafkaacls.IntentsAdminFactoryFunction,
	enforcementEnabledGlobally bool,
	operatorPodName string,
	operatorPodNamespace string,
	serviceResolver serviceidresolver.ServiceResolver,
) *KafkaACLReconciler {
	return &KafkaACLReconciler{
		client:                     client,
		scheme:                     scheme,
		KafkaServersStore:          serversStore,
		enableKafkaACLCreation:     enableKafkaACLCreation,
		getNewKafkaIntentsAdmin:    factoryFunc,
		enforcementEnabledGlobally: enforcementEnabledGlobally,
		operatorPodName:            operatorPodName,
		operatorPodNamespace:       operatorPodNamespace,
		serviceResolver:            serviceResolver,
	}
}

func getIntentsByServer(defaultNamespace string, intents []otterizev1alpha2.Intent) map[types.NamespacedName][]otterizev1alpha2.Intent {
	intentsByServer := map[types.NamespacedName][]otterizev1alpha2.Intent{}
	for _, intent := range intents {
		if intent.Type != otterizev1alpha2.IntentTypeKafka {
			continue
		}

		serverName := types.NamespacedName{
			Name:      intent.GetServerName(),
			Namespace: intent.GetServerNamespace(defaultNamespace),
		}

		intentsByServer[serverName] = append(intentsByServer[serverName], intent)
	}

	return intentsByServer
}

func (r *KafkaACLReconciler) applyACLs(intents *otterizev1alpha2.ClientIntents) (serverCount int, err error) {
	intentsByServer := getIntentsByServer(intents.Namespace, intents.Spec.Calls)

	if err := r.KafkaServersStore.MapErr(func(serverName types.NamespacedName, config *otterizev1alpha2.KafkaServerConfig, tls otterizev1alpha2.TLSSource) error {
		kafkaIntentsAdmin, err := r.getNewKafkaIntentsAdmin(*config, tls, r.enableKafkaACLCreation, r.enforcementEnabledGlobally)
		if err != nil {
			err = fmt.Errorf("failed to connect to Kafka server %s: %w", serverName, err)
			r.RecordWarningEventf(intents, ReasonCouldNotConnectToKafkaServer, "Kafka ACL reconcile failed: %s", err.Error())
			return err
		}
		defer kafkaIntentsAdmin.Close()

		intentsForServer := intentsByServer[serverName]
		if err := kafkaIntentsAdmin.ApplyClientIntents(intents.Spec.Service.Name, intents.Namespace, intentsForServer); err != nil {
			r.RecordWarningEventf(intents, ReasonCouldNotApplyIntentsOnKafkaServer, "Kafka ACL reconcile failed: %s", err.Error())
			return fmt.Errorf("failed applying intents on kafka server %s: %w", serverName, err)
		}
		return nil
	}); err != nil {
		return 0, err
	}

	if !r.enforcementEnabledGlobally {
		r.RecordNormalEvent(intents, ReasonEnforcementGloballyDisabled, "Enforcement is disabled globally, Kafka ACL creation skipped")
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

func (r *KafkaACLReconciler) RemoveACLs(intents *otterizev1alpha2.ClientIntents) error {
	return r.KafkaServersStore.MapErr(func(serverName types.NamespacedName, config *otterizev1alpha2.KafkaServerConfig, tls otterizev1alpha2.TLSSource) error {
		kafkaIntentsAdmin, err := r.getNewKafkaIntentsAdmin(*config, tls, r.enableKafkaACLCreation, r.enforcementEnabledGlobally)
		if err != nil {
			return err
		}
		defer kafkaIntentsAdmin.Close()

		if err := kafkaIntentsAdmin.RemoveClientIntents(intents.Spec.Service.Name, intents.Namespace); err != nil {
			return fmt.Errorf("failed removing intents from kafka server %s: %w", serverName, err)
		}
		return nil
	})
}

func (r *KafkaACLReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	intents := &otterizev1alpha2.ClientIntents{}
	logger := logrus.WithField("namespacedName", req.String())
	err := r.client.Get(ctx, req.NamespacedName, intents)
	if err != nil && k8serrors.IsNotFound(err) {
		logger.Info("No intents found")
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	if intents.Spec == nil {
		logger.Info("No specs found")
		return ctrl.Result{}, nil
	}

	if r.intentsObjectUnderDeletion(intents) {
		return r.handleIntentsDeletion(ctx, intents, logger)
	}

	if r.isMissingKafkaFinalizer(intents) {
		logger.Infof("Adding finalizer %s", KafkaACLsFinalizerName)
		controllerutil.AddFinalizer(intents, KafkaACLsFinalizerName)
		if err := r.client.Update(ctx, intents); err != nil {
			return ctrl.Result{}, err
		}
	}

	clientIsOperator, err := r.isIntentsForTheIntentsOperator(ctx, intents)
	if err != nil {
		return ctrl.Result{}, err
	}

	if clientIsOperator {
		logger.Info("Skipping ACLs creation for the intents operator")
		return ctrl.Result{}, nil
	}

	var result ctrl.Result
	result, err = r.applyAcls(logger, intents)
	if err != nil {
		return result, err
	}

	return ctrl.Result{}, nil

}

func (r *KafkaACLReconciler) isIntentsForTheIntentsOperator(ctx context.Context, intents *otterizev1alpha2.ClientIntents) (bool, error) {
	if r.operatorPodNamespace != intents.Namespace {
		return false, nil
	}

	name, ok, err := r.serviceResolver.GetPodAnnotatedName(ctx, r.operatorPodName, r.operatorPodNamespace)
	if err != nil {
		return false, fmt.Errorf("failed resolving intents operator identity - %w", err)
	}

	if !ok {
		r.RecordWarningEventf(intents, ReasonIntentsOperatorIdentityResolveFailed, "failed resolving intents operator identity - service name annotation required")
		return false, fmt.Errorf("failed resolving intents operator identity - service name annotation required")
	}

	return name == intents.Spec.Service.Name, nil
}

func (r *KafkaACLReconciler) applyAcls(logger *logrus.Entry, intents *otterizev1alpha2.ClientIntents) (ctrl.Result, error) {
	logger.Info("Applying new ACLs")
	serverCount, err := r.applyACLs(intents)
	if err != nil {
		r.RecordWarningEventf(intents, ReasonApplyingKafkaACLsFailed, "could not apply Kafka ACLs: %s", err.Error())
		return ctrl.Result{}, err
	}

	if serverCount > 0 {
		r.RecordNormalEventf(intents, ReasonAppliedKafkaACLs, "Kafka ACL reconcile complete, reconciled %d Kafka brokers", serverCount)
	}
	return ctrl.Result{}, nil
}

func (r *KafkaACLReconciler) isMissingKafkaFinalizer(intents *otterizev1alpha2.ClientIntents) bool {
	return !controllerutil.ContainsFinalizer(intents, KafkaACLsFinalizerName) && intents.HasKafkaTypeInCallList()
}

func (r *KafkaACLReconciler) intentsObjectUnderDeletion(intents *otterizev1alpha2.ClientIntents) bool {
	return !intents.ObjectMeta.DeletionTimestamp.IsZero()
}

func (r *KafkaACLReconciler) handleIntentsDeletion(ctx context.Context, intents *otterizev1alpha2.ClientIntents, logger *logrus.Entry) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(intents, KafkaACLsFinalizerName) {
		logger.Infof("Removing associated Acls")
		if err := r.RemoveACLs(intents); err != nil {
			r.RecordWarningEventf(intents, ReasonRemovingKafkaACLsFailed, "Could not remove Kafka ACLs: %s", err.Error())
			return ctrl.Result{}, err
		}

		removeIntentFinalizers(intents, KafkaACLsFinalizerName)
		if err := r.client.Update(ctx, intents); err != nil {
			if k8serrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}
