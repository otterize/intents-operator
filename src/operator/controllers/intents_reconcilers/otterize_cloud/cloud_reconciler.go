package otterize_cloud

import (
	"context"
	"fmt"
	"github.com/Khan/genqlient/graphql"
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type OtterizeCloudReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	tokenSrc  oauth2.TokenSource
	cloudAddr string
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
		TokenURL:     fmt.Sprintf("%s/auth/tokens/token", cloudAddr),
		AuthStyle:    oauth2.AuthStyleInParams,
	}

	return &OtterizeCloudReconciler{
		Client:    client,
		Scheme:    scheme,
		tokenSrc:  cfg.TokenSource(context.Background()),
		cloudAddr: cloudAddr,
	}
}

func (r *OtterizeCloudReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	intentsList := otterizev1alpha1.ClientIntentsList{}
	err := r.List(ctx, &intentsList, &client.ListOptions{Namespace: req.Namespace})
	if err != nil {
		return ctrl.Result{}, err
	}

	_, err = r.ApplyIntentsToCloud(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *OtterizeCloudReconciler) newClientForURI(uri string, ctx context.Context) graphql.Client {
	return graphql.NewClient(
		fmt.Sprintf("%s/%s", r.cloudAddr, uri),
		oauth2.NewClient(ctx, r.tokenSrc))
}

func (r *OtterizeCloudReconciler) ApplyIntentsToCloud(ctx context.Context) (ctrl.Result, error) {
	fmt.Println("yalla intents")
	return ctrl.Result{}, nil
}
