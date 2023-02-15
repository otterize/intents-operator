package reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const OtterizeClientNameIndexField = "spec.service.name"

type PodWatcher struct {
	client.Client
	serviceIdResolver *serviceidresolver.Resolver
}

func NewPodWatcher(c client.Client) *PodWatcher {
	return &PodWatcher{Client: c, serviceIdResolver: serviceidresolver.NewResolver(c)}
}

func (p *PodWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logrus.Infof("Reconciling due to pod change: %s", req.Name)
	pod := v1.Pod{}
	err := p.Get(ctx, req.NamespacedName, &pod)
	if k8serrors.IsNotFound(err) {
		logrus.Infoln("Pod was deleted")
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	serviceID, err := p.serviceIdResolver.ResolvePodToServiceIdentity(ctx, &pod)
	if err != nil {
		return ctrl.Result{}, err
	}

	otterizeServerLabelValue := otterizev1alpha2.GetFormattedOtterizeIdentity(serviceID.Name, pod.Namespace)
	if !otterizev1alpha2.HasOtterizeServerLabel(&pod, otterizeServerLabelValue) {
		// Label pods as destination servers
		logrus.Infof("Labeling pod %s with server identity %s", pod.Name, serviceID)
		updatedPod := pod.DeepCopy()
		updatedPod.Labels[otterizev1alpha2.OtterizeServerLabelKey] = otterizeServerLabelValue

		err := p.Patch(ctx, updatedPod, client.MergeFrom(&pod))
		if err != nil {
			logrus.Errorln("Failed labeling pod as server", "Pod name", pod.Name, "Namespace", pod.Namespace)
			logrus.Errorln(err)
			return ctrl.Result{}, err
		}
	}

	// Intents were deleted and the pod was updated by the operator, skip reconciliation
	_, ok := pod.Annotations[otterizev1alpha2.AllIntentsRemovedAnnotation]
	if ok {
		logrus.Infof("Skipping reconciliation for pod %s - pod is handled by intents-operator", req.Name)
		return ctrl.Result{}, nil
	}

	var intents otterizev1alpha2.ClientIntentsList
	err = p.List(
		ctx, &intents,
		&client.MatchingFields{OtterizeClientNameIndexField: serviceID.Name},
		&client.ListOptions{Namespace: pod.Namespace})

	if err != nil {
		logrus.WithFields(logrus.Fields{"ServiceName": serviceID, "Namespace": pod.Namespace}).Errorln("Failed listing intents")
		return ctrl.Result{}, err
	}

	if len(intents.Items) == 0 {
		return ctrl.Result{}, nil
	}

	otterizeAccessLabels := map[string]string{}
	for _, intent := range intents.Items {
		currIntentLabels := intent.GetIntentsLabelMapping(pod.Namespace)
		for k, v := range currIntentLabels {
			otterizeAccessLabels[k] = v
		}
	}
	if otterizev1alpha2.IsMissingOtterizeAccessLabels(&pod, otterizeAccessLabels) {
		logrus.Infof("Updating Otterize access labels for %s", serviceID)
		updatedPod := otterizev1alpha2.UpdateOtterizeAccessLabels(pod.DeepCopy(), otterizeAccessLabels)
		err := p.Patch(ctx, updatedPod, client.MergeFrom(&pod))
		if err != nil {
			logrus.Errorf("Failed updating Otterize labels for pod %s in namespace %s", pod.Name, pod.Namespace)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (p *PodWatcher) InitIntentsClientIndices(mgr manager.Manager) error {
	err := mgr.GetCache().IndexField(
		context.Background(),
		&otterizev1alpha2.ClientIntents{},
		OtterizeClientNameIndexField,
		func(object client.Object) []string {
			intents := object.(*otterizev1alpha2.ClientIntents)
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

func (p *PodWatcher) Register(mgr manager.Manager) error {
	watcher, err := controller.New("otterize-pod-watcher", mgr, controller.Options{
		Reconciler: p,
	})
	if err != nil {
		return fmt.Errorf("unable to set up pods controller: %p", err)
	}

	if err = watcher.Watch(&source.Kind{Type: &v1.Pod{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return fmt.Errorf("unable to watch Pods: %p", err)
	}

	return nil
}
