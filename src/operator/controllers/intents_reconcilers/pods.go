package intents_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha1 "github.com/otterize/intents-operator/shared/api/v1alpha1"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"
)

const PodLabelFinalizerName = "otterize-intents.pods/finalizer"

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

	if !intents.DeletionTimestamp.IsZero() {
		err := r.cleanFinalizerAndUnlabelPods(ctx, intents)
		if err != nil {
			if k8serrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(intents, PodLabelFinalizerName) {
		logrus.WithField("namespacedName", req.String()).Infof("Adding finalizer %s", PodLabelFinalizerName)
		controllerutil.AddFinalizer(intents, PodLabelFinalizerName)
		if err := r.Update(ctx, intents); err != nil {
			return ctrl.Result{}, err
		}
	}

	serviceName := intents.GetServiceName()
	intentLabels := intents.GetIntentsLabelMapping(namespace)
	// List the pods in the namespace and update labels if required
	err = r.List(ctx, pods, &client.ListOptions{Namespace: namespace})
	if err != nil {
		return ctrl.Result{}, err
	}

	for _, pod := range pods.Items {
		if strings.HasPrefix(pod.Name, serviceName) && otterizev1alpha1.IsMissingOtterizeAccessLabels(&pod, intentLabels) {
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

func (r *PodLabelReconciler) cleanFinalizerAndUnlabelPods(
	ctx context.Context, intents *otterizev1alpha1.Intents) error {

	if !controllerutil.ContainsFinalizer(intents, PodLabelFinalizerName) {
		return nil
	}

	logrus.Infof("Unlabeling pods for Otterize service %s", intents.Spec.Service.Name)
	// We're getting the pods matched
	labelSelector, err := labels.Parse(
		fmt.Sprintf("%s=%s",
			otterizev1alpha1.OtterizeServerLabelKey,
			// Since all pods are also labeled with their server identity, we use the Otterize server label
			// To find all pods for this specific service
			otterizev1alpha1.GetFormattedOtterizeIdentity(intents.Spec.Service.Name, intents.Namespace)))
	if err != nil {
		return err
	}

	podList := &v1.PodList{}
	err = r.List(ctx, podList, client.MatchingLabelsSelector{Selector: labelSelector})
	if err != nil {
		return err
	}

	// Remove the access label for each intent, for every pod in the list
	for _, pod := range podList.Items {
		updatedPod := pod.DeepCopy()
		updatedPod.Annotations[otterizev1alpha1.AllIntentsRemoved] = "true"
		for _, intent := range intents.GetCallsList() {
			targetServerIdentity := otterizev1alpha1.GetFormattedOtterizeIdentity(
				intent.Server, intent.ResolveIntentNamespace(intents.Namespace))

			accessLabel := fmt.Sprintf(otterizev1alpha1.OtterizeAccessLabelKey, targetServerIdentity)
			delete(updatedPod.Labels, accessLabel)
		}

		err := r.Patch(ctx, updatedPod, client.MergeFrom(&pod))
		if err != nil {
			return err
		}
	}

	controllerutil.RemoveFinalizer(intents, PodLabelFinalizerName)
	if err := r.Update(ctx, intents); err != nil {
		return err
	}

	return nil
}
