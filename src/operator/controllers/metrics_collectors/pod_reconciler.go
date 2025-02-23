package metrics_collectors

import (
	"context"
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;update;patch;list;watch;delete;create

type PodReconciler struct {
	client.Client
	netpolHandle *NetworkPolicyHandler
	injectablerecorder.InjectableRecorder
}

func NewPodReconciler(client client.Client, netpolHandle *NetworkPolicyHandler) *PodReconciler {
	return &PodReconciler{
		Client:       client,
		netpolHandle: netpolHandle,
	}
}

func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	recorder := mgr.GetEventRecorderFor("intents-operator")
	r.InjectRecorder(recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}

func (r *PodReconciler) InjectRecorder(recorder record.EventRecorder) {
	r.Recorder = recorder
	r.netpolHandle.InjectRecorder(recorder)
}

func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pod := &corev1.Pod{}
	err := r.Get(ctx, req.NamespacedName, pod)
	if k8serrors.IsNotFound(err) {
		// delete is handled by garbage collection - the service owns the network policy
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	var endpointsList corev1.EndpointsList

	err = r.Client.List(
		ctx,
		&endpointsList,
		&client.MatchingFields{v2alpha1.EndpointsPodNamesIndexField: pod.Name},
		&client.ListOptions{Namespace: pod.Namespace},
	)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	for _, endpoints := range endpointsList.Items {
		err = r.netpolHandle.HandleEndpoints(ctx, &endpoints)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err)
		}
	}

	return ctrl.Result{}, nil
}
