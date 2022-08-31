package intents_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha1 "github.com/otterize/intents-operator/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/operator/controllers/kafkaacls"
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
	Scheme            *runtime.Scheme
	KafkaServersStore *kafkaacls.ServersStore
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

func (r *KafkaACLsReconciler) applyACLs(intents *otterizev1alpha1.Intents) error {
	intentsByServer := getIntentsByServer(intents.Namespace, intents.Spec.Service.Calls)

	if err := r.KafkaServersStore.MapErr(func(serverName types.NamespacedName, config *otterizev1alpha1.KafkaServerConfig) error {
		kafkaIntentsAdmin, err := kafkaacls.NewKafkaIntentsAdmin(*config)
		if err != nil {
			return err
		}
		defer kafkaIntentsAdmin.Close()

		intentsForServer := intentsByServer[serverName]
		if err := kafkaIntentsAdmin.ApplyClientIntents(intents.Spec.Service.Name, intents.Namespace, intentsForServer); err != nil {
			return fmt.Errorf("failed applying intents on kafka server %s: %w", serverName, err)
		}
		return nil
	}); err != nil {
		return err
	}

	for serverName, _ := range intentsByServer {
		if !r.KafkaServersStore.Exists(serverName.Name, serverName.Namespace) {
			logrus.WithField("server", serverName).Warning("Did not apply intents to server - no server configuration was defined")
		}
	}

	return nil
}

func (r *KafkaACLsReconciler) RemoveACLs(intents *otterizev1alpha1.Intents) error {
	return r.KafkaServersStore.MapErr(func(serverName types.NamespacedName, config *otterizev1alpha1.KafkaServerConfig) error {
		kafkaIntentsAdmin, err := kafkaacls.NewKafkaIntentsAdmin(*config)
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
