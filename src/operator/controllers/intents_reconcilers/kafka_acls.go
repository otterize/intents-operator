package intents_reconcilers

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/operator/controllers/kafkaacls"
	otterizev1alpha1 "github.com/otterize/intents-operator/shared/api/v1alpha1"
	"github.com/otterize/intents-operator/shared/injectablerecorder"
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
	injectablerecorder.InjectableRecorder
}

func getIntentsByServer(defaultNamespace string, intents []otterizev1alpha1.Intent) map[types.NamespacedName][]otterizev1alpha1.Intent {
	intentsByServer := map[types.NamespacedName][]otterizev1alpha1.Intent{}
	for _, intent := range intents {
		serverName := types.NamespacedName{
			Name:      intent.Server,
			Namespace: lo.Ternary(intent.Namespace != "", intent.Namespace, defaultNamespace),
		}

		if intent.Type != otterizev1alpha1.IntentTypeKafka {
			continue
		}

		intentsByServer[serverName] = append(intentsByServer[serverName], intent)
	}

	return intentsByServer
}

func (r *KafkaACLsReconciler) applyACLs(intents *otterizev1alpha1.Intents) (serverCount int, err error) {
	intentsByServer := getIntentsByServer(intents.Namespace, intents.Spec.Service.Calls)

	if err := r.KafkaServersStore.MapErr(func(serverName types.NamespacedName, kafkaIntentsAdmin *kafkaacls.KafkaIntentsAdmin) error {
		intentsForServer := intentsByServer[serverName]
		if err := kafkaIntentsAdmin.ApplyClientIntents(intents.Spec.Service.Name, intents.Namespace, intentsForServer); err != nil {
			return fmt.Errorf("failed applying intents on kafka server %s: %w", serverName, err)
		}
		return nil
	}); err != nil {
		return 0, err
	}

	return len(intentsByServer), nil
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
				r.RecordWarningEvent(intents, "could not remove Kafka ACLs", err.Error())
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
	serverCount, err := r.applyACLs(intents)
	if err != nil {
		r.RecordWarningEvent(intents, "could not apply Kafka ACLs", err.Error())
		return ctrl.Result{}, err
	}

	if serverCount > 0 {
		r.RecordNormalEventf(intents, "Kafka ACL reconcile complete", "Reconciled %d Kafka brokers", serverCount)
	}
	return ctrl.Result{}, nil

}
