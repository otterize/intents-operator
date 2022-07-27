package reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha1 "github.com/otterize/intents-operator/shared/api/v1alpha1"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

const OtterizeDestServerLabelKey = "otterize-server"

type PodWatcher struct {
	client.Client
}

func NewPodWatcher(c client.Client) *PodWatcher {
	return &PodWatcher{c}
}

func (w *PodWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logrus.Infof("Reconciling due to pod change: %s\n", req.Name)
	pod := &v1.Pod{}
	err := w.Get(ctx, req.NamespacedName, pod)
	if k8serrors.IsNotFound(err) {
		logrus.Infoln("Pod was deleted")
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	otterizeIdentity, err := w.ResolvePodToOtterizeIdentity(ctx, pod)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !w.hasOtterizeServerLabel(pod) {
		// Label pods as destination servers
		logrus.Infof("Labeling pod %s with server identity %s", pod.Name, otterizeIdentity.Name)
		pod.Labels[OtterizeDestServerLabelKey] = otterizeIdentity.Name
		err := w.Update(ctx, pod)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Validate pods are aligned with intents as clients
	intents := w.getIntentsObjForClient(otterizeIdentity.Name, otterizeIdentity.Namespace)
	err = w.Get(ctx, req.NamespacedName, &intents)
	if err != nil {
		logrus.Errorln(err)
		return ctrl.Result{}, err
	}

	logrus.Infoln("INTENTS!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
	logrus.Infof("%v\n", intents)

	return ctrl.Result{}, nil
}

func (w *PodWatcher) ResolvePodToOtterizeIdentity(ctx context.Context, pod *v1.Pod) (*types.NamespacedName, error) {
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

func (w *PodWatcher) getIntentsObjForClient(name string, ns string) otterizev1alpha1.Intents {
	return otterizev1alpha1.Intents{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
		},
		Spec: &otterizev1alpha1.IntentsSpec{
			Service: otterizev1alpha1.Service{
				Name: name,
			},
		},
	}
}

func (w *PodWatcher) getOtterizeIdentityFromObject(obj client.Object) string {
	owners := obj.GetOwnerReferences()
	if len(owners) != 0 {
		return owners[0].Name
	}
	return obj.GetName()
}

func (w *PodWatcher) labelPodServerIdentity(pod *v1.Pod, serverName string) {

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

func (w *PodWatcher) hasOtterizeServerLabel(pod *v1.Pod) bool {
	_, exists := pod.Labels[OtterizeDestServerLabelKey]
	return exists
}
