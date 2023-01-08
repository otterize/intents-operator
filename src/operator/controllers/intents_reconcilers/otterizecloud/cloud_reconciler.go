package otterizecloud

import (
	"context"
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
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
	clientIntentsList := otterizev1alpha1.ClientIntentsList{}
	if err := r.List(ctx, &clientIntentsList, &client.ListOptions{Namespace: req.Namespace}); err != nil {
		return ctrl.Result{}, nil
	}

	intentsInput, err := clientIntentsList.FormatAsOtterizeIntents(req.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err = r.otterizeClient.ReportAppliedIntents(ctx, lo.ToPtr(req.Namespace), intentsInput); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
