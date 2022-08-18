package reconcilers

import (
	"context"
	otterizev1alpha1 "github.com/otterize/intents-operator/shared/api/v1alpha1"
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
	pods := &v1.PodList{}
	namespace := req.NamespacedName.Namespace

	intents := &otterizev1alpha1.Intents{}
	err := r.Get(ctx, req.NamespacedName, intents)
	if k8serrors.IsNotFound(err) {
		logrus.Infof("Intents deleted for namespace %s", namespace)
		return ctrl.Result{}, nil
	}

	if intents.Spec == nil {
		return ctrl.Result{}, nil
	}

	serviceName := intents.GetServiceName()
	intentLabels := intents.GetIntentsLabelMapping(namespace)
	logrus.Infof("Intent")
	// List the pods in the namespace and update labels if required
	err = r.List(ctx, pods, &client.ListOptions{Namespace: namespace})
	if err != nil {
		return ctrl.Result{}, err
	}

	for _, pod := range pods.Items {
		if strings.HasPrefix(pod.Name, serviceName) && otterizev1alpha1.LabelDiffExists(&pod, intentLabels) {
			logrus.Infof("Updating %s pod labels with new intents", serviceName)

			updatedPod := otterizev1alpha1.UpdateOtterizeAccessLabels(pod.DeepCopy(), intentLabels)
			err := r.Patch(ctx, updatedPod, client.MergeFrom(&pod))
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}
	return ctrl.Result{}, nil
}
