package database

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	ReasonApplyingDatabaseIntentsFailed = "ApplyingDatabaseIntentsFailed"
	ReasonAppliedDatabaseIntents        = "AppliedDatabaseIntents"
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
	intents := &otterizev1alpha3.ClientIntents{}
	logger := logrus.WithField("namespacedName", req.String())
	err := r.client.Get(ctx, req.NamespacedName, intents)
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

	action := graphqlclient.DBPermissionChangeApply
	if !intents.ObjectMeta.DeletionTimestamp.IsZero() {
		action = graphqlclient.DBPermissionChangeDelete
	}

	var intentInputList []graphqlclient.IntentInput
	for _, intent := range intents.GetCallsList() {
		if intent.Type != otterizev1alpha3.IntentTypeDatabase {
			continue
		}

		intentInput := intent.ConvertToCloudFormat(intents.Namespace, intents.GetServiceName())
		intentInputList = append(intentInputList, intentInput)
	}

	if len(intentInputList) == 0 {
		return ctrl.Result{}, nil
	}

	if err := r.otterizeClient.ApplyDatabaseIntent(ctx, intentInputList, action); err != nil {
		errType, errMsg, ok := graphqlclient.GetGraphQLUserError(err)
		if !ok || errType != graphqlclient.UserErrorTypeAppliedIntentsError {
			r.RecordWarningEventf(intents, ReasonApplyingDatabaseIntentsFailed, "Failed applying database intents: %s", err.Error())
			return ctrl.Result{}, errors.Wrap(err)
		}
		r.RecordWarningEventf(intents, ReasonApplyingDatabaseIntentsFailed, "Failed applying database intents: %s", errMsg)
		return ctrl.Result{}, errors.Wrap(err)
	}

	r.RecordNormalEventf(intents, ReasonAppliedDatabaseIntents, "Database intents reconcile complete, reconciled %d intent calls", len(intentInputList))

	return ctrl.Result{}, nil
}

// PeriodicReconcileNewDBTables compensates for tables created in a database after the intents were applied and
// permissions were configured. Runs periodically for all existing database intents without a specified table.
func (r *DatabaseReconciler) PeriodicReconcileNewDBTables(ctx context.Context) {
	newTablesTicker := time.NewTicker(time.Second * 30) // TODO: Change after DB enforcement refactor
	logrus.Info("Database enforcement is on. Starting periodic database tables reconciliation")
	for {
		select {
		case <-newTablesTicker.C:
			if err := r.reconcileNewTables(ctx); err != nil {
				logrus.WithError(err).Error("Failed reconciling new tables for intents")
			}
		case <-ctx.Done():
			logrus.Info("Periodic database tables reconcile finished")
			return
		}
	}
}

func (r *DatabaseReconciler) reconcileNewTables(ctx context.Context) error {
	intentsList := otterizev1alpha3.ClientIntentsList{}
	if err := r.client.List(ctx, &intentsList); err != nil {
		return errors.Wrap(err)
	}

	var intentInputList []graphqlclient.IntentInput
	for _, intents := range intentsList.Items {
		if !intents.ObjectMeta.DeletionTimestamp.IsZero() {
			// No need to do anything, will be handled by the operator
			continue
		}

		for _, intent := range intents.GetCallsList() {
			if intent.Type != otterizev1alpha3.IntentTypeDatabase {
				continue
			}
			hasWildcardTable := false
			for _, dbResource := range intent.DatabaseResources {
				if dbResource.Table == "" || dbResource.Table == "*" {
					hasWildcardTable = true
				}
			}
			// We only add to the list in case of wildcard table permissions
			if hasWildcardTable {
				intentInput := intent.ConvertToCloudFormat(intents.Namespace, intents.GetServiceName())
				intentInputList = append(intentInputList, intentInput)
			}
		}
	}

	if len(intentInputList) == 0 {
		return nil
	}

	err := r.otterizeClient.ApplyDatabaseIntent(ctx, intentInputList, graphqlclient.DBPermissionChangeApply)
	if err != nil {
		return errors.Wrap(err)
	}
	return nil
}
