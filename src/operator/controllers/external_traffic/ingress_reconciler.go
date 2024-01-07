package external_traffic

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	sets "k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"strings"
)

//+kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=get;list;watch
//+kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;update;patch;list;watch;delete;create

type IngressReconciler struct {
	client.Client
	extNetpolHandler *NetworkPolicyHandler
	injectablerecorder.InjectableRecorder
}

func NewIngressReconciler(client client.Client, extNetpolHandler *NetworkPolicyHandler) *IngressReconciler {
	return &IngressReconciler{Client: client, extNetpolHandler: extNetpolHandler}
}

func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	recorder := mgr.GetEventRecorderFor("intents-operator")
	r.InjectRecorder(recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Ingress{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}

// Reconcile handles ingress creation, update and delete. In all of these cases, it resolves which services
// the ingress refers to, and sends a Reconcile request to the EndpointsReconciler.
// The EndpointsReconciler is responsible for determining which services and ingresses are related to an Endpoints resource
// and managing the network policies accordingly.
func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ingress := &v1.Ingress{}
	err := r.Get(ctx, req.NamespacedName, ingress)
	if k8serrors.IsNotFound(err) {
		// delete is handled here - and we don't have state to retry
		// this is fine, as the EndpointsReconciler will reconcile all endpoints (and as a result, services and ingresses)
		// if the controller restarts
		services, err := r.getServicesReferencedByNetworkPoliciesCreatedForIngress(ctx, req.NamespacedName)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err)
		}

		for service := range services {
			err := r.extNetpolHandler.HandleEndpointsByName(ctx, service, req.Namespace)
			if err != nil {
				return ctrl.Result{}, errors.Wrap(err)
			}
		}
		return ctrl.Result{}, nil
	}

	return r.reconcileIngressCreateOrUpdate(ctx, ingress)
}

func (r *IngressReconciler) getServicesReferencedByNetworkPoliciesCreatedForIngress(ctx context.Context, ingressName types.NamespacedName) (sets.Set[string], error) {
	services := sets.Set[string]{}

	netpolList := &v1.NetworkPolicyList{}
	err := r.List(ctx, netpolList, &client.MatchingFields{otterizev1alpha3.NetworkPoliciesByIngressNameIndexField: ingressName.Name},
		&client.ListOptions{Namespace: ingressName.Namespace})
	if err != nil {
		return nil, errors.Wrap(err)
	}

	for _, netpol := range netpolList.Items {
		serviceName, ok := netpol.Annotations[otterizev1alpha3.OtterizeCreatedForServiceAnnotation]
		if !ok {
			continue
		}
		services.Insert(serviceName)
	}

	return services, nil
}

func (r *IngressReconciler) reconcileIngressCreateOrUpdate(ctx context.Context, ingress *v1.Ingress) (ctrl.Result, error) {
	services := serviceNamesFromIngress(ingress)

	netpolReferencedServices, err := r.getServicesReferencedByNetworkPoliciesCreatedForIngress(ctx, types.NamespacedName{Name: ingress.Name, Namespace: ingress.Namespace})
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	services = services.Union(netpolReferencedServices)

	// Ingress create/update - reconciles each service
	for service := range services {
		err := r.extNetpolHandler.HandleEndpointsByName(ctx, service, ingress.Namespace)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err)
		}
	}
	return ctrl.Result{}, nil
}

func (r *IngressReconciler) InitNetworkPoliciesByIngressNameIndex(mgr ctrl.Manager) error {
	err := mgr.GetCache().IndexField(
		context.Background(),
		&v1.NetworkPolicy{},
		otterizev1alpha3.NetworkPoliciesByIngressNameIndexField,
		func(object client.Object) []string {
			netpol := object.(*v1.NetworkPolicy)
			value, ok := netpol.Annotations[otterizev1alpha3.OtterizeCreatedForIngressAnnotation]
			if !ok {
				return nil
			}

			ingressNames := strings.Split(value, ",")

			return ingressNames
		})

	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}
