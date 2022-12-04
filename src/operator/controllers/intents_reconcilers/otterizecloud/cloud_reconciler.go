package otterizecloud

import (
	"context"
	"fmt"
	"github.com/Khan/genqlient/graphql"
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/otterizecloud/graphql_clients/kubernetes"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type OtterizeCloudReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	tokenSrc       oauth2.TokenSource
	otterizeClient graphql.Client
	injectablerecorder.InjectableRecorder
}

func NewOtterizeCloudReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	otterizeClientID,
	otterizeClientSecret,
	cloudAddr string) *OtterizeCloudReconciler {

	cfg := clientcredentials.Config{
		ClientID:     otterizeClientID,
		ClientSecret: otterizeClientSecret,
		TokenURL:     fmt.Sprintf("%s/api/auth/tokens/token", cloudAddr),
		AuthStyle:    oauth2.AuthStyleInParams,
	}
	ctx := context.Background()
	otterizeClient := graphql.NewClient(
		fmt.Sprintf("%s/api/graphql/v1", cloudAddr),
		oauth2.NewClient(ctx, cfg.TokenSource(ctx)))

	return &OtterizeCloudReconciler{
		Client:         client,
		Scheme:         scheme,
		tokenSrc:       cfg.TokenSource(context.Background()),
		otterizeClient: otterizeClient,
	}
}

func (r *OtterizeCloudReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	// Skip this reconciliation, no client id/secret provided
	if r.tokenSrc == nil {
		return ctrl.Result{}, nil
	}

	clientIntents := otterizev1alpha1.ClientIntents{}
	err := r.Get(ctx, req.NamespacedName, &clientIntents)
	// In case of "Not found" k8s error, we report intents and namespace normally
	if err != nil && !k8serrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if err = r.reportNamespace(ctx, req.Namespace); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *OtterizeCloudReconciler) reportNamespace(ctx context.Context, namespace string) error {
	res, err := kubernetes.ReportKubernetesNamespace(ctx, r.otterizeClient, namespace)
	if err != nil {
		return err
	}
	// Return value is bool - returns true if anything was updated in the DB
	if res.ReportKubernetesNamespace {
		logrus.Infof("Successfully reported namespace # %s # to Otterize cloud", namespace)
	}
	return nil
}
