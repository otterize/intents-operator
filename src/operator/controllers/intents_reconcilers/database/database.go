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
