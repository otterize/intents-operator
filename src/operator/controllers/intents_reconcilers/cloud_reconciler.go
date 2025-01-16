package intents_reconcilers

import (
	"context"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type OtterizeCloudReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	otterizeClient    operator_cloud_client.CloudClient
	serviceIdResolver serviceidresolver.ServiceResolver
	injectablerecorder.InjectableRecorder
}

func NewOtterizeCloudReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	cloudClient operator_cloud_client.CloudClient) *OtterizeCloudReconciler {

	return &OtterizeCloudReconciler{
		Client:            client,
		Scheme:            scheme,
		otterizeClient:    cloudClient,
		serviceIdResolver: serviceidresolver.NewResolver(client),
	}
}

func (r *OtterizeCloudReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	// Report Applied intents from namespace
	clientIntentsList := &otterizev2alpha1.ClientIntentsList{}
	if err := r.List(ctx, clientIntentsList, &client.ListOptions{Namespace: req.Namespace}); err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	clientIntentsList.Items = lo.Filter(clientIntentsList.Items, func(intents otterizev2alpha1.ClientIntents, _ int) bool {
		return intents.DeletionTimestamp == nil
	})

	intentsInput, err := clientIntentsList.FormatAsOtterizeIntents(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, viper.GetDuration(otterizecloudclient.CloudClientTimeoutKey))
	defer cancel()

	if err = r.otterizeClient.ReportAppliedIntents(timeoutCtx, lo.ToPtr(req.Namespace), intentsInput); err != nil {
		logrus.WithError(err).Error("failed to report applied intents")
		return ctrl.Result{}, errors.Wrap(err)
	}

	logrus.Debugf("successfully reported %d applied intents", len(clientIntentsList.Items))

	return ctrl.Result{}, nil
}

func (r *OtterizeCloudReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&otterizev2alpha1.ClientIntents{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}
