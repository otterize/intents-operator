package reconcilers

import (
	"context"
	otterizev1alpha1 "github.com/otterize/otternose/api/v1alpha1"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

type PodLabelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *PodLabelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// List the pods in the namespace and update labels if required
	pods := &v1.PodList{}
	namespace := req.NamespacedName.Namespace

	err := r.List(ctx, pods, &client.ListOptions{Namespace: namespace})
	if err != nil {
		return ctrl.Result{}, err
	}

	intents := &otterizev1alpha1.Intents{}
	err = r.Get(ctx, req.NamespacedName, intents)
	if k8serrors.IsNotFound(err) {
		logrus.Infof("No intents found for namespace %s\n", namespace)
		for _, pod := range pods.Items {
			// If the pod was never labeled by Otterize, carry on
			if _, ok := pod.Labels[otterizev1alpha1.OtterizeMarkerLabelKey]; !ok {
				continue
			}
			labels := cleanupOtterizeIntentLabels(pod.Labels)

			// Modify the pod's labels and update the object
			logrus.Debugf("Removing Otterize labels from pod: %s\n", pod.Name)
			pod.Labels = labels
			err := r.Update(ctx, &pod)
			if err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}

		return ctrl.Result{}, nil
	}

	serviceName := intents.GetServiceName()
	intentLabels := intents.GetIntentsLabelMapping(namespace)

	for _, pod := range pods.Items {
		// TODO: This is weak, change this
		if strings.HasPrefix(pod.Name, serviceName) && hasMissingOtterizeLabels(pod, intentLabels) {
			logrus.Infof("Updating %s pod labels with new intents\n", serviceName)
			logrus.Debugln(intentLabels)

			pod = updateOtterizeIntentLabels(pod, intentLabels)
			err := r.Update(ctx, &pod)
			if err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true}, nil
		}
	}

	return ctrl.Result{}, nil
}

// Check if this pod's labels need updating
func hasMissingOtterizeLabels(pod v1.Pod, labels map[string]string) bool {
	for k, _ := range labels {
		if _, ok := pod.Labels[k]; !ok {
			return true
		}
	}
	return false
}

func hasDestServerLabel(pod *v1.Pod) {
}

// updateOtterizeIntentLabels updates a pod's labels with Otterize labels representing their intents
// The pod is also labeled with "otterize-client=true" to mark it as having intents
func updateOtterizeIntentLabels(pod v1.Pod, labels map[string]string) v1.Pod {
	for k, v := range labels {
		pod.Labels[k] = v
	}
	pod.Labels[otterizev1alpha1.OtterizeMarkerLabelKey] = "true"
	return pod
}

// cleanupOtterizeIntentLabels Removes intent related labels from pods
// Returns the pod's label map without Otterize labels
func cleanupOtterizeIntentLabels(labels map[string]string) map[string]string {
	postCleanupLabels := map[string]string{}

	for k, v := range labels {
		if !isOtterizeLabelKey(k) {
			postCleanupLabels[k] = v
		}
	}
	return postCleanupLabels
}

func isOtterizeLabelKey(s string) bool {
	return !strings.Contains(s, "otterize-access") && !strings.Contains(s, "otterize-client")
}
