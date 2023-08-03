package exp

import (
	"context"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	DatabaseFinalizerName = "intents.otterize.com/database-finalizer"
)

type DatabaseReconciler struct {
	client         client.Client
	scheme         *runtime.Scheme
	otterizeClient operator_cloud_client.CloudClient
	injectablerecorder.InjectableRecorder
}

func NewDatabaseReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	otterizeClient operator_cloud_client.CloudClient,
) *DatabaseReconciler {
	return &DatabaseReconciler{
		client:         client,
		scheme:         scheme,
		otterizeClient: otterizeClient,
	}
}

func (r *DatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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

	if r.isMissingDatabaseFinalizer(intents) {
		logger.Infof("Adding finalizer %s", DatabaseFinalizerName)
		controllerutil.AddFinalizer(intents, DatabaseFinalizerName)
		if err := r.client.Update(ctx, intents); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	action := graphqlclient.DBPermissionChangeApply
	if !intents.ObjectMeta.DeletionTimestamp.IsZero() {
		action = graphqlclient.DBPermissionChangeDelete
	}

	var intentInputList []graphqlclient.IntentInput
	for _, intent := range intents.GetCallsList() {
		if intent.Type != otterizev1alpha2.IntentTypeDatabase {
			continue
		}

		intentInput := intent.ConvertToCloudFormat(intents.Namespace, intents.GetServiceName())
		intentInputList = append(intentInputList, intentInput)
	}

	if err := r.otterizeClient.ApplyDatabaseIntent(ctx, intentInputList, action); err != nil {
		return ctrl.Result{}, err
	}

	if action == graphqlclient.DBPermissionChangeDelete {
		intents_reconcilers.RemoveIntentFinalizers(intents, DatabaseFinalizerName)
		if err := r.client.Update(ctx, intents); err != nil {
			if k8serrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *DatabaseReconciler) isMissingDatabaseFinalizer(intents *otterizev1alpha2.ClientIntents) bool {
	return !controllerutil.ContainsFinalizer(intents, DatabaseFinalizerName) && intents.HasDatabaseTypeInCallList()
}
