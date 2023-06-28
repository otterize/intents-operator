package external_traffic

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/otterizecloud"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	v12 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type NetworkPolicyReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	serviceIdResolver *serviceidresolver.Resolver
	otterizeClient    otterizecloud.CloudClient
	injectablerecorder.InjectableRecorder
}

func NewNetworkPolicyReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	otterizeClient otterizecloud.CloudClient,
) *NetworkPolicyReconciler {
	return &NetworkPolicyReconciler{
		Client:            client,
		Scheme:            scheme,
		serviceIdResolver: serviceidresolver.NewResolver(client),
		otterizeClient:    otterizeClient,
	}
}

func (r *NetworkPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	recorder := mgr.GetEventRecorderFor("intents-operator")
	r.InjectRecorder(recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.NetworkPolicy{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		WithEventFilter(filterOtterizeNetworkPolicy()).
		Complete(r)
}

func (r *NetworkPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logrus.WithField("policy", req.NamespacedName.String()).Infof("Reconcile NetworkPolicy")

	netpol := &v1.NetworkPolicy{}
	err := r.Get(ctx, req.NamespacedName, netpol)

	if k8serrors.IsNotFound(err) {
		logrus.WithField("policy", req.NamespacedName.String()).Info("NetPol was deleted")
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	selector, err := metav1.LabelSelectorAsSelector(&netpol.Spec.PodSelector)

	if err != nil {
		return ctrl.Result{}, err
	}

	var podList v12.PodList
	err = r.List(
		ctx, &podList,
		&client.MatchingLabelsSelector{Selector: selector},
		&client.ListOptions{Namespace: netpol.Namespace})

	if err != nil {
		logrus.WithError(err).Errorf("error when reading podlist")
		return ctrl.Result{}, nil
	}

	var inputs []graphqlclient.NetworkPolicyInput

	for _, pod := range podList.Items {
		serviceId, err := r.serviceIdResolver.ResolvePodToServiceIdentity(ctx, &pod)

		logrus.
			WithField("pod", fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)).
			WithField("service", serviceId.Name).
			Debug("matching pod to otterize service")

		if err != nil {
			logrus.WithField("policy", req.NamespacedName.String()).WithError(err).WithField("pod", pod.Name).Errorf("Error resolving pod")
			continue
		}

		inputs = append(inputs, graphqlclient.NetworkPolicyInput{
			Namespace:             req.Namespace,
			Name:                  req.Name,
			ServerName:            serviceId.Name,
			ExternalTrafficPolicy: true,
		})
	}

	err = r.otterizeClient.ReportNetworkPolicies(ctx, req.Namespace, inputs)

	if err != nil {
		logrus.WithError(err).
			WithField("namespace", req.Namespace).
			Error("failed reporting network policies")

		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func filterOtterizeNetworkPolicy() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			labels := e.Object.GetLabels()
			_, isExternalTrafficPolicy := labels[v1alpha2.OtterizeNetworkPolicyExternalTraffic]

			return isExternalTrafficPolicy
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			labels := e.ObjectNew.GetLabels()
			_, isExternalTrafficPolicy := labels[v1alpha2.OtterizeNetworkPolicyExternalTraffic]

			return isExternalTrafficPolicy
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.DeleteStateUnknown {
				return false
			}

			labels := e.Object.GetLabels()
			_, isExternalTrafficPolicy := labels[v1alpha2.OtterizeNetworkPolicyExternalTraffic]

			return isExternalTrafficPolicy
		},
	}
}
