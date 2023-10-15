package port_network_policy

import (
	"context"
	"fmt"
	"github.com/amit7itz/goset"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type ServiceWatcher struct {
	client.Client
	networkPolicyReconciler *PortNetworkPolicyReconciler
	injectablerecorder.InjectableRecorder
}

func NewServiceWatcher(c client.Client, eventRecorder record.EventRecorder, networkPolicyReconciler *PortNetworkPolicyReconciler) *ServiceWatcher {
	recorder := injectablerecorder.InjectableRecorder{Recorder: eventRecorder}
	return &ServiceWatcher{
		Client:                  c,
		InjectableRecorder:      recorder,
		networkPolicyReconciler: networkPolicyReconciler,
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

	var intentsList otterizev1alpha2.ClientIntentsList
	err = r.List(
		ctx, &intentsList,
		&client.MatchingFields{otterizev1alpha2.OtterizeTargetServerIndexField: fmt.Sprintf("svc:%s.%s", req.Name, req.Namespace)})

	if err != nil {
		return ctrl.Result{}, err
	}

	// Reconcile for any clientIntent in the cluster that points to the service enqueued in the request
	for _, clientIntent := range intentsList.Items {
		res, err := r.networkPolicyReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
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

	err = r.reconcileServiceLabelsOnPods(ctx, &service)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ServiceWatcher) reconcileServiceLabelsOnPods(ctx context.Context, service *corev1.Service) error {
	formattedTargetServer := otterizev1alpha2.GetFormattedOtterizeIdentity(service.Name, service.Namespace)
	kubernetesServiceLabelKey := fmt.Sprintf(otterizev1alpha2.OtterizeKubernetesServiceLabelKey, formattedTargetServer)

	currentlyLabeledPodList := corev1.PodList{}
	err := r.List(ctx, &currentlyLabeledPodList, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{kubernetesServiceLabelKey: "true"}),
		Namespace:     service.Namespace,
	})
	if err != nil {
		return err
	}

	if len(currentlyLabeledPodList.Items) == 0 {
		return nil
	}
	// Kubernetes service is being deleted, remove labels from pods
	if !service.DeletionTimestamp.IsZero() {
		for _, pod := range currentlyLabeledPodList.Items {
			updatedPod := pod.DeepCopy()
			delete(updatedPod.Labels, kubernetesServiceLabelKey)
			if err := r.Patch(ctx, updatedPod, client.MergeFrom(&pod)); err != nil {
				return err
			}
		}
		return nil
	}

	shouldBeLabeledPodList := corev1.PodList{}
	err = r.List(ctx, &shouldBeLabeledPodList, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(service.Spec.Selector),
		Namespace:     service.Namespace,
	})
	if err != nil {
		return err
	}

	// Add missing labels
	for _, pod := range shouldBeLabeledPodList.Items {
		if _, ok := pod.Labels[kubernetesServiceLabelKey]; !ok {
			updatedPod := pod.DeepCopy()
			updatedPod.Labels[kubernetesServiceLabelKey] = "true"
			if err := r.Patch(ctx, updatedPod, client.MergeFrom(&pod)); err != nil {
				return err
			}
		}
	}

	// Create pod UIDs set
	shouldBeLabeledUIDs := lo.Map(shouldBeLabeledPodList.Items, func(pod corev1.Pod, _ int) types.UID {
		return pod.UID
	})
	UIDSet := goset.FromSlice(shouldBeLabeledUIDs)

	// Compare current state with required state, unlabel pods if no longer selected by service
	for _, pod := range currentlyLabeledPodList.Items {
		if !UIDSet.Contains(pod.UID) { // Pod is labeled for the service but shouldn't be
			updatedPod := pod.DeepCopy()
			delete(updatedPod.Labels, kubernetesServiceLabelKey)
			if err := r.Patch(ctx, updatedPod, client.MergeFrom(&pod)); err != nil {
				return err
			}
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
