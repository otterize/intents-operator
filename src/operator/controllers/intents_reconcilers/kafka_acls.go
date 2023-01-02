package intents_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	KafkaACLsFinalizerName                  = "intents.otterize.com/kafka-finalizer"
	ReasonCouldNotConnectToKafkaServer      = "CouldNotConnectToKafkaServer"
	ReasonCouldNotApplyIntentsOnKafkaServer = "CouldNotApplyIntentsOnKafkaServer"
	ReasonEnforcementDisabledGlobally       = "EnforcementDisabledGlobally"
	ReasonKafkaACLCreationDisabled          = "KafkaACLCreationDisabled"
	ReasonKafkaServerNotConfigured          = "KafkaServerNotConfigured"
	ReasonRemovingKafkaACLsFailed           = "RemovingKafkaACLsFailed"
	ReasonApplyingKafkaACLsFailed           = "ApplyingKafkaACLsFailed"
	ReasonAppliedKafkaACLs                  = "AppliedKafkaACLs"
)

type KafkaACLReconciler struct {
	client                  client.Client
	scheme                  *runtime.Scheme
	KafkaServersStore       kafkaacls.ServersStore
	globalEnforceSetting    bool
	enableKafkaACLCreation  bool
	getNewKafkaIntentsAdmin kafkaacls.IntentsAdminFactoryFunction
	injectablerecorder.InjectableRecorder
}

func NewKafkaACLReconciler(client client.Client, scheme *runtime.Scheme, serversStore kafkaacls.ServersStore, enableKafkaACLCreation bool, factoryFunc kafkaacls.IntentsAdminFactoryFunction, globalEnforceSetting bool) *KafkaACLReconciler {
	return &KafkaACLReconciler{
		client:                  client,
		scheme:                  scheme,
		KafkaServersStore:       serversStore,
		enableKafkaACLCreation:  enableKafkaACLCreation,
		getNewKafkaIntentsAdmin: factoryFunc,
		globalEnforceSetting:    globalEnforceSetting,
	}
}

func getIntentsByServer(defaultNamespace string, intents []otterizev1alpha1.Intent) map[types.NamespacedName][]otterizev1alpha1.Intent {
	intentsByServer := map[types.NamespacedName][]otterizev1alpha1.Intent{}
	for _, intent := range intents {
		if intent.Type != otterizev1alpha1.IntentTypeKafka {
			continue
		}

		serverName := types.NamespacedName{
			Name:      intent.Name,
			Namespace: lo.Ternary(intent.Namespace != "", intent.Namespace, defaultNamespace),
		}

		intentsByServer[serverName] = append(intentsByServer[serverName], intent)
	}

	return intentsByServer
}

func (r *KafkaACLReconciler) applyACLs(intents *otterizev1alpha1.ClientIntents) (serverCount int, err error) {
	intentsByServer := getIntentsByServer(intents.Namespace, intents.Spec.Calls)

	if err := r.KafkaServersStore.MapErr(func(serverName types.NamespacedName, config *otterizev1alpha1.KafkaServerConfig, tls otterizev1alpha1.TLSSource) error {
		kafkaIntentsAdmin, err := r.getNewKafkaIntentsAdmin(*config, tls, r.enableKafkaACLCreation, r.globalEnforceSetting)
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

	if !r.globalEnforceSetting {
		r.RecordNormalEvent(intents, ReasonEnforcementGloballyDisabled, "Enforcement is disabled globally, Kafka ACL creation skipped")
	}
	if !r.enableKafkaACLCreation {
		r.RecordNormalEvent(intents, ReasonKafkaACLCreationDisabled, "Kafka ACL creation is disabled, creation skipped")
	}

	for serverName, _ := range intentsByServer {
		if !r.KafkaServersStore.Exists(serverName.Name, serverName.Namespace) {
			r.RecordWarningEventf(intents, ReasonKafkaServerNotConfigured, "broker %s not configured", serverName)
			logrus.WithField("server", serverName).Warning("Did not apply intents to server - no server configuration was defined")
		}
	}

	return len(intentsByServer), nil
}

func (r *KafkaACLReconciler) RemoveACLs(intents *otterizev1alpha1.ClientIntents) error {
	return r.KafkaServersStore.MapErr(func(serverName types.NamespacedName, config *otterizev1alpha1.KafkaServerConfig, tls otterizev1alpha1.TLSSource) error {
		kafkaIntentsAdmin, err := r.getNewKafkaIntentsAdmin(*config, tls, r.enableKafkaACLCreation, r.globalEnforceSetting)
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
	intents := &otterizev1alpha1.ClientIntents{}
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

	var result ctrl.Result
	result, err = r.applyAcls(logger, err, intents)
	if err != nil {
		return result, err
	}

	return ctrl.Result{}, nil

}

func (r *KafkaACLReconciler) applyAcls(logger *logrus.Entry, err error, intents *otterizev1alpha1.ClientIntents) (ctrl.Result, error) {
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

func (r *KafkaACLReconciler) isMissingKafkaFinalizer(intents *otterizev1alpha1.ClientIntents) bool {
	return !controllerutil.ContainsFinalizer(intents, KafkaACLsFinalizerName) && intents.HasKafkaTypeInCallList()
}

func (r *KafkaACLReconciler) intentsObjectUnderDeletion(intents *otterizev1alpha1.ClientIntents) bool {
	return !intents.ObjectMeta.DeletionTimestamp.IsZero()
}

func (r *KafkaACLReconciler) handleIntentsDeletion(ctx context.Context, intents *otterizev1alpha1.ClientIntents, logger *logrus.Entry) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(intents, KafkaACLsFinalizerName) {
		logger.Infof("Removing associated Acls")
		if err := r.RemoveACLs(intents); err != nil {
			r.RecordWarningEventf(intents, ReasonRemovingKafkaACLsFailed, "Could not remove Kafka ACLs: %s", err.Error())
			return ctrl.Result{}, err
		}

		controllerutil.RemoveFinalizer(intents, KafkaACLsFinalizerName)
		if err := r.client.Update(ctx, intents); err != nil {
			if k8serrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}
