package intents_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ReasonRemovingPodLabelsFailed = "RemovingPodLabelsFailed"
	ReasonUpdatePodFailed         = "UpdatePodFailed"
	ReasonListPodsFailed          = "ListPodsFailed"
)

type PodLabelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	injectablerecorder.InjectableRecorder
}

func NewPodLabelReconciler(c client.Client, s *runtime.Scheme) *PodLabelReconciler {
	return &PodLabelReconciler{
		Client: c,
		Scheme: s,
	}
}

func (r *PodLabelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	namespace := req.NamespacedName.Namespace

	intents := &otterizev1alpha3.ClientIntents{}
	err := r.Get(ctx, req.NamespacedName, intents)
	if k8serrors.IsNotFound(err) {
		logrus.WithField("namespacedName", req.String()).Infof("Intents deleted")
		return ctrl.Result{}, nil
	}

	if intents.Spec == nil {
		return ctrl.Result{}, nil
	}

	if !intents.DeletionTimestamp.IsZero() {
		err := r.removeLabelsFromPods(ctx, intents)
		if err != nil {
			// TODO: check IsConflict error next to the actual client call
			if k8serrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			r.RecordWarningEventf(intents, ReasonRemovingPodLabelsFailed, "could not remove pod labels: %s", err.Error())
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	serviceName := intents.GetServiceName()
	intentLabels := intents.GetIntentsLabelMapping(namespace)

	// List the pods in the namespace and update labels if required
	labelSelector, err := intents.BuildPodLabelSelector()
	if err != nil {
		r.RecordWarningEventf(intents, ReasonListPodsFailed, "could not list pods: %s", err.Error())
		return ctrl.Result{}, err
	}

	podList := &v1.PodList{}
	err = r.List(ctx, podList,
		&client.ListOptions{Namespace: namespace},
		client.MatchingLabelsSelector{Selector: labelSelector})
	if err != nil {
		return ctrl.Result{}, err
	}

	for _, pod := range podList.Items {
		if otterizev1alpha3.IsMissingOtterizeAccessLabels(&pod, intentLabels) {
			logrus.Infof("Updating %s pod labels with new intents", serviceName)
			updatedPod := otterizev1alpha3.UpdateOtterizeAccessLabels(pod.DeepCopy(), serviceName, intentLabels)
			err := r.Patch(ctx, updatedPod, client.MergeFrom(&pod))
			if err != nil {
				r.RecordWarningEventf(intents, ReasonUpdatePodFailed, "could not update pod: %s", err.Error())
				return ctrl.Result{}, err
			}
		}
	}
	return ctrl.Result{}, nil
}

func (r *PodLabelReconciler) removeLabelsFromPods(
	ctx context.Context, intents *otterizev1alpha3.ClientIntents) error {

	logrus.Infof("Unlabeling pods for Otterize service %s", intents.Spec.Service.Name)

	labelSelector, err := intents.BuildPodLabelSelector()
	if err != nil {
		return err
	}

	podList := &v1.PodList{}
	err = r.List(ctx, podList,
		&client.ListOptions{Namespace: intents.Namespace},
		client.MatchingLabelsSelector{Selector: labelSelector})
	if err != nil {
		return err
	}

	// Remove the access label for each intent, for every pod in the list
	for _, pod := range podList.Items {
		updatedPod := pod.DeepCopy()
		if updatedPod.Annotations == nil {
			updatedPod.Annotations = make(map[string]string)
		}
		updatedPod.Annotations[otterizev1alpha3.AllIntentsRemovedAnnotation] = "true"
		for _, intent := range intents.GetCallsList() {
			targetServerIdentity := otterizev1alpha3.GetFormattedOtterizeIdentity(
				intent.Name, intent.GetTargetServerNamespace(intents.Namespace))

			accessLabel := fmt.Sprintf(otterizev1alpha3.OtterizeAccessLabelKey, targetServerIdentity)
			delete(updatedPod.Labels, accessLabel)
		}

		err := r.Patch(ctx, updatedPod, client.MergeFrom(&pod))
		if err != nil {
			return err
		}
	}

	return nil
}
