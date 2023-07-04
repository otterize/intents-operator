package external_traffic

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

const OtterizeExternalNetworkPolicyNameTemplate = "external-access-to-%s"

//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch
//+kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;update;patch;list;watch;delete;create

type EndpointsReconciler interface {
	Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
	InitIngressReferencedServicesIndex(mgr ctrl.Manager) error
	SetupWithManager(mgr ctrl.Manager) error
	InjectRecorder(recorder record.EventRecorder)
}

type EndpointsReconcilerImpl struct {
	client.Client
	Scheme                     *runtime.Scheme
	netpolCreator              *NetworkPolicyCreator
	createEvenIfNoIntentsFound bool
	injectablerecorder.InjectableRecorder
}

func (r *EndpointsReconcilerImpl) formatPolicyName(serviceName string) string {
	return fmt.Sprintf(OtterizeExternalNetworkPolicyNameTemplate, serviceName)
}

func NewEndpointsReconciler(client client.Client, scheme *runtime.Scheme, enabled bool, createEvenIfNoIntentsFound bool, enforcementEnabledGlobally bool) EndpointsReconciler {
	return &EndpointsReconcilerImpl{
		Client:                     client,
		Scheme:                     scheme,
		createEvenIfNoIntentsFound: createEvenIfNoIntentsFound,
		netpolCreator:              NewNetworkPolicyCreator(client, scheme, enabled, createEvenIfNoIntentsFound, enforcementEnabledGlobally),
	}
}

func (r *EndpointsReconcilerImpl) SetupWithManager(mgr ctrl.Manager) error {
	recorder := mgr.GetEventRecorderFor("intents-operator")
	r.InjectRecorder(recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Endpoints{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}

func (r *EndpointsReconcilerImpl) InjectRecorder(recorder record.EventRecorder) {
	r.Recorder = recorder
	r.netpolCreator.InjectRecorder(recorder)
}

// Reconcile handles three cases:
// (1) It watches Endpoints, which means it gets updates when Services are updated, or the pods backing them are updated.
//
//	    When that happens, and the Service is of type LoadBalancer, NodePort, or is referenced by an Ingress,
//		   it checks if the backing pods are affected by Otterize Intents Network Policies.
//		   If so, and the reconciler is enabled, it will create network policies to allow external traffic to those pods.
//		   If the Endpoints (= Services) update port, it will update the port specified in the corresponding network policy.
//		   If the Endpoints no longer refer to pods affected by Intents, then the network policy will be deleted.
//		   If the Service is deleted completely, then the corresponding network policy will be deleted, since it is owned
//		   by the service.
//
// (2) It receives reconcile requests from the IngressReconciler, when Ingresses are created, updated or deleted.
// (3) It receives reconcile requests from the Intents NetworkPolicyReconciler, when Network Policies that apply intents
//
//	are created, updated or deleted. This means that if you create, update or delete intents, the corresponding
//	external traffic policy will be created (if there were no other intents affecting the service before then) or
//	deleted (if no intents network policies refer to the pods backing the service any longer).
func (r *EndpointsReconcilerImpl) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	endpoints := &corev1.Endpoints{}
	err := r.Get(ctx, req.NamespacedName, endpoints)
	if k8serrors.IsNotFound(err) {
		// delete is handled by garbage collection - the service owns the network policy
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	svc := &corev1.Service{}
	err = r.Get(ctx, req.NamespacedName, svc)
	if k8serrors.IsNotFound(err) {
		// delete is handled by garbage collection - the service owns the network policy
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	ingressList, err := r.getIngressRefersToService(ctx, svc)
	if err != nil {
		return ctrl.Result{}, err
	}
	// If it's not a load balancer or a node port service, and the service is not referenced by any Ingress,
	// then there's nothing we need to do.
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer && svc.Spec.Type != corev1.ServiceTypeNodePort && len(ingressList.Items) == 0 {
		return r.handlePolicyDelete(ctx, r.formatPolicyName(svc.Name), svc.Namespace)
	}

	return r.reconcileEndpoints(ctx, endpoints, ingressList)
}

func (r *EndpointsReconcilerImpl) getIngressRefersToService(ctx context.Context, svc *corev1.Service) (*v1.IngressList, error) {
	var endpointsList v1.IngressList
	err := r.List(
		ctx, &endpointsList,
		&client.MatchingFields{v1alpha2.IngressServiceNamesIndexField: svc.Name},
		&client.ListOptions{Namespace: svc.Namespace})

	if err != nil {
		return nil, err
	}

	return &endpointsList, nil
}

func (r *EndpointsReconcilerImpl) InitIngressReferencedServicesIndex(mgr ctrl.Manager) error {
	err := mgr.GetCache().IndexField(
		context.Background(),
		&v1.Ingress{},
		v1alpha2.IngressServiceNamesIndexField,
		func(object client.Object) []string {
			ingress := object.(*v1.Ingress)
			services := serviceNamesFromIngress(ingress)
			return sets.List(services)
		})

	if err != nil {
		return err
	}

	return nil
}

func (r *EndpointsReconcilerImpl) reconcileEndpoints(ctx context.Context, endpoints *corev1.Endpoints, ingressList *v1.IngressList) (ctrl.Result, error) {
	foundOtterizeNetpolsAffectingPods := false

	addresses := make([]corev1.EndpointAddress, 0)
	for _, subset := range endpoints.Subsets {
		addresses = append(addresses, subset.Addresses...)
		addresses = append(addresses, subset.NotReadyAddresses...)

	}

	for _, address := range addresses {
		if address.TargetRef == nil || address.TargetRef.Kind != "Pod" {
			continue
		}

		pod := &corev1.Pod{}
		err := r.Get(ctx, types.NamespacedName{Name: address.TargetRef.Name, Namespace: address.TargetRef.Namespace}, pod)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				continue
			}
			return ctrl.Result{}, err
		}

		serverLabel, ok := pod.Labels[v1alpha2.OtterizeServerLabelKey]
		if !ok {
			// only act on pods affected by Otterize policies
			return ctrl.Result{}, nil
		}

		netpolList := &v1.NetworkPolicyList{}
		// there's only ever one
		err = r.List(ctx, netpolList, client.MatchingLabels{v1alpha2.OtterizeNetworkPolicy: serverLabel}, client.Limit(1))
		if err != nil {
			if k8serrors.IsNotFound(err) {
				// only act on pods affected by Otterize policies - if they were not created yet,
				// the intents reconciler will call the endpoints reconciler once it does.
				continue
			}
			return ctrl.Result{}, err
		}

		if len(netpolList.Items) == 0 {
			if r.createEvenIfNoIntentsFound {
				result, err := r.ReconcileServiceForServiceWithoutIntents(ctx, endpoints, serverLabel, ingressList)
				if err != nil {
					return ctrl.Result{}, err
				}
				if !result.IsZero() {
					return result, nil
				}
			}
			continue
		}

		foundOtterizeNetpolsAffectingPods = true
		result, err := r.ReconcileServiceForOtterizeNetpol(ctx, endpoints, serverLabel, ingressList, &netpolList.Items[0])
		if err != nil {
			return ctrl.Result{}, err
		}
		if !result.IsZero() {
			return result, nil
		}

	}

	if !foundOtterizeNetpolsAffectingPods && !r.createEvenIfNoIntentsFound {
		policyName := r.formatPolicyName(endpoints.Name)
		result, err := r.handlePolicyDelete(ctx, policyName, endpoints.Namespace)
		if err != nil {
			return result, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *EndpointsReconcilerImpl) handlePolicyDelete(ctx context.Context, policyName string, policyNamespace string) (ctrl.Result, error) {

	policy := &v1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: policyName, Namespace: policyNamespace}, policy)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// nothing to do
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	err = r.Delete(ctx, policy)
	if err != nil {
		if k8serrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *EndpointsReconcilerImpl) ReconcileServiceForOtterizeNetpol(ctx context.Context, endpoints *corev1.Endpoints, otterizeServiceName string, ingressList *v1.IngressList, netpol *v1.NetworkPolicy) (ctrl.Result, error) {
	svc := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: endpoints.Name, Namespace: endpoints.Namespace}, svc)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.netpolCreator.handleNetworkPolicyCreationOrUpdateForNetpol(ctx, endpoints, svc, otterizeServiceName, svc, netpol, ingressList, r.formatPolicyName(endpoints.Name))
	if err != nil {
		if k8serrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *EndpointsReconcilerImpl) ReconcileServiceForServiceWithoutIntents(ctx context.Context, endpoints *corev1.Endpoints, otterizeServiceName string, ingressList *v1.IngressList) (ctrl.Result, error) {
	svc := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: endpoints.Name, Namespace: endpoints.Namespace}, svc)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.netpolCreator.handleNetworkPolicyCreationOrUpdateForServiceWithoutIntents(ctx, endpoints, svc, otterizeServiceName, svc, svc, ingressList, r.formatPolicyName(endpoints.Name))
	if err != nil {
		if k8serrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}
