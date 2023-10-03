package intents_reconcilers

import (
	"context"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type OtterizeCloudReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	otterizeClient    operator_cloud_client.CloudClient
	serviceIdResolver *serviceidresolver.Resolver
	injectablerecorder.InjectableRecorder
}

func NewOtterizeCloudReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	cloudClient operator_cloud_client.CloudClient,
	serviceIdResolver *serviceidresolver.Resolver) *OtterizeCloudReconciler {

	return &OtterizeCloudReconciler{
		Client:            client,
		Scheme:            scheme,
		otterizeClient:    cloudClient,
		serviceIdResolver: serviceIdResolver,
	}
}

func (r *OtterizeCloudReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	// Report Applied intents from namespace
	clientIntentsList := &otterizev1alpha2.ClientIntentsList{}
	if err := r.List(ctx, clientIntentsList, &client.ListOptions{Namespace: req.Namespace}); err != nil {
		return ctrl.Result{}, nil
	}

	clientIntentsList, err := r.convertK8sServicesToOtterizeIdentities(ctx, clientIntentsList)
	if err != nil {
		return ctrl.Result{}, err
	}

	intentsInput, err := clientIntentsList.FormatAsOtterizeIntents()
	if err != nil {
		return ctrl.Result{}, err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, viper.GetDuration(otterizecloudclient.CloudClientTimeoutKey))
	defer cancel()

	if err = r.otterizeClient.ReportAppliedIntents(timeoutCtx, lo.ToPtr(req.Namespace), intentsInput); err != nil {
		logrus.WithError(err).Error("failed to report applied intents")
		return ctrl.Result{}, err
	}

	logrus.Infof("successfully reported %d applied intents", len(clientIntentsList.Items))

	return ctrl.Result{}, nil
}

func (r *OtterizeCloudReconciler) convertK8sServicesToOtterizeIdentities(
	ctx context.Context,
	clientIntentsList *otterizev1alpha2.ClientIntentsList) (*otterizev1alpha2.ClientIntentsList, error) {

	// TODO: Remove when access graph supports Kubernetes services
	for _, clientIntent := range clientIntentsList.Items {
		callList := make([]otterizev1alpha2.Intent, 0)
		for _, intent := range clientIntent.GetCallsList() {
			if intent.IsTargetServerKubernetesService() {
				svc := corev1.Service{}
				kubernetesSvcName := intent.GetTargetServerName()
				kubernetesSvcNamespace := intent.GetTargetServerNamespace(clientIntent.Namespace)
				err := r.Get(ctx, types.NamespacedName{
					Namespace: kubernetesSvcNamespace,
					Name:      kubernetesSvcName,
				}, &svc)
				if err != nil {
					return nil, err
				}
				podList := corev1.PodList{}
				err = r.List(ctx, &podList, &client.ListOptions{LabelSelector: labels.SelectorFromSet(svc.Spec.Selector)})
				if err != nil {
					return nil, err
				}
				if len(podList.Items) != 0 {
					otterizeIdentity, err := r.serviceIdResolver.ResolvePodToServiceIdentity(ctx, &podList.Items[0])
					if err != nil {
						return nil, err
					}
					intent.Name = otterizeIdentity.Name
					callList = append(callList, intent)
				}
			}
			clientIntent.Spec.Calls = callList
		}
	}

	return clientIntentsList, nil
}
