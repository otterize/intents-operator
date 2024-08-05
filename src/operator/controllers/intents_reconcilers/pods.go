package intents_reconcilers

import (
	"context"
	"fmt"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/prometheus"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/sirupsen/logrus"
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

	intents := &otterizev2alpha1.ClientIntents{}
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
			return ctrl.Result{}, errors.Wrap(err)
		}
		return ctrl.Result{}, nil
	}

	intentLabels := intents.GetIntentsLabelMapping(namespace)
	serviceIdentity := intents.ToServiceIdentity()

	pods, ok, err := serviceidresolver.NewResolver(r.Client).ResolveServiceIdentityToPodSlice(ctx, serviceIdentity)
	if err != nil {
		r.RecordWarningEventf(intents, ReasonListPodsFailed, "could not list pods: %s", err.Error())
		return ctrl.Result{}, errors.Wrap(err)
	}

	if !ok {
		logrus.Debugf("No pods found for service %s", serviceIdentity.Name)
		return ctrl.Result{}, nil
	}

	for _, pod := range pods {
		if otterizev2alpha1.IsMissingOtterizeAccessLabels(&pod, intentLabels) {
			logrus.Debugf("Updating %s pod labels with new intents", serviceIdentity.Name)
			updatedPod := otterizev2alpha1.UpdateOtterizeAccessLabels(pod.DeepCopy(), serviceIdentity, intentLabels)
			err := r.Patch(ctx, updatedPod, client.StrategicMergeFrom(&pod))
			if err != nil {
				r.RecordWarningEventf(intents, ReasonUpdatePodFailed, "could not update pod: %s", err.Error())
				return ctrl.Result{}, errors.Wrap(err)
			}
			prometheus.IncrementPodsLabeledForNetworkPolicies(1)
		}
	}
	return ctrl.Result{}, nil
}

func (r *PodLabelReconciler) removeLabelsFromPods(
	ctx context.Context, intents *otterizev2alpha1.ClientIntents) error {

	logrus.Debugf("Unlabeling pods for Otterize service %s", intents.Spec.Workload.Name)

	si := intents.ToServiceIdentity()
	pods, ok, err := serviceidresolver.NewResolver(r.Client).ResolveServiceIdentityToPodSlice(ctx, si)
	if err != nil {
		return errors.Wrap(err)
	}

	if !ok {
		logrus.Debugf("No pods found for service %s", si.Name)
		return nil
	}

	// Remove the access label for each intent, for every pod in the list
	for _, pod := range pods {
		updatedPod := pod.DeepCopy()
		if updatedPod.Annotations == nil {
			updatedPod.Annotations = make(map[string]string)
		}
		updatedPod.Annotations[otterizev2alpha1.AllIntentsRemovedAnnotation] = "true"
		for _, intent := range intents.GetTargetList() {
			targetServerIdentity := intent.ToServiceIdentity(intents.Namespace)
			accessLabel := fmt.Sprintf(otterizev2alpha1.OtterizeAccessLabelKey, targetServerIdentity.GetFormattedOtterizeIdentityWithoutKind())
			delete(updatedPod.Labels, accessLabel)
		}

		prometheus.IncrementPodsUnlabeledForNetworkPolicies(1)
		err := r.Patch(ctx, updatedPod, client.StrategicMergeFrom(&pod))
		if err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}
