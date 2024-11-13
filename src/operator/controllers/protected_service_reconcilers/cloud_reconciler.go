package protected_service_reconcilers

import (
	"context"
	otterizev2 "github.com/otterize/intents-operator/src/operator/api/v2"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CloudReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	otterizeClient operator_cloud_client.CloudClient
	injectablerecorder.InjectableRecorder
}

func NewCloudReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	cloudClient operator_cloud_client.CloudClient) *CloudReconciler {

	return &CloudReconciler{
		Client:         client,
		Scheme:         scheme,
		otterizeClient: cloudClient,
	}
}

func (r *CloudReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	err := r.reportAllProtectedServicesInNamespace(ctx, req.Namespace)
	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	return ctrl.Result{}, nil
}

func (r *CloudReconciler) reportAllProtectedServicesInNamespace(ctx context.Context, namespace string) error {
	var protectedServices otterizev2.ProtectedServiceList
	err := r.List(ctx, &protectedServices, client.InNamespace(namespace))
	if err != nil {
		return errors.Wrap(err)
	}

	services := sets.Set[string]{}
	for _, protectedService := range protectedServices.Items {
		if protectedService.DeletionTimestamp != nil {
			continue
		}

		services.Insert(protectedService.Spec.Name)
	}

	protectedServicesInput := r.formatAsCloudProtectedService(sets.List(services))
	return r.otterizeClient.ReportProtectedServices(ctx, namespace, protectedServicesInput)
}

func (r *CloudReconciler) formatAsCloudProtectedService(services []string) []graphqlclient.ProtectedServiceInput {
	protectedServicesInput := make([]graphqlclient.ProtectedServiceInput, 0)
	for _, service := range services {
		input := graphqlclient.ProtectedServiceInput{
			Name: service,
		}
		protectedServicesInput = append(protectedServicesInput, input)
	}
	return protectedServicesInput
}
