package intents_reconcilers

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/operator/controllers/kafkaacls"
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

var finalizerName = "otterize-intents.kafka/finalizer"

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

	if err := r.KafkaServersStore.MapErr(func(serverName types.NamespacedName, kafkaIntentsAdmin *kafkaacls.KafkaIntentsAdmin) error {
		intentsForServer := intentsByServer[serverName]
		if err := kafkaIntentsAdmin.ApplyIntents(intents.Spec.Service.Name, intents.Namespace, intentsForServer); err != nil {
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
	return r.KafkaServersStore.MapErr(func(serverName types.NamespacedName, kafkaIntentsAdmin *kafkaacls.KafkaIntentsAdmin) error {
		if err := kafkaIntentsAdmin.RemoveClientIntents(intents.Spec.Service.Name, intents.Namespace); err != nil {
			return fmt.Errorf("failed removing intents from kafka server %s: %w", serverName, err)
		}
		return nil
	})
}

func (r *KafkaACLsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	intents := &otterizev1alpha1.Intents{}
	logger := logrus.WithField("namespaced_name", req.NamespacedName.String())
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
		if !controllerutil.ContainsFinalizer(intents, finalizerName) {
			logger.Infof("Adding finalizer %s", finalizerName)
			controllerutil.AddFinalizer(intents, finalizerName)
			if err := r.Update(ctx, intents); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(intents, finalizerName) {
			logger.Infof("Removing associated Acls")
			if err := r.RemoveACLs(intents); err != nil {
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(intents, finalizerName)
			if err := r.Update(ctx, intents); err != nil {
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
