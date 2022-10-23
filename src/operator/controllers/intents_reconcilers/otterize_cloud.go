package intents_reconcilers

import (
	"context"
	"fmt"
	"github.com/Khan/genqlient/graphql"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/intents_client"
	"github.com/samber/lo"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const OtterizeApiAddressKey = "http://host.docker.internal:8080/api"

type OtterizeCloudReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	tokenSrc oauth2.TokenSource
	injectablerecorder.InjectableRecorder
}

func NewOtterizeCloudReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	otterizeClientID string,
	otterizeClientSecret string) *OtterizeCloudReconciler {

	cfg := clientcredentials.Config{
		ClientID:     otterizeClientID,
		ClientSecret: otterizeClientSecret,
		TokenURL:     fmt.Sprintf("%s/auth/tokens/token", OtterizeApiAddressKey),
		AuthStyle:    oauth2.AuthStyleInParams,
	}

	return &OtterizeCloudReconciler{
		Client:   client,
		Scheme:   scheme,
		tokenSrc: cfg.TokenSource(context.Background()),
	}
}

func (r *OtterizeCloudReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	fmt.Println("YALLA INTENTS !!!")
	_, err := r.ApplyIntentsToCloud(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *OtterizeCloudReconciler) newClientForURI(uri string, ctx context.Context) graphql.Client {
	return graphql.NewClient(
		fmt.Sprintf("%s/%s", OtterizeApiAddressKey, uri),
		oauth2.NewClient(ctx, r.tokenSrc))
}

func (r *OtterizeCloudReconciler) ApplyIntentsToCloud(ctx context.Context) (ctrl.Result, error) {
	gqlClient := r.newClientForURI("/intents/query", ctx)
	res, err := intents_client.ApplyIntents(
		ctx, gqlClient, &intents_client.EnvSelector{Name: lo.ToPtr("abcd")}, lo.ToPtr("intents-operator"), []*intents_client.IntentInput{
			{
				Client: lo.ToPtr("asdasd"),
				Server: lo.ToPtr("ASJKDJASKDKASD"),
				Body: &intents_client.IntentBody{
					Type: lo.ToPtr(intents_client.IntentTypeHttp),
				},
			},
		})

	fmt.Println(res)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
