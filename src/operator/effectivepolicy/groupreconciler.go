package effectivepolicy

import (
	"context"
	goerrors "errors"
	"github.com/amit7itz/goset"
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/access_annotation"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

type reconciler interface {
	ReconcileEffectivePolicies(ctx context.Context, eps []ServiceEffectivePolicy) (int, []error)
	InjectRecorder(recorder record.EventRecorder)
}

type GroupReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	reconcilers       []reconciler
	serviceIdResolver *serviceidresolver.Resolver
	injectablerecorder.InjectableRecorder
}

func NewGroupReconciler(k8sClient client.Client, scheme *runtime.Scheme, serviceIdResolver *serviceidresolver.Resolver, reconcilers ...reconciler) *GroupReconciler {
	return &GroupReconciler{
		Client:            k8sClient,
		Scheme:            scheme,
		serviceIdResolver: serviceIdResolver,
		reconcilers:       reconcilers,
	}
}

func (g *GroupReconciler) AddReconciler(reconciler reconciler) {
	g.reconcilers = append(g.reconcilers, reconciler)
}

func (g *GroupReconciler) InjectRecorder(recorder record.EventRecorder) {
	g.Recorder = recorder
	for _, r := range g.reconcilers {
		r.InjectRecorder(recorder)
	}
}

func (g *GroupReconciler) Reconcile(ctx context.Context) error {
	timeoutCtx, cancel := context.WithTimeoutCause(ctx, 45*time.Second, errors.Errorf("timeout while reconciling service effective policies"))
	defer cancel()
	ctx = timeoutCtx
	eps, err := g.getAllServiceEffectivePolicies(ctx)
	if err != nil {
		return errors.Wrap(err)
	}

	errorList := make([]error, 0)
	logrus.Debugf("Reconciling %d effectivePolicies", len(eps))
	for _, epReconciler := range g.reconcilers {
		logrus.Debugf("Starting cycle for %T", epReconciler)
		_, errs := epReconciler.ReconcileEffectivePolicies(ctx, eps)
		for _, err := range errs {
			errorList = append(errorList, errors.Wrap(err))
		}
	}
	return goerrors.Join(errorList...)
}

func (g *GroupReconciler) getAllServiceEffectivePolicies(ctx context.Context) ([]ServiceEffectivePolicy, error) {
	timeoutCtx, cancel := context.WithTimeoutCause(ctx, 10*time.Second, errors.Errorf("timeout while building list of service effective policies"))
	defer cancel()
	ctx = timeoutCtx
	var intentsList v2alpha1.ClientIntentsList

	err := g.Client.List(ctx, &intentsList)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	serviceToIntent := make(map[serviceidentity.ServiceIdentity]v2alpha1.ClientIntents)
	// Extract all services from intents
	services := goset.NewSet[serviceidentity.ServiceIdentity]()
	for _, clientIntent := range intentsList.Items {
		if !clientIntent.DeletionTimestamp.IsZero() {
			continue
		}
		service := clientIntent.ToServiceIdentity()
		services.Add(service)
		serviceToIntent[service] = clientIntent
		for _, intentCall := range clientIntent.GetTargetList() {
			if !g.shouldCreateEffectivePolicyForIntentTargetServer(intentCall, clientIntent.Namespace) {
				continue
			}
			services.Add(intentCall.ToServiceIdentity(clientIntent.Namespace))
		}
	}

	annotationIntents, err := access_annotation.GetIntentsInCluster(ctx, g.Client, g.serviceIdResolver, &g.InjectableRecorder)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	serversFromAnnotation := lo.Keys(annotationIntents.IntentsByServer)
	clientsFromAnnotation := lo.Keys(annotationIntents.IntentsByClient)

	services.Add(serversFromAnnotation...)
	services.Add(clientsFromAnnotation...)

	// buildNetworkPolicy SEP for every service
	epSlice := make([]ServiceEffectivePolicy, 0)
	for _, service := range services.Items() {
		ep, err := g.buildServiceEffectivePolicy(ctx, service, serviceToIntent, annotationIntents)
		if err != nil {
			return nil, errors.Wrap(err)
		}

		epSlice = append(epSlice, ep)
	}

	return epSlice, nil
}

// shouldCreateEffectivePolicyForIntentTargetServer that checks if we should create a SEP for a given intent target server
func (g *GroupReconciler) shouldCreateEffectivePolicyForIntentTargetServer(intent v2alpha1.Target, clinetIntentNamespace string) bool {
	if intent.IsTargetOutOfCluster() {
		return false
	}
	// We are not treating the kubernetes API server as a service
	if intent.IsTargetTheKubernetesAPIServer(clinetIntentNamespace) {
		return false
	}
	return true
}

func (g *GroupReconciler) buildServiceEffectivePolicy(
	ctx context.Context,
	service serviceidentity.ServiceIdentity,
	serviceToIntent map[serviceidentity.ServiceIdentity]v2alpha1.ClientIntents,
	intentsFromAnnotation access_annotation.AnnotationIntents,
) (ServiceEffectivePolicy, error) {
	relevantClientIntents, err := g.getClientIntentsAsAServer(ctx, service)
	if err != nil {
		return ServiceEffectivePolicy{}, errors.Wrap(err)
	}

	clientsFoundInClientIntents := goset.NewSet[serviceidentity.ServiceIdentity]()
	ep := ServiceEffectivePolicy{Service: service}
	for _, clientIntent := range relevantClientIntents {
		if !clientIntent.DeletionTimestamp.IsZero() || clientIntent.Spec == nil {
			continue
		}
		clientCalls := g.filterAndTransformClientIntentsIntoClientCalls(clientIntent, func(intent v2alpha1.Target) bool {
			if service.Kind == serviceidentity.KindService {
				return intent.IsTargetServerKubernetesService() && intent.GetTargetServerName() == service.Name && intent.GetTargetServerNamespace(clientIntent.Namespace) == service.Namespace
			}
			return !intent.IsTargetServerKubernetesService() && intent.GetTargetServerName() == service.Name && intent.GetTargetServerNamespace(clientIntent.Namespace) == service.Namespace
		})
		clientsFoundInClientIntents.Add(clientIntent.ToServiceIdentity())
		ep.CalledBy = append(ep.CalledBy, clientCalls...)
	}

	annotationsAsServer, ok := intentsFromAnnotation.IntentsByServer[service]
	if ok {
		calledBy := g.getAnnotationIntentsAsServer(service, annotationsAsServer, clientsFoundInClientIntents)
		ep.CalledBy = append(ep.CalledBy, calledBy...)
	}

	serversFoundInClientIntents := goset.NewSet[serviceidentity.ServiceIdentity]()
	// Ignore intents in deletion process
	clientIntents, ok := serviceToIntent[service]
	if ok && clientIntents.DeletionTimestamp.IsZero() && clientIntents.Spec != nil {
		recorder := injectablerecorder.NewObjectEventRecorder(&g.InjectableRecorder, lo.ToPtr(clientIntents))
		// Build Calls and populate ReferencingKubernetesServices to the calls
		calls := make([]Call, 0)
		for _, intent := range clientIntents.GetTargetList() {
			serversFoundInClientIntents.Add(intent.ToServiceIdentity(clientIntents.Namespace))
			call := Call{Target: intent, EventRecorder: recorder}

			call, err = g.populateReferencedKubernetesServices(ctx, call, clientIntents, intent)
			if err != nil {
				return ServiceEffectivePolicy{}, errors.Wrap(err)
			}
			calls = append(calls, call)

		}
		ep.Calls = append(ep.Calls, calls...)
		ep.ClientIntentsEventRecorder = recorder
		ep.ClientIntentsStatus = clientIntents.Status
	}

	annotationsAsClient, ok := intentsFromAnnotation.IntentsByClient[service]
	if ok {
		calls := g.getAnnotationIntentsAsClient(annotationsAsClient, serversFoundInClientIntents)
		ep.Calls = append(ep.Calls, calls...)
	}

	return ep, nil
}

func (g *GroupReconciler) populateReferencedKubernetesServices(ctx context.Context, call Call, clientIntents v2alpha1.ClientIntents, intent v2alpha1.Target) (Call, error) {
	var podList v1.PodList
	if call.GetTargetServerKind() == serviceidentity.KindOtterizeLegacy {
		err := g.Client.List(ctx, &podList, client.MatchingLabels{v2alpha1.OtterizeServiceLabelKey: intent.ToServiceIdentity(clientIntents.Namespace).GetFormattedOtterizeIdentityWithoutKind()})
		if err != nil {
			return Call{}, errors.Wrap(err)
		}
	} else {
		err := g.Client.List(ctx, &podList, client.MatchingLabels{v2alpha1.OtterizeServiceLabelKey: intent.ToServiceIdentity(clientIntents.Namespace).GetFormattedOtterizeIdentityWithoutKind(),
			v2alpha1.OtterizeOwnerKindLabelKey: intent.GetTargetServerKind()})
		if err != nil {
			return Call{}, errors.Wrap(err)
		}

	}

	if len(podList.Items) == 0 {
		return call, nil
	}

	pod := podList.Items[0]

	var serviceList v1.ServiceList
	err := g.Client.List(ctx, &serviceList, client.InNamespace(pod.Namespace))
	if err != nil {
		return Call{}, errors.Wrap(err)
	}

	for _, svc := range serviceList.Items {
		if svc.Spec.Selector != nil && labels.Set(svc.Spec.Selector).AsSelector().Matches(labels.Set(pod.Labels)) {
			call.ReferencingKubernetesServices = append(call.ReferencingKubernetesServices, svc)
		}
	}
	return call, nil
}

func (g *GroupReconciler) getAnnotationIntentsAsClient(annotationsIntents []access_annotation.AnnotationIntent, serversFoundInClientIntents *goset.Set[serviceidentity.ServiceIdentity]) []Call {
	calls := make([]Call, 0)
	for _, annotationIntent := range annotationsIntents {
		if serversFoundInClientIntents.Contains(annotationIntent.Server) {
			// Ignoring annotation in case intent already exists in client intents
			continue
		}

		call := Call{
			Target:        asIntentTarget(annotationIntent),
			EventRecorder: annotationIntent.EventRecorder,
		}
		calls = append(calls, call)
	}
	return calls
}

func asIntentTarget(annotationIntent access_annotation.AnnotationIntent) v2alpha1.Target {
	return v2alpha1.Target{
		Kubernetes: &v2alpha1.KubernetesTarget{
			Name: annotationIntent.Server.GetNameAsServer(),
			Kind: annotationIntent.Server.Kind,
		},
	}
}

func (g *GroupReconciler) getAnnotationIntentsAsServer(service serviceidentity.ServiceIdentity, annotationsIntents []access_annotation.AnnotationIntent, clientsFoundInClientIntents *goset.Set[serviceidentity.ServiceIdentity]) []ClientCall {
	calledBy := make([]ClientCall, 0)
	for _, annotationIntent := range annotationsIntents {
		if clientsFoundInClientIntents.Contains(annotationIntent.Client) {
			// Ignoring annotation in case this client has client intents to this server already
			continue
		}

		call := ClientCall{
			Service:             annotationIntent.Client,
			IntendedCall:        asIntentTarget(annotationIntent),
			ObjectEventRecorder: annotationIntent.EventRecorder,
		}
		calledBy = append(calledBy, call)
	}
	return calledBy
}

func (g *GroupReconciler) filterAndTransformClientIntentsIntoClientCalls(clientIntent v2alpha1.ClientIntents, filter func(intent v2alpha1.Target) bool) []ClientCall {
	clientService := serviceidentity.ServiceIdentity{Name: clientIntent.Spec.Workload.Name, Namespace: clientIntent.Namespace}
	clientCalls := make([]ClientCall, 0)
	for _, intendedCall := range clientIntent.GetTargetList() {
		if !filter(intendedCall) {
			continue
		}
		objEventRecorder := injectablerecorder.NewObjectEventRecorder(&g.InjectableRecorder, lo.ToPtr(clientIntent))
		clientCalls = append(clientCalls, ClientCall{Service: clientService, IntendedCall: intendedCall, ObjectEventRecorder: objEventRecorder})
	}
	return clientCalls
}

func (g *GroupReconciler) getClientIntentsAsAServer(ctx context.Context, server serviceidentity.ServiceIdentity) ([]v2alpha1.ClientIntents, error) {
	var intentsList v2alpha1.ClientIntentsList
	matchFields := client.MatchingFields{v2alpha1.OtterizeFormattedTargetServerIndexField: server.GetFormattedOtterizeIdentityWithKind()}
	err := g.Client.List(
		ctx, &intentsList,
		&matchFields,
	)

	if err != nil {
		return nil, errors.Wrap(err)
	}
	return intentsList.Items, nil
}
