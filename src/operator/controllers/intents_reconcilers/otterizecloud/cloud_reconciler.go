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
	"sync"
)

type OtterizeCloudReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	otterizeClient     CloudClient
	reportedNamespaces sync.Map
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

	// Report namespace if not in cache
	if _, ok := r.reportedNamespaces.Load(req.Namespace); !ok {
		// Namespace is not in cache, report it to Otterize cloud
		if err = r.otterizeClient.ReportKubernetesNamespace(ctx, req.Namespace); err != nil {
			return ctrl.Result{}, err
		}
		r.reportedNamespaces.Store(req.Namespace, true)
	}

	// Report Applied intents from namespace
	clientIntentsList := otterizev1alpha1.ClientIntentsList{}
	if err = r.List(ctx, &clientIntentsList); err != nil {
		return ctrl.Result{}, nil
	}

	if err = r.otterizeClient.ReportAppliedIntents(ctx, req.Namespace, clientIntentsList); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
