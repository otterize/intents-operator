package otterize_cloud

import (
	"context"
	"fmt"
	"github.com/Khan/genqlient/graphql"
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/otterize_cloud/graphql_clients/intents"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
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

func (r *OtterizeCloudReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	// Skip this reconciliation, no client id/secret provided
	if r.tokenSrc == nil {
		return ctrl.Result{}, nil
	}

	// Bootstrap env

	// Send intents to the cloud
	intentsList := otterizev1alpha1.ClientIntentsList{}
	err := r.List(ctx, &intentsList, &client.ListOptions{Namespace: req.Namespace})
	if err != nil {
		return ctrl.Result{}, err
	}

	otterizeIntents, err := intentsList.FormatAsOtterizeIntents()
	if err != nil {
		return ctrl.Result{}, err
	}

	_, err = r.ApplyIntentsToCloud(ctx, otterizeIntents, req.Namespace)
	if err != nil {
		// TODO: Record event on err. Consider requeueing
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *OtterizeCloudReconciler) newClientForURI(ctx context.Context, uri string) graphql.Client {
	return graphql.NewClient(
		fmt.Sprintf("%s/%s", r.cloudAddr, uri),
		oauth2.NewClient(ctx, r.tokenSrc))
}

func (r *OtterizeCloudReconciler) ApplyIntentsToCloud(
	ctx context.Context,
	intentsInput []*intents.IntentInput,
	ns string) (ctrl.Result, error) {

	fmt.Println("yalla intents")
	c := r.newClientForURI(ctx, "intents/query")

	// TODO: Change this intents source to be meaningful. Consider using integration name
	_, err := intents.ApplyIntents(ctx, c, &intents.EnvSelector{Name: lo.ToPtr(ns)}, lo.ToPtr("intents-operator"), intentsInput)
	if err != nil {
		return ctrl.Result{}, err
	}
	logrus.Infof("Successfuly reported intents to Otterize cloud. count for namespace %s: %d", ns, len(intentsInput))

	return ctrl.Result{}, nil
}
