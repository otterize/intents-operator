package otterize_cloud

import (
	"context"
	"fmt"
	"github.com/Khan/genqlient/graphql"
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/otterize_cloud/graphql_clients/environments"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strings"
)

const ErrEnvNotFound = "environment not found"

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
		TokenURL:     fmt.Sprintf("%s/auth/tokens/token", cloudAddr),
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

	// Bootstrap Otterize environment
	logrus.Infof("Querying environment ID for Otterize Kubernetes integration")
	envID, err := r.getOrCreateOtterizeEnv(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	clientIntents := otterizev1alpha1.ClientIntents{}
	err = r.Get(ctx, req.NamespacedName, &clientIntents)
	// In case of "Not found" k8s error, we update intents and environment normally
	if err != nil && !k8serrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	err = r.updateOtterizeEnvWithNamespace(ctx, envID, req.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *OtterizeCloudReconciler) getOrCreateOtterizeEnv(ctx context.Context) (string, error) {
	env, err := environments.KubernetesEnvironment(ctx, r.otterizeClient)
	if err != nil {
		if strings.Contains(err.Error(), ErrEnvNotFound) {
			logrus.Infof("No environment found associated with token. Creating new environment")
			env, err := environments.CreateKubernetesEnvironment(ctx, r.otterizeClient)
			if err != nil {
				return "", err
			}
			return env.Me.CreateKubernetesEnvironment.GetId(), nil
		} else {
			return "", err
		}
	}
	return env.Me.KubernetesEnvironment.GetId(), nil
}

func (r *OtterizeCloudReconciler) updateOtterizeEnvWithNamespace(ctx context.Context, envID, namespace string) error {
	// The field "Cluster" in type NamespaceInput is set in the cloud, and the integration name that sent the req is used
	env, err := environments.AddNamespacesToEnv(ctx, r.otterizeClient, envID, []string{namespace})
	if err != nil {
		return err
	}

	logrus.Infof("Added namespace %s to Otterize env %s", namespace, env.AddEnvironmentNamespaces.GetName())
	return nil
}
