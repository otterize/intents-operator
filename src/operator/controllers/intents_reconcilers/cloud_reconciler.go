package intents_reconcilers

import (
	"context"
	otterizev2 "github.com/otterize/intents-operator/src/operator/api/v2"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type OtterizeCloudReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	otterizeClient    operator_cloud_client.CloudClient
	serviceIdResolver serviceidresolver.ServiceResolver
	injectablerecorder.InjectableRecorder
}

func NewOtterizeCloudReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	cloudClient operator_cloud_client.CloudClient) *OtterizeCloudReconciler {

	return &OtterizeCloudReconciler{
		Client:            client,
		Scheme:            scheme,
		otterizeClient:    cloudClient,
		serviceIdResolver: serviceidresolver.NewResolver(client),
	}
}

func (r *OtterizeCloudReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	// Report Applied intents from namespace
	clientIntentsList := &otterizev2.ClientIntentsList{}
	if err := r.List(ctx, clientIntentsList, &client.ListOptions{Namespace: req.Namespace}); err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	clientIntentsList.Items = lo.Filter(clientIntentsList.Items, func(intents otterizev2.ClientIntents, _ int) bool {
		return intents.DeletionTimestamp == nil
	})

	clientIntentsListConverted, err := r.convertK8sServicesToOtterizeIdentities(ctx, clientIntentsList)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	intentsInput, err := clientIntentsListConverted.FormatAsOtterizeIntents(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, viper.GetDuration(otterizecloudclient.CloudClientTimeoutKey))
	defer cancel()

	if err = r.otterizeClient.ReportAppliedIntents(timeoutCtx, lo.ToPtr(req.Namespace), intentsInput); err != nil {
		logrus.WithError(err).Error("failed to report applied intents")
		return ctrl.Result{}, errors.Wrap(err)
	}

	logrus.Debugf("successfully reported %d applied intents", len(clientIntentsList.Items))

	return ctrl.Result{}, nil
}

func (r *OtterizeCloudReconciler) convertK8sServicesToOtterizeIdentities(
	ctx context.Context,
	clientIntentsList *otterizev2.ClientIntentsList) (*otterizev2.ClientIntentsList, error) {

	// TODO: Remove when access graph supports Kubernetes services
	for _, clientIntent := range clientIntentsList.Items {
		callList := make([]otterizev2.Target, 0)
		for _, intent := range clientIntent.GetTargetList() {
			if !intent.IsTargetServerKubernetesService() {
				callList = append(callList, intent)
				continue
			}
			if intent.IsTargetTheKubernetesAPIServer(clientIntent.Namespace) {
				intentCopy := intent.DeepCopy()
				if intent.Kubernetes != nil {
					intentCopy.Kubernetes.Name = intent.GetServerFullyQualifiedName(clientIntent.Namespace)
				}
				if intent.Service != nil {
					intentCopy.Service.Name = intent.GetServerFullyQualifiedName(clientIntent.Namespace)
				}
				callList = append(callList, intent)
				continue
			}

			svc := corev1.Service{}
			kubernetesSvcName := intent.GetTargetServerName()
			kubernetesSvcNamespace := intent.GetTargetServerNamespace(clientIntent.Namespace)
			err := r.Get(ctx, types.NamespacedName{
				Namespace: kubernetesSvcNamespace,
				Name:      kubernetesSvcName,
			}, &svc)
			if err != nil {
				if k8serrors.IsNotFound(err) {
					continue
				}
				return nil, errors.Wrap(err)
			}
			podList := corev1.PodList{}
			err = r.List(ctx, &podList, &client.ListOptions{LabelSelector: labels.SelectorFromSet(svc.Spec.Selector)})
			if err != nil {
				return nil, errors.Wrap(err)
			}
			if len(podList.Items) != 0 {
				otterizeIdentity, err := r.serviceIdResolver.ResolvePodToServiceIdentity(ctx, &podList.Items[0])
				if err != nil {
					return nil, errors.Wrap(err)
				}
				if intent.Kubernetes != nil {
					intent.Kubernetes.Name = otterizeIdentity.Name
				}
				if intent.Service != nil {
					intent.Service.Name = otterizeIdentity.Name
				}
				callList = append(callList, intent)
			}
			clientIntent.Spec.Targets = callList
		}
	}

	return clientIntentsList, nil
}

func (r *OtterizeCloudReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&otterizev2.ClientIntents{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}
