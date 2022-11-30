package otterize_cloud

import (
	"context"
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/sirupsen/logrus"
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
	// Bootstrap Otterize environment
	logrus.Infof("Querying environment ID for Otterize Kubernetes integration")
	envID, err := r.otterizeClient.GetOrCreateOtterizeEnv(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	clientIntents := otterizev1alpha1.ClientIntents{}
	err = r.Get(ctx, req.NamespacedName, &clientIntents)
	// In case of "Not found" k8s error, we update intents and environment normally
	if err != nil && !k8serrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	// The field "Cluster" in type NamespaceInput is set in the cloud, and the integration name that sent the req is used
	err = r.otterizeClient.UpdateOtterizeEnvWithNamespace(ctx, envID, req.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
