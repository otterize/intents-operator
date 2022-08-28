package reconcilers

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	OwnerTypeReplicaSet  = "ReplicaSet"
	OwnerTypeStatefulSet = "StatefulSet"
	OwnerTypeDaemonSet   = "DaemonSet"
	OwnerTypeDeployment  = "Deployment"
)

const OtterizeClientNameIndexField = "spec.service.name"

type PodWatcher struct {
	client.Client
}

func NewPodWatcher(c client.Client) *PodWatcher {
	return &PodWatcher{c}
}

func (w *PodWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logrus.Infof("Reconciling due to pod change: %s", req.Name)
	pod := v1.Pod{}
	err := w.Get(ctx, req.NamespacedName, &pod)
	if k8serrors.IsNotFound(err) {
		logrus.Infoln("Pod was deleted")
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	otterizeIdentity, err := w.resolvePodToOtterizeIdentity(ctx, &pod)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !v1alpha1.HasOtterizeServerLabel(&pod) {
		// Label pods as destination servers
		logrus.Infof("Labeling pod %s with server identity %s", pod.Name, otterizeIdentity.Name)
		updatedPod := pod.DeepCopy()
		updatedPod.Labels[v1alpha1.OtterizeServerLabelKey] =
			v1alpha1.GetFormattedOtterizeIdentity(otterizeIdentity.Name, otterizeIdentity.Namespace)

		err := w.Patch(ctx, updatedPod, client.MergeFrom(&pod))
		if err != nil {
			logrus.Errorln("Failed labeling pod as server", "Pod name", pod.Name, "Namespace", pod.Namespace)
			logrus.Errorln(err)
			return ctrl.Result{}, err
		}
	}

	var intents v1alpha1.IntentsList
	err = w.List(
		ctx, &intents,
		&client.MatchingFields{OtterizeClientNameIndexField: otterizeIdentity.Name},
		&client.ListOptions{Namespace: otterizeIdentity.Namespace})

	if err != nil {
		logrus.Errorln("Failed listing intents", "Service name",
			otterizeIdentity.Name, "Namespace", otterizeIdentity.Namespace)
		return ctrl.Result{}, err
	}

	if len(intents.Items) == 0 {
		return ctrl.Result{}, nil
	}

	otterizeAccessLabels := map[string]string{}
	for _, intent := range intents.Items {
		currIntentLabels := intent.GetIntentsLabelMapping(otterizeIdentity.Namespace)
		for k, v := range currIntentLabels {
			otterizeAccessLabels[k] = v
		}
	}
	if v1alpha1.IsMissingOtterizeAccessLabels(&pod, otterizeAccessLabels) {
		logrus.Infof("Updating Otterize access labels for %s", otterizeIdentity.Name)
		updatedPod := v1alpha1.UpdateOtterizeAccessLabels(pod.DeepCopy(), otterizeAccessLabels)
		err := w.Patch(ctx, updatedPod, client.MergeFrom(&pod))
		if err != nil {
			logrus.Errorln("Failed updating Otterize labels for pod", "Pod name",
				pod.Name, "Namespace", pod.Namespace)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (w *PodWatcher) InitIntentIndexes(mgr manager.Manager) error {
	err := mgr.GetCache().IndexField(
		context.Background(),
		&v1alpha1.Intents{},
		OtterizeClientNameIndexField,
		func(object client.Object) []string {
			intents := object.(*v1alpha1.Intents)
			if intents.Spec == nil {
				return nil
			}
			return []string{intents.Spec.Service.Name}
		})

	if err != nil {
		return err
	}

	return nil
}

func (w *PodWatcher) resolvePodToOtterizeIdentity(ctx context.Context, pod *v1.Pod) (*types.NamespacedName, error) {
	var otterizeIdentity string
	var ownerKind client.Object
	for _, owner := range pod.OwnerReferences {
		namespacedName := types.NamespacedName{Name: owner.Name, Namespace: pod.Namespace}
		switch owner.Kind {
		case OwnerTypeReplicaSet:
			ownerKind = &appsv1.ReplicaSet{}
		case OwnerTypeDaemonSet:
			ownerKind = &appsv1.DaemonSet{}
		case OwnerTypeStatefulSet:
			ownerKind = &appsv1.StatefulSet{}
		case OwnerTypeDeployment:
			ownerKind = &appsv1.Deployment{}
		default:
			logrus.Infof("Unknown owner kind %s for pod %s", owner.Kind, pod.Name)
		}
		err := w.Get(ctx, namespacedName, ownerKind)
		if err != nil {
			return nil, err
		}
		otterizeIdentity = w.getOtterizeIdentityFromObject(ownerKind)
		return &types.NamespacedName{Name: otterizeIdentity, Namespace: pod.Namespace}, nil
	}

	return nil, fmt.Errorf("pod %s has no owner", pod.Name)
}

func (w *PodWatcher) getOtterizeIdentityFromObject(obj client.Object) string {
	owners := obj.GetOwnerReferences()
	if len(owners) != 0 {
		return owners[0].Name
	}
	return obj.GetName()
}

func (w *PodWatcher) Register(mgr manager.Manager) error {
	watcher, err := controller.New("pod-watcher", mgr, controller.Options{
		Reconciler: w,
	})
	if err != nil {
		return fmt.Errorf("unable to set up pods controller: %w", err)
	}

	if err = watcher.Watch(&source.Kind{Type: &v1.Pod{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return fmt.Errorf("unable to watch Pods: %w", err)
	}

	return nil
}
