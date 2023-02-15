package otterizecloud

import (
	"context"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type OtterizeCloudReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	otterizeClient CloudClient
	injectablerecorder.InjectableRecorder
}

func NewOtterizeCloudReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	cloudClient CloudClient) *OtterizeCloudReconciler {

	return &OtterizeCloudReconciler{
		Client:         client,
		Scheme:         scheme,
		otterizeClient: cloudClient,
	}
}

func (r *OtterizeCloudReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	// Report Applied intents from namespace
	clientIntentsList := otterizev1alpha2.ClientIntentsList{}
	if err := r.List(ctx, &clientIntentsList, &client.ListOptions{Namespace: req.Namespace}); err != nil {
		return ctrl.Result{}, nil
	}

	intentsInput, err := clientIntentsList.FormatAsOtterizeIntents()
	if err != nil {
		return ctrl.Result{}, err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, viper.GetDuration(otterizecloudclient.CloudClientTimeoutKey))
	defer cancel()

	if err = r.otterizeClient.ReportAppliedIntents(timeoutCtx, lo.ToPtr(req.Namespace), intentsInput); err != nil {
		logrus.WithError(err).Error("failed to report applied intents")
		return ctrl.Result{}, err
	}

	logrus.Infof("successfully reported %d applied intents", len(clientIntentsList.Items))

	return ctrl.Result{}, nil
}
