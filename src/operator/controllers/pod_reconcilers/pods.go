package pod_reconcilers

import (
	"context"
	"fmt"
	"github.com/amit7itz/goset"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/istiopolicy"
	"github.com/otterize/intents-operator/src/prometheus"
	"github.com/otterize/intents-operator/src/shared/errors"
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
	OtterizeClientNameIndexField         = "spec.service.name"
	OtterizeClientNameWithKindIndexField = "spec.service.nameWithKind"
)

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;update;patch;list;watch

type PodWatcher struct {
	client.Client
	serviceIdResolver *serviceidresolver.Resolver
	istioPolicyAdmin  istiopolicy.PolicyManager
	injectablerecorder.InjectableRecorder
}

func NewPodWatcher(c client.Client, eventRecorder record.EventRecorder, watchedNamespaces []string, enforcementDefaultState bool, istioEnforcementEnabled bool, activeNamespaces *goset.Set[string]) *PodWatcher {
	recorder := injectablerecorder.InjectableRecorder{Recorder: eventRecorder}
	creator := istiopolicy.NewPolicyManager(c, &recorder, watchedNamespaces, enforcementDefaultState, istioEnforcementEnabled, activeNamespaces)
	return &PodWatcher{
		Client:             c,
		serviceIdResolver:  serviceidresolver.NewResolver(c),
		istioPolicyAdmin:   creator,
		InjectableRecorder: recorder,
	}
}

func (p *PodWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logrus.Debugf("Reconciling due to pod change: %s", req.Name)
	pod := v1.Pod{}
	err := p.Get(ctx, req.NamespacedName, &pod)
	if k8serrors.IsNotFound(err) {
		logrus.Infoln("Pod was deleted")
		return ctrl.Result{}, nil
	}

	if pod.Status.Phase != v1.PodPending && pod.Status.Phase != v1.PodRunning {
		logrus.Debugf("Pod %s is not in a running state, skipping reconciliation", pod.Name)
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	serviceID, err := p.serviceIdResolver.ResolvePodToServiceIdentity(ctx, &pod)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	err = p.addOtterizePodLabels(ctx, req, serviceID, pod)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	err = p.handleIstioPolicy(ctx, pod, serviceID)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	return ctrl.Result{}, nil
}

func (p *PodWatcher) handleIstioPolicy(ctx context.Context, pod v1.Pod, serviceID serviceidentity.ServiceIdentity) error {
	if !p.istioEnforcementEnabled() || pod.DeletionTimestamp != nil {
		return nil
	}

	isIstioInstalled, err := istiopolicy.IsIstioAuthorizationPoliciesInstalled(ctx, p.Client)
	if err != nil {
		return errors.Wrap(err)
	}

	if !isIstioInstalled {
		logrus.Debug("Authorization policies CRD is not installed, Istio policy creation skipped")
		return nil
	}

	err = p.updateServerSideCar(ctx, pod, serviceID)
	if err != nil {
		return errors.Wrap(err)
	}

	intents, err := p.getClientIntentsForServiceIdentity(ctx, serviceID)
	if err != nil {
		logrus.WithFields(logrus.Fields{"ServiceName": serviceID, "Namespace": pod.Namespace}).Errorln("Failed listing intents")
		return errors.Wrap(err)
	}

	if len(intents.Items) == 0 {
		return nil
	}

	for _, clientIntents := range intents.Items {
		err = p.createIstioPolicies(ctx, clientIntents, pod)
		if err != nil {
			return errors.Wrap(err)
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
		return errors.Wrap(err)
	}

	if len(intentsList.Items) == 0 {
		return nil
	}

	for _, clientIntents := range intentsList.Items {
		err = p.istioPolicyAdmin.UpdateServerSidecar(ctx, &clientIntents, serviceID.GetFormattedOtterizeIdentity(), missingSideCar)
		if err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (p *PodWatcher) addOtterizePodLabels(ctx context.Context, req ctrl.Request, serviceID serviceidentity.ServiceIdentity, pod v1.Pod) error {
	if !viper.GetBool(operatorconfig.EnableNetworkPolicyKey) && !viper.GetBool(operatorconfig.EnableIstioPolicyKey) {
		logrus.Debug("Not labeling new pod since network policy creation and Istio policy creation is disabled")
		return nil
	}

	// Intents were deleted and the pod was updated by the operator, skip reconciliation
	_, ok := pod.Annotations[otterizev1alpha3.AllIntentsRemovedAnnotation]
	if ok {
		logrus.Debugf("Skipping reconciliation for pod %s - pod is handled by intents-operator", req.Name)
		return nil
	}

	otterizeServerLabelValue := serviceID.GetFormattedOtterizeIdentityWithoutKind()
	updatedPod := pod.DeepCopy()
	hasUpdates := false

	// Update server label - the server identity of the pod.
	// This is the pod selector used in network policies to grant access to this pod.
	if !otterizev1alpha3.HasOtterizeServiceLabel(&pod, otterizeServerLabelValue) {
		// Label pods as destination servers
		logrus.Debugf("Labeling pod %s with server identity %s", pod.Name, serviceID.Name)
		if updatedPod.Labels == nil {
			updatedPod.Labels = make(map[string]string)
		}
		updatedPod.Labels[otterizev1alpha3.OtterizeServiceLabelKey] = otterizeServerLabelValue
		hasUpdates = true
	}

	if otterizev1alpha3.HasOtterizeDeprecatedServerLabel(&pod) {
		logrus.Debugf("Removing deprecated label for pod %s with server identity %s", pod.Name, serviceID.Name)
		delete(updatedPod.Labels, otterizev1alpha3.OtterizeServerLabelKeyDeprecated)
		hasUpdates = true
	}

	intents, err := p.getClientIntentsForServiceIdentity(ctx, serviceID)
	if err != nil {
		logrus.WithFields(logrus.Fields{"ServiceName": serviceID, "Namespace": pod.Namespace}).Errorln("Failed listing intents")
		return errors.Wrap(err)
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
			logrus.Debugf("Updating Otterize access labels for %s", serviceID.Name)
			updatedPod = otterizev1alpha3.UpdateOtterizeAccessLabels(updatedPod.DeepCopy(), serviceID, otterizeAccessLabels)
			prometheus.IncrementPodsLabeledForNetworkPolicies(1)
			hasUpdates = true
		}
	}

	if hasUpdates {
		err = p.Patch(ctx, updatedPod, client.MergeFrom(&pod))
		if client.IgnoreNotFound(err) != nil {
			return errors.Errorf("failed updating Otterize labels for pod %s in namespace %s: %w", pod.Name, pod.Namespace, err)
		}
	}
	return nil
}

func (p *PodWatcher) getClientIntentsForServiceIdentity(ctx context.Context, serviceID serviceidentity.ServiceIdentity) (otterizev1alpha3.ClientIntentsList, error) {
	var intents otterizev1alpha3.ClientIntentsList

	// first check if there are intents specifically for this service identity (with kind)
	err := p.List(
		ctx, &intents,
		&client.MatchingFields{OtterizeClientNameWithKindIndexField: serviceID.GetNameWithKind()},
		&client.ListOptions{Namespace: serviceID.Namespace})
	if err != nil {
		return otterizev1alpha3.ClientIntentsList{}, errors.Wrap(err)
	}

	// If there are specific intents for this service identity, return them
	if len(intents.Items) != 0 {
		return intents, nil
	}

	// list all intents for this service name (without kind)
	err = p.List(
		ctx, &intents,
		&client.MatchingFields{OtterizeClientNameIndexField: serviceID.Name},
		&client.ListOptions{Namespace: serviceID.Namespace})

	if err != nil {
		return otterizev1alpha3.ClientIntentsList{}, errors.Wrap(err)
	}
	return intents, nil
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
		return errors.Wrap(err)
	}

	if missingSideCar {
		logrus.Debugf("Pod %s/%s does not have a sidecar, skipping Istio policy creation", pod.Namespace, pod.Name)
		return nil
	}

	err = p.istioPolicyAdmin.Create(ctx, &intents, pod.Spec.ServiceAccountName)
	if err != nil {
		logrus.WithError(err).Errorln("Failed creating Istio authorization policy")
		return errors.Wrap(err)
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
		return errors.Wrap(err)
	}

	err = mgr.GetCache().IndexField(
		context.Background(),
		&otterizev1alpha3.ClientIntents{},
		OtterizeClientNameWithKindIndexField,
		func(object client.Object) []string {
			intents := object.(*otterizev1alpha3.ClientIntents)
			serviceIdentity := intents.ToServiceIdentity()
			return []string{serviceIdentity.GetNameWithKind()}
		})
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (p *PodWatcher) Register(mgr manager.Manager) error {
	watcher, err := controller.New("intents-operator", mgr, controller.Options{
		Reconciler:   p,
		RecoverPanic: lo.ToPtr(true),
	})
	if err != nil {
		return errors.Errorf("unable to set up pods controller: %p", err)
	}

	if err = watcher.Watch(source.Kind(mgr.GetCache(), &v1.Pod{}), &handler.EnqueueRequestForObject{}); err != nil {
		return errors.Errorf("unable to watch Pods: %p", err)
	}

	return nil
}
