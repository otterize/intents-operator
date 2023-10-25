package port_network_policy

import (
	"context"
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ServiceWatcher struct {
	client.Client
	reconcilers []reconcile.Reconciler
	injectablerecorder.InjectableRecorder
}

func NewServiceWatcher(c client.Client, eventRecorder record.EventRecorder, reconcilers []reconcile.Reconciler) *ServiceWatcher {
	recorder := injectablerecorder.InjectableRecorder{Recorder: eventRecorder}
	return &ServiceWatcher{
		Client:             c,
		InjectableRecorder: recorder,
		reconcilers:        reconcilers,
	}
}

func (r *ServiceWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// We should reconcile everytime a service changes.
	// When a target port changes netpols should be updated, etc.
	service := corev1.Service{}
	err := r.Get(ctx, req.NamespacedName, &service)
	if err != nil && !k8serrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	var intentsList otterizev1alpha3.ClientIntentsList
	err = r.List(
		ctx, &intentsList,
		&client.MatchingFields{otterizev1alpha3.OtterizeTargetServerIndexField: fmt.Sprintf("svc:%s.%s", req.Name, req.Namespace)})

	if err != nil {
		return ctrl.Result{}, err
	}

	// Reconcile for any clientIntent in the cluster that points to the service enqueued in the request
	for _, clientIntent := range intentsList.Items {
		for _, reconciler := range r.reconcilers {
			res, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
				Namespace: clientIntent.Namespace,
				Name:      clientIntent.Name,
			}})
			if err != nil {
				return ctrl.Result{}, err
			}
			if !res.IsZero() {
				return res, nil
			}
		}
	}

	err = r.removeOrphanEgressServiceNetpols(ctx)
	if err != nil {
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func matchEgressAccessNetworkPolicy() (labels.Selector, error) {
	isOtterizeNetworkPolicy := metav1.LabelSelectorRequirement{
		Key:      otterizev1alpha3.OtterizeSvcEgressNetworkPolicy,
		Operator: metav1.LabelSelectorOpExists,
	}
	return metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		isOtterizeNetworkPolicy,
	}})
}

// removeOrphanEgressServiceNetpols removes orphaned egress service network policies. Normally, for service network policies,
// the network policy has an owner reference to the service, which takes care of the deletion.
// But for egress policies, the network policy is in the client's namespace, which is potentially in a different namespace
// than the service, and cross-namespace owner references are not possible.
// To resolve this, we must either use a finalizer on the service (which would be very harmful if for any reason we fail to remove it),
// or, this implementation, which lists all service egress network policies, and for each of them (O(n)) checks if the service
// exists (O(1)). If not, it deletes the netpol.
func (r *ServiceWatcher) removeOrphanEgressServiceNetpols(ctx context.Context) error {
	networkPolicyList := &v1.NetworkPolicyList{}
	selector, err := matchEgressAccessNetworkPolicy()
	if err != nil {
		return err
	}

	err = r.List(ctx, networkPolicyList, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return err
	}

	for _, netpol := range networkPolicyList.Items {
		svcName, ok := netpol.Annotations[otterizev1alpha3.OtterizeSvcEgressNetworkPolicyTargetService]
		if !ok {
			return err
		}

		svcNamespace, ok := netpol.Annotations[otterizev1alpha3.OtterizeSvcEgressNetworkPolicyTargetServiceNamespace]
		if !ok {
			return err
		}

		netpolSvc := &corev1.Service{}
		err := r.Get(ctx, types.NamespacedName{
			Namespace: svcNamespace,
			Name:      svcName,
		}, netpolSvc)
		if k8serrors.IsNotFound(err) || netpolSvc.DeletionTimestamp != nil {
			err := r.Delete(ctx, &netpol)
			if k8serrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return err
			}
		}

		if err != nil {
			return err
		}

	}
	return nil
}

func (r *ServiceWatcher) SetupWithManager(mgr manager.Manager) error {
	recorder := mgr.GetEventRecorderFor("intents-operator")
	r.InjectRecorder(recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}
