package otterizecloud

import (
	"context"
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
	clientIntents := otterizev1alpha1.ClientIntents{}
	err := r.Get(ctx, req.NamespacedName, &clientIntents)
	// In case of "Not found" k8s error, we report intents and namespace normally
	if err != nil && !k8serrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if err = r.otterizeClient.ReportKubernetesNamespace(ctx, req.Namespace); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
