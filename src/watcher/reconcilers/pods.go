package reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	istiopolicy2 "github.com/otterize/intents-operator/src/operator/controllers/istiopolicy"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	OtterizeClientNameIndexField = "spec.service.name"
)

type PodWatcher struct {
	client.Client
	serviceIdResolver  *serviceidresolver.Resolver
	istioPolicyCreator *istiopolicy2.Creator
	injectablerecorder.InjectableRecorder
}

func NewPodWatcher(c client.Client, eventRecorder record.EventRecorder, watchedNamespaces []string) *PodWatcher {
	recorder := injectablerecorder.InjectableRecorder{Recorder: eventRecorder}
	creator := istiopolicy2.NewCreator(c, &recorder, watchedNamespaces)
	return &PodWatcher{
		Client:             c,
		serviceIdResolver:  serviceidresolver.NewResolver(c),
		istioPolicyCreator: creator,
		InjectableRecorder: recorder,
	}
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

	err = p.addOtterizePodLabels(ctx, req, serviceID, pod)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = p.handleIstioPolicy(ctx, pod, serviceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (p *PodWatcher) handleIstioPolicy(ctx context.Context, pod v1.Pod, serviceID serviceidresolver.ServiceIdentity) error {
	if !p.istioEnforcementEnabled() || pod.DeletionTimestamp != nil {
		return nil
	}

	err := p.updateServerSideCar(ctx, pod, serviceID)
	if err != nil {
		return err
	}

	var intents otterizev1alpha2.ClientIntentsList
	err = p.List(
		ctx,
		&intents,
		&client.MatchingFields{OtterizeClientNameIndexField: serviceID.Name},
		&client.ListOptions{Namespace: pod.Namespace})
	if err != nil {
		logrus.WithFields(logrus.Fields{"ServiceName": serviceID, "Namespace": pod.Namespace}).Errorln("Failed listing intents")
		return err
	}

	if len(intents.Items) == 0 {
		return nil
	}

	for _, clientIntents := range intents.Items {
		err = p.createIstioPolicies(ctx, clientIntents, pod)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *PodWatcher) updateServerSideCar(ctx context.Context, pod v1.Pod, serviceID serviceidresolver.ServiceIdentity) error {
	missingSideCar := !istiopolicy2.IsPodPartOfIstioMesh(pod)

	serviceFullName := fmt.Sprintf("%s.%s", serviceID.Name, pod.Namespace)
	var intentsList otterizev1alpha2.ClientIntentsList
	err := p.List(
		ctx, &intentsList,
		&client.MatchingFields{otterizev1alpha2.OtterizeTargetServerIndexField: serviceFullName})
	if err != nil {
		return err
	}

	if len(intentsList.Items) == 0 {
		return nil
	}

	for _, clientIntents := range intentsList.Items {
		formattedTargetServer := otterizev1alpha2.GetFormattedOtterizeIdentity(serviceID.Name, pod.Namespace)
		err = p.istioPolicyCreator.UpdateServerSidecar(ctx, &clientIntents, formattedTargetServer, missingSideCar)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *PodWatcher) addOtterizePodLabels(ctx context.Context, req ctrl.Request, serviceID serviceidresolver.ServiceIdentity, pod v1.Pod) error {
	otterizeServerLabelValue := otterizev1alpha2.GetFormattedOtterizeIdentity(serviceID.Name, pod.Namespace)
	if !otterizev1alpha2.HasOtterizeServerLabel(&pod, otterizeServerLabelValue) {
		// Label pods as destination servers
		logrus.Infof("Labeling pod %s with server identity %s", pod.Name, serviceID.Name)
		updatedPod := pod.DeepCopy()
		if updatedPod.Labels == nil {
			updatedPod.Labels = make(map[string]string)
		}
		updatedPod.Labels[otterizev1alpha2.OtterizeServerLabelKey] = otterizeServerLabelValue

		err := p.Patch(ctx, updatedPod, client.MergeFrom(&pod))
		if err != nil {
			logrus.WithError(err).Errorln("Failed labeling pod as server", "Pod name", pod.Name, "Namespace", pod.Namespace)
			return err
		}
		return nil
	}

	// Intents were deleted and the pod was updated by the operator, skip reconciliation
	_, ok := pod.Annotations[otterizev1alpha2.AllIntentsRemovedAnnotation]
	if ok {
		logrus.Infof("Skipping reconciliation for pod %s - pod is handled by intents-operator", req.Name)
		return nil
	}

	var intents otterizev1alpha2.ClientIntentsList
	err := p.List(
		ctx, &intents,
		&client.MatchingFields{OtterizeClientNameIndexField: serviceID.Name},
		&client.ListOptions{Namespace: pod.Namespace})

	if err != nil {
		logrus.WithFields(logrus.Fields{"ServiceName": serviceID, "Namespace": pod.Namespace}).Errorln("Failed listing intents")
		return err
	}

	if len(intents.Items) == 0 {
		return nil
	}

	otterizeAccessLabels := map[string]string{}
	for _, intent := range intents.Items {
		currIntentLabels := intent.GetIntentsLabelMapping(pod.Namespace)
		for k, v := range currIntentLabels {
			otterizeAccessLabels[k] = v
		}
	}
	if otterizev1alpha2.IsMissingOtterizeAccessLabels(&pod, otterizeAccessLabels) {
		logrus.Infof("Updating Otterize access labels for %s", serviceID.Name)
		updatedPod := otterizev1alpha2.UpdateOtterizeAccessLabels(pod.DeepCopy(), otterizeAccessLabels)
		err := p.Patch(ctx, updatedPod, client.MergeFrom(&pod))
		if err != nil {
			logrus.Errorf("Failed updating Otterize labels for pod %s in namespace %s", pod.Name, pod.Namespace)
			return err
		}
	}

	return nil
}

func (p *PodWatcher) istioEnforcementEnabled() bool {
	return viper.GetBool(operatorconfig.EnableIstioPolicyKey)
}

func (p *PodWatcher) createIstioPolicies(ctx context.Context, intents otterizev1alpha2.ClientIntents, pod v1.Pod) error {
	if intents.DeletionTimestamp != nil {
		return nil
	}

	missingSideCar := !istiopolicy2.IsPodPartOfIstioMesh(pod)

	err := p.istioPolicyCreator.UpdateIntentsStatus(ctx, &intents, pod.Spec.ServiceAccountName, missingSideCar)
	if err != nil {
		return err
	}

	if missingSideCar {
		logrus.Infof("Pod %s/%s does not have a sidecar, skipping Istio policy creation", pod.Namespace, pod.Name)
		return nil
	}

	err = p.istioPolicyCreator.Create(ctx, &intents, pod.Namespace, pod.Spec.ServiceAccountName)
	if err != nil {
		logrus.WithError(err).Errorln("Failed creating Istio authorization policy")
		return err
	}

	return nil
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

func (r *PodWatcher) InitIntentsServerIndices(mgr ctrl.Manager) error {
	err := mgr.GetCache().IndexField(
		context.Background(),
		&otterizev1alpha2.ClientIntents{},
		otterizev1alpha2.OtterizeTargetServerIndexField,
		func(object client.Object) []string {
			var res []string
			intents := object.(*otterizev1alpha2.ClientIntents)
			if intents.Spec == nil {
				return nil
			}

			for _, intent := range intents.GetCallsList() {
				res = append(res, intent.Name)
			}

			return res
		})

	if err != nil {
		return err
	}

	return nil
}

func (p *PodWatcher) Register(mgr manager.Manager) error {
	watcher, err := controller.New("otterize-pod-watcher", mgr, controller.Options{
		Reconciler:   p,
		RecoverPanic: lo.ToPtr(true),
	})
	if err != nil {
		return fmt.Errorf("unable to set up pods controller: %p", err)
	}

	if err = watcher.Watch(&source.Kind{Type: &v1.Pod{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return fmt.Errorf("unable to watch Pods: %p", err)
	}

	return nil
}
