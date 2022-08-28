package kafka_acls

import (
	"context"
	"fmt"
	otterizev1alpha1 "github.com/otterize/intents-operator/shared/api/v1alpha1"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const FinalizerName = "otterize-intents.kafka/finalizer"

type KafkaACLsReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	KafkaServers []otterizev1alpha1.KafkaServer
}

func getIntentsByServer(defaultNamespace string, intents []otterizev1alpha1.Intent) map[types.NamespacedName][]otterizev1alpha1.Intent {
	intentsByServer := map[types.NamespacedName][]otterizev1alpha1.Intent{}
	for _, intent := range intents {
		serverName := types.NamespacedName{
			Name:      intent.Server,
			Namespace: lo.Ternary(intent.Namespace != "", intent.Namespace, defaultNamespace),
		}

		intentsByServer[serverName] = append(intentsByServer[serverName], intent)
	}

	return intentsByServer
}

func (r *KafkaACLsReconciler) logMissingIntentServers(intentsByServer map[types.NamespacedName][]otterizev1alpha1.Intent) {
	serversByName := map[types.NamespacedName]otterizev1alpha1.KafkaServer{}
	for _, kafkaServer := range r.KafkaServers {
		serverName := types.NamespacedName{Name: kafkaServer.Name, Namespace: kafkaServer.Namespace}
		serversByName[serverName] = kafkaServer
	}

	for serverName, _ := range intentsByServer {
		if _, ok := serversByName[serverName]; !ok {
			logrus.WithField("server", serverName).Warning("Did not apply intents to server - no server configuration was defined")
		}
	}
}

func (r *KafkaACLsReconciler) applyACLs(intents *otterizev1alpha1.Intents) error {
	intentsByServer := getIntentsByServer(intents.Namespace, intents.Spec.Service.Calls)

	for _, kafkaServer := range r.KafkaServers {
		serverName := types.NamespacedName{Name: kafkaServer.Name, Namespace: kafkaServer.Namespace}

		kafkaIntentsAdmin, err := NewKafkaIntentsAdmin(kafkaServer)
		if err != nil {
			return fmt.Errorf("failed connecting to kafka server %s: %w", serverName, err)
		}

		intentsForServer := intentsByServer[serverName]

		if err := kafkaIntentsAdmin.ApplyIntents(intents.Spec.Service.Name, intents.Namespace, intentsForServer); err != nil {
			return fmt.Errorf("failed applying intents on kafka server %s: %w", serverName, err)
		}
	}

	r.logMissingIntentServers(intentsByServer)

	return nil
}

func (r *KafkaACLsReconciler) RemoveACLs(intents *otterizev1alpha1.Intents) error {
	for _, kafkaServer := range r.KafkaServers {
		serverName := types.NamespacedName{Name: kafkaServer.Name, Namespace: kafkaServer.Namespace}

		kafkaIntentsAdmin, err := NewKafkaIntentsAdmin(kafkaServer)
		if err != nil {
			return fmt.Errorf("failed connecting to kafka server %s: %w", serverName, err)
		}

		if err := kafkaIntentsAdmin.RemoveClientIntents(intents.Spec.Service.Name, intents.Namespace); err != nil {
			return fmt.Errorf("failed applying intents on kafka server %s: %w", serverName, err)
		}
	}
	return nil
}

func (r *KafkaACLsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	intents := &otterizev1alpha1.Intents{}
	logger := logrus.WithField("namespacedName", req.String())
	err := r.Get(ctx, req.NamespacedName, intents)
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

	// examine DeletionTimestamp to determine if object is under deletion
	if intents.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(intents, FinalizerName) {
			logger.Infof("Adding finalizer %s", FinalizerName)
			controllerutil.AddFinalizer(intents, FinalizerName)
			if err := r.Update(ctx, intents); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(intents, FinalizerName) {
			logger.Infof("Removing associated Acls")
			if err := r.RemoveACLs(intents); err != nil {
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(intents, FinalizerName)
			if err := r.Update(ctx, intents); err != nil {
				if k8serrors.IsConflict(err) {
					return ctrl.Result{Requeue: true}, nil
				}
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
	}

	logger.Info("Applying new ACLs")
	if err := r.applyACLs(intents); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil

}
