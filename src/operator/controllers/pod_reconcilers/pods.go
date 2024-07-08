package pod_reconcilers

import (
	"context"
	"fmt"
	"github.com/amit7itz/goset"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/access_annotation"
	"github.com/otterize/intents-operator/src/operator/controllers/istiopolicy"
	"github.com/otterize/intents-operator/src/prometheus"
	"github.com/otterize/intents-operator/src/shared/databaseconfigurator"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	OtterizeClientNameIndexField         = "spec.service.name"
	OtterizeClientNameWithKindIndexField = "spec.service.nameWithKind"
	FailedParsingAnnotationEvent         = "FailedParsingAccessAnnotation"
)

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;update;patch;list;watch

type PodWatcher struct {
	client.Client
	serviceIdResolver *serviceidresolver.Resolver
	istioPolicyAdmin  istiopolicy.PolicyManager
	injectablerecorder.InjectableRecorder
	intentsReconciler reconcile.Reconciler
	epReconciler      GroupReconciler
}

type GroupReconciler interface {
	Reconcile(ctx context.Context) error
}

func NewPodWatcher(c client.Client, eventRecorder record.EventRecorder, watchedNamespaces []string, enforcementDefaultState bool, istioEnforcementEnabled bool, activeNamespaces *goset.Set[string], intentsReconciler reconcile.Reconciler, serviceEffectivePolicyReconciler GroupReconciler) *PodWatcher {
	recorder := injectablerecorder.InjectableRecorder{Recorder: eventRecorder}
	creator := istiopolicy.NewPolicyManager(c, &recorder, watchedNamespaces, enforcementDefaultState, istioEnforcementEnabled, activeNamespaces)
	return &PodWatcher{
		Client:             c,
		serviceIdResolver:  serviceidresolver.NewResolver(c),
		istioPolicyAdmin:   creator,
		InjectableRecorder: recorder,
		intentsReconciler:  intentsReconciler,
		epReconciler:       serviceEffectivePolicyReconciler,
	}
}

func (p *PodWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logrus.Debugf("Reconciling due to pod change: %s", req.Name)
	pod := v1.Pod{}
	err := p.Get(ctx, req.NamespacedName, &pod)

	if k8serrors.IsNotFound(err) {
		logrus.Debugf("Pod was deleted")
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
		if k8serrors.IsAlreadyExists(err) {
			// Can happen if the Istio policy is created in parallel by another controller
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, errors.Wrap(err)
	}

	res, err := p.handleDatabaseIntents(ctx, pod, serviceID)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	err = p.runServiceEffectivePolicy(ctx, pod, serviceID)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	if res.Requeue {
		return res, nil
	}

	return ctrl.Result{}, nil
}

func (p *PodWatcher) runServiceEffectivePolicy(ctx context.Context, pod v1.Pod, serviceID serviceidentity.ServiceIdentity) error {
	// Run even if the pod is being deleted to remove intents if needed
	_, ok, err := access_annotation.ParseAccessAnnotations(&pod)
	if err != nil {
		return errors.Wrap(err)
	}
	if !ok {
		return nil
	}

	err = p.epReconciler.Reconcile(ctx)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
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

	if len(intents) == 0 {
		return nil
	}

	for _, clientIntents := range intents {
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
		err = p.istioPolicyAdmin.UpdateServerSidecar(ctx, &clientIntents, serviceID.GetFormattedOtterizeIdentityWithoutKind(), missingSideCar)
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

	if !otterizev1alpha3.HasOtterizeOwnerKindLabel(&pod, serviceID.Kind) {
		logrus.Debugf("Labeling pod %s with owner kind %s", pod.Name, serviceID.Kind)
		updatedPod.Labels[otterizev1alpha3.OtterizeOwnerKindLabelKey] = serviceID.Kind
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

	if len(intents) != 0 {
		// Update access labels - which servers the client can access (current intents), and remove old access labels (deleted intents)
		otterizeAccessLabels := make(map[string]string)
		for _, intent := range intents {
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

func (p *PodWatcher) getClientIntentsForServiceIdentity(ctx context.Context, serviceID serviceidentity.ServiceIdentity) ([]otterizev1alpha3.ClientIntents, error) {
	var intents otterizev1alpha3.ClientIntentsList

	// first check if there are intents specifically for this service identity (with kind)
	err := p.List(
		ctx, &intents,
		&client.MatchingFields{OtterizeClientNameWithKindIndexField: serviceID.GetNameWithKind()},
		&client.ListOptions{Namespace: serviceID.Namespace})
	if err != nil {
		return []otterizev1alpha3.ClientIntents{}, errors.Wrap(err)
	}

	clientIntentsWithKind := intents.Items

	intentsFromAnnotation, err := p.getIntentsFromAccessAnnotation(ctx, serviceID)
	if err != nil {
		return []otterizev1alpha3.ClientIntents{}, errors.Wrap(err)
	}

	// list all intents for this service name (without kind)
	err = p.List(
		ctx, &intents,
		&client.MatchingFields{OtterizeClientNameIndexField: serviceID.Name},
		&client.ListOptions{Namespace: serviceID.Namespace})
	if err != nil {
		return []otterizev1alpha3.ClientIntents{}, errors.Wrap(err)
	}

	clientIntentsWithoutKind := intents.Items

	// For backwards compatibility if the user defined intents for the service without kind we ignore annotation intents
	if len(clientIntentsWithKind) == 0 && len(clientIntentsWithoutKind) > 0 {
		return clientIntentsWithoutKind, nil
	}

	return appendCalls(serviceID, clientIntentsWithoutKind, intentsFromAnnotation), nil
}

func (p *PodWatcher) getIntentsFromAccessAnnotation(ctx context.Context, serviceID serviceidentity.ServiceIdentity) ([]otterizev1alpha3.Intent, error) {
	serversPods, err := p.getServersFromAnnotationsCalledByTheService(ctx, serviceID)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	intents := make([]otterizev1alpha3.Intent, 0)
	for _, serverPod := range serversPods.Items {
		serverIdentity, err := p.serviceIdResolver.ResolvePodToServiceIdentity(ctx, &serverPod)
		if err != nil {
			return nil, errors.Wrap(err)
		}

		intents = append(intents, otterizev1alpha3.Intent{
			Name: serverIdentity.GetNameAsServer(),
			Kind: serverIdentity.Kind,
		})
	}
	return intents, nil
}

func (p *PodWatcher) getServersFromAnnotationsCalledByTheService(ctx context.Context, serviceID serviceidentity.ServiceIdentity) (v1.PodList, error) {
	var serversPods v1.PodList
	err := p.List(
		ctx,
		&serversPods,
		&client.MatchingFields{otterizev1alpha3.OtterizeClientOnAccessAnnotationIndexField: serviceID.GetFormattedOtterizeIdentityWithKind()})
	if err != nil {
		return v1.PodList{}, errors.Wrap(err)
	}
	return serversPods, nil
}

func appendCalls(client serviceidentity.ServiceIdentity, intentsFromCRD []otterizev1alpha3.ClientIntents, intentsFromAnnotation []otterizev1alpha3.Intent) []otterizev1alpha3.ClientIntents {
	if len(intentsFromCRD) == 0 {
		clientIntent := otterizev1alpha3.ClientIntents{
			ObjectMeta: metav1.ObjectMeta{
				Name:      client.Name,
				Namespace: client.Namespace,
			},
			Spec: &otterizev1alpha3.IntentsSpec{
				Service: otterizev1alpha3.Service{
					Name: client.Name,
					Kind: client.Kind,
				},
				Calls: intentsFromAnnotation,
			},
		}
		return []otterizev1alpha3.ClientIntents{clientIntent}
	}
	for _, clientIntent := range intentsFromCRD {
		clientIntent.Spec.Calls = append(clientIntent.Spec.Calls, intentsFromAnnotation...)
	}
	return intentsFromCRD
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

	err = mgr.GetCache().IndexField(
		context.Background(),
		&v1.Pod{},
		otterizev1alpha3.OtterizeClientOnAccessAnnotationIndexField,
		func(object client.Object) []string {
			pod := object.(*v1.Pod)
			if pod.DeletionTimestamp != nil {
				return []string{}
			}
			clients, ok, err := access_annotation.ParseAccessAnnotations(pod)
			if err != nil {
				logrus.WithError(err).Error("Failed to parse access annotation")
				mgr.GetEventRecorderFor("intents-operator").Eventf(pod, "Warning", FailedParsingAnnotationEvent, annotationParsingErr(err))
				return []string{}
			}
			if !ok {
				return []string{}
			}

			clientIdentities := lo.Map(clients, func(serviceIdentity serviceidentity.ServiceIdentity, _ int) string {
				return serviceIdentity.GetFormattedOtterizeIdentityWithKind()
			})
			return clientIdentities
		})
	if err != nil {
		return errors.Wrap(err)
	}

	err = mgr.GetCache().IndexField(
		context.Background(),
		&v1.Pod{},
		otterizev1alpha3.OtterizeServerHasAnyCalledByAnnotationIndexField,
		func(object client.Object) []string {
			pod := object.(*v1.Pod)

			if pod.DeletionTimestamp != nil {
				return []string{}
			}
			_, ok, err := access_annotation.ParseAccessAnnotations(pod)
			if err != nil {
				logrus.WithError(err).Error("Failed to parse access annotation")
				mgr.GetEventRecorderFor("intents-operator").Eventf(pod, "Warning", FailedParsingAnnotationEvent, annotationParsingErr(err))
				return []string{}
			}
			if !ok {
				return []string{}
			}
			return []string{otterizev1alpha3.OtterizeServerHasAnyCalledByAnnotationValue}
		})
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func annotationParsingErr(err error) string {
	return fmt.Sprintf("failed to parse access annotation: %s", err.Error())
}

func (p *PodWatcher) Register(mgr manager.Manager) error {
	watcher, err := controller.New("intents-operator", mgr, controller.Options{
		Reconciler:   p,
		RecoverPanic: lo.ToPtr(true),
	})
	if err != nil {
		return errors.Errorf("unable to set up pods controller: %p", err)
	}

	err = watcher.Watch(source.Kind(mgr.GetCache(), &v1.Pod{}), handler.EnqueueRequestsFromMapFunc(p.PodsToRequests))
	if err != nil {
		return errors.Errorf("unable to watch Pods: %p", err)
	}

	return nil
}

func (p *PodWatcher) PodsToRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	pod := obj.(*v1.Pod)

	currentPod := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: pod.Namespace,
			Name:      pod.Name,
		},
	}
	requests := []reconcile.Request{currentPod}

	// Explicitly generate requests for clients of this pod even if it's during deletion, Since those pods
	// should get reconciled, but now the server access intents would be considered as deleted
	clients, ok, err := access_annotation.ParseAccessAnnotations(pod)
	if err != nil {
		p.RecordAnnotationParsingErr(pod, err)
		return requests
	}
	if ok {
		clientRequests := p.serviceIdentitiesToPodRequests(ctx, clients)
		requests = append(requests, clientRequests...)
	}
	return requests
}

func (p *PodWatcher) serviceIdentitiesToPodRequests(ctx context.Context, clients []serviceidentity.ServiceIdentity) []reconcile.Request {
	requests := make([]reconcile.Request, 0)
	for _, clientIdentity := range clients {
		clientPods, podsFound, err := p.serviceIdResolver.ResolveServiceIdentityToPodSlice(ctx, clientIdentity)
		if err != nil {
			if errors.Is(otterizev1alpha3.ServiceHasNoSelector, err) {
				continue
			}

			logrus.WithError(err).Error("Failed to resolve annotation client")
			continue
		}
		if !podsFound {
			continue
		}
		clientsRequests := lo.Map(clientPods, func(clientPod v1.Pod, _ int) reconcile.Request {
			return reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: clientPod.Namespace,
					Name:      clientPod.Name,
				},
			}
		})

		requests = append(requests, clientsRequests...)
	}
	return requests
}

func (p *PodWatcher) RecordAnnotationParsingErr(pod *v1.Pod, err error) {
	p.RecordWarningEvent(pod, FailedParsingAnnotationEvent, fmt.Sprintf("Failed to parse access annotation: %s", err.Error()))
}

func (p *PodWatcher) handleDatabaseIntents(ctx context.Context, pod v1.Pod, serviceID serviceidentity.ServiceIdentity) (ctrl.Result, error) {
	if pod.Annotations == nil {
		return ctrl.Result{}, nil
	}

	if _, ok := pod.Annotations[databaseconfigurator.DatabaseAccessAnnotation]; ok {
		// Has database access annotation, no need to do anything
		return ctrl.Result{}, nil
	}

	clientIntents, err := p.getClientIntentsForServiceIdentity(ctx, serviceID)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	dbIntents := lo.Filter(clientIntents, func(clientIntents otterizev1alpha3.ClientIntents, _ int) bool {
		return len(clientIntents.GetDatabaseIntents()) > 0
	})
	for _, clientIntents := range dbIntents {
		res, err := p.intentsReconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: clientIntents.Namespace,
				Name:      clientIntents.Name,
			},
		})
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err)
		}
		if res.Requeue {
			return res, nil
		}
	}

	return ctrl.Result{}, nil
}
