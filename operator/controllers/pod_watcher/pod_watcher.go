package pod_watcher

import (
	"context"
	"fmt"
	otterizev1alpha1 "github.com/otterize/otternose/api/v1alpha1"
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

type PodWatcher struct {
	client.Client
}

func NewPodWatcher(c client.Client) *PodWatcher {
	return &PodWatcher{c}
}

func (w *PodWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logrus.Infoln("Reconciling due to pod change")
	pod := &v1.Pod{}

	err := w.Get(ctx, req.NamespacedName, pod)
	if k8serrors.IsNotFound(err) {
		logrus.Infoln("Pod was deleted")
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	owner, err := w.ResolvePodToServiceIdentity(ctx, pod)
	if err != nil {
		return ctrl.Result{}, err
	}

	intents := getIntentsObjForClient(owner.Name, owner.Namespace)
	err = w.Get(ctx, req.NamespacedName, &intents)
	if err != nil {
		return ctrl.Result{}, nil
	}

	logrus.Infoln("INTENTS!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
	logrus.Infof("%v\n", intents)

	return ctrl.Result{}, nil
}

func (w *PodWatcher) ResolvePodToServiceIdentity(ctx context.Context, pod *v1.Pod) (*types.NamespacedName, error) {
	for _, owner := range pod.OwnerReferences {
		namespacedName := types.NamespacedName{Name: owner.Name, Namespace: pod.Namespace}
		switch owner.Kind {
		case "ReplicaSet":
			rs := &appsv1.ReplicaSet{}
			err := w.Get(ctx, namespacedName, rs)
			if err != nil {
				return nil, err
			}
			return &types.NamespacedName{Name: rs.OwnerReferences[0].Name, Namespace: pod.Namespace}, nil
		case "DaemonSet":
			ds := &appsv1.DaemonSet{}
			err := w.Get(ctx, namespacedName, ds)
			if err != nil {
				return nil, err
			}
			return &types.NamespacedName{Name: ds.Name, Namespace: pod.Namespace}, nil
		case "StatefulSet":
			ss := &appsv1.StatefulSet{}
			err := w.Get(ctx, namespacedName, ss)
			if err != nil {
				return nil, err
			}
			return &types.NamespacedName{Name: ss.Name, Namespace: pod.Namespace}, nil
		default:
			logrus.Infof("Unknown owner kind %s for pod %s", owner.Kind, pod.Name)
		}
	}
	return nil, fmt.Errorf("pod %s has no owner", pod.Name)
}

func getIntentsObjForClient(name string, ns string) otterizev1alpha1.Intents {
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

func (w *PodWatcher) labelPodServerIdentity(pod *v1.Pod, serverName string) {

}

func (w *PodWatcher) Register(mgr manager.Manager) error {
	watcher, err := controller.New("pod-watcher", mgr, controller.Options{
		Reconciler: w,
	})
	if err != nil {
		return fmt.Errorf("unable to set up pods controller: %w", err)
	}

	err = watcher.Watch(&source.Kind{Type: &v1.Pod{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return fmt.Errorf("unable to watch Pods: %w", err)
	}
	return nil
}
