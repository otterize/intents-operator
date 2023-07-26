package external_traffic

import (
	"context"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

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
	extNetpolHandler *NetworkPolicyHandler
	injectablerecorder.InjectableRecorder
}

func NewEndpointsReconciler(client client.Client, extNetpolHandler *NetworkPolicyHandler) EndpointsReconciler {
	return &EndpointsReconcilerImpl{
		Client:           client,
		extNetpolHandler: extNetpolHandler,
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
	r.extNetpolHandler.InjectRecorder(recorder)
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

	err = r.extNetpolHandler.HandleEndpoints(ctx, endpoints)

	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
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
