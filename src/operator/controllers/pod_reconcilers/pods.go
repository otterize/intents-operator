package pod_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/istiopolicy"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
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

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;update;patch;list;watch

type PodWatcher struct {
	client.Client
	serviceIdResolver *serviceidresolver.Resolver
	istioPolicyAdmin  istiopolicy.PolicyManager
	injectablerecorder.InjectableRecorder
}

func NewPodWatcher(c client.Client, eventRecorder record.EventRecorder, watchedNamespaces []string, enforcementDefaultState bool, istioEnforcementEnabled bool) *PodWatcher {
	recorder := injectablerecorder.InjectableRecorder{Recorder: eventRecorder}
	creator := istiopolicy.NewPolicyManager(c, &recorder, watchedNamespaces, enforcementDefaultState, istioEnforcementEnabled)
	return &PodWatcher{
		Client:             c,
		serviceIdResolver:  serviceidresolver.NewResolver(c),
		istioPolicyAdmin:   creator,
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

	// If a new pod starts, check if we need to do something for it.
	var intents otterizev1alpha3.ClientIntentsList
	err = p.List(
		ctx,
		&intents,
		&client.MatchingFields{OtterizeClientNameIndexField: serviceID.Name},
		&client.ListOptions{Namespace: pod.Namespace})
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

func (p *PodWatcher) handleIstioPolicy(ctx context.Context, pod v1.Pod, serviceID serviceidentity.ServiceIdentity) error {
	if !p.istioEnforcementEnabled() || pod.DeletionTimestamp != nil {
		return nil
	}

	isIstioInstalled, err := istiopolicy.IsIstioAuthorizationPoliciesInstalled(ctx, p.Client)
	if err != nil {
		return err
	}

	if !isIstioInstalled {
		logrus.Debug("Authorization policies CRD is not installed, Istio policy creation skipped")
		return nil
	}

	err = p.updateServerSideCar(ctx, pod, serviceID)
	if err != nil {
		return err
	}

	var intents otterizev1alpha3.ClientIntentsList
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

func (p *PodWatcher) updateServerSideCar(ctx context.Context, pod v1.Pod, serviceID serviceidentity.ServiceIdentity) error {
	missingSideCar := !istiopolicy.IsPodPartOfIstioMesh(pod)

	serviceFullName := fmt.Sprintf("%s.%s", serviceID.Name, pod.Namespace)
	var intentsList otterizev1alpha3.ClientIntentsList
	err := p.List(
		ctx, &intentsList,
		&client.MatchingFields{otterizev1alpha3.OtterizeTargetServerIndexField: serviceFullName})
	if err != nil {
		return err
	}

	if len(intentsList.Items) == 0 {
		return nil
	}

	for _, clientIntents := range intentsList.Items {
		formattedTargetServer := otterizev1alpha3.GetFormattedOtterizeIdentity(serviceID.Name, pod.Namespace)
		err = p.istioPolicyAdmin.UpdateServerSidecar(ctx, &clientIntents, formattedTargetServer, missingSideCar)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *PodWatcher) addOtterizePodLabels(ctx context.Context, req ctrl.Request, serviceID serviceidentity.ServiceIdentity, pod v1.Pod) error {
	// Intents were deleted and the pod was updated by the operator, skip reconciliation
	_, ok := pod.Annotations[otterizev1alpha3.AllIntentsRemovedAnnotation]
	if ok {
		logrus.Infof("Skipping reconciliation for pod %s - pod is handled by intents-operator", req.Name)
		return nil
	}

	otterizeServerLabelValue := otterizev1alpha3.GetFormattedOtterizeIdentity(serviceID.Name, pod.Namespace)
	updatedPod := pod.DeepCopy()
	hasUpdates := false

	// Update server label - the server identity of the pod.
	// This is the pod selector used in network policies to grant access to this pod.
	if !otterizev1alpha3.HasOtterizeServerLabel(&pod, otterizeServerLabelValue) {
		// Label pods as destination servers
		logrus.Infof("Labeling pod %s with server identity %s", pod.Name, serviceID.Name)
		if updatedPod.Labels == nil {
			updatedPod.Labels = make(map[string]string)
		}
		updatedPod.Labels[otterizev1alpha3.OtterizeServerLabelKey] = otterizeServerLabelValue
		hasUpdates = true
	}

	var intents otterizev1alpha3.ClientIntentsList
	err := p.List(
		ctx, &intents,
		&client.MatchingFields{OtterizeClientNameIndexField: serviceID.Name},
		&client.ListOptions{Namespace: pod.Namespace})

	if err != nil {
		logrus.WithFields(logrus.Fields{"ServiceName": serviceID, "Namespace": pod.Namespace}).Errorln("Failed listing intents")
		return err
	}

	if len(intents.Items) != 0 {
		// Update access labels - which servers the client can access (current intents), and remove old access labels (deleted intents)
		otterizeAccessLabels := make(map[string]string)
		for _, intent := range intents.Items {
			currIntentLabels := intent.GetIntentsLabelMapping(pod.Namespace)
			for k, v := range currIntentLabels {
				otterizeAccessLabels[k] = v
			}
		}
		if otterizev1alpha3.IsMissingOtterizeAccessLabels(&pod, otterizeAccessLabels) {
			logrus.Infof("Updating Otterize access labels for %s", serviceID.Name)
			updatedPod = otterizev1alpha3.UpdateOtterizeAccessLabels(updatedPod.DeepCopy(), otterizeAccessLabels)
			hasUpdates = true
		}
	}

	if hasUpdates {
		err = p.Patch(ctx, updatedPod, client.MergeFrom(&pod))
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

func (p *PodWatcher) createIstioPolicies(ctx context.Context, intents otterizev1alpha3.ClientIntents, pod v1.Pod) error {
	if intents.DeletionTimestamp != nil {
		return nil
	}

	missingSideCar := !istiopolicy.IsPodPartOfIstioMesh(pod)

	err := p.istioPolicyAdmin.UpdateIntentsStatus(ctx, &intents, pod.Spec.ServiceAccountName, missingSideCar)
	if err != nil {
		return err
	}

	if missingSideCar {
		logrus.Infof("Pod %s/%s does not have a sidecar, skipping Istio policy creation", pod.Namespace, pod.Name)
		return nil
	}

	err = p.istioPolicyAdmin.Create(ctx, &intents, pod.Spec.ServiceAccountName)
	if err != nil {
		logrus.WithError(err).Errorln("Failed creating Istio authorization policy")
		return err
	}

	return nil
}

func (p *PodWatcher) InitIntentsClientIndices(mgr manager.Manager) error {
	err := mgr.GetCache().IndexField(
		context.Background(),
		&otterizev1alpha3.ClientIntents{},
		OtterizeClientNameIndexField,
		func(object client.Object) []string {
			intents := object.(*otterizev1alpha3.ClientIntents)
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
	watcher, err := controller.New("intents-operator", mgr, controller.Options{
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
