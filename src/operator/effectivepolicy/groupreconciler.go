package effectivepolicy

import (
	"context"
	goerrors "errors"
	"github.com/amit7itz/goset"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type reconciler interface {
	ReconcileEffectivePolicies(ctx context.Context, eps []ServiceEffectivePolicy) (int, error)
	InjectRecorder(recorder record.EventRecorder)
}

type GroupReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	reconcilers []reconciler
	injectablerecorder.InjectableRecorder
}

func NewGroupReconciler(k8sClient client.Client, scheme *runtime.Scheme, reconcilers ...reconciler) *GroupReconciler {
	return &GroupReconciler{
		Client:      k8sClient,
		Scheme:      scheme,
		reconcilers: reconcilers,
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
	eps, err := g.getAllServiceEffectivePolicies(ctx)
	if err != nil {
		return errors.Wrap(err)
	}

	errorList := make([]error, 0)
	logrus.Infof("Reconciling %d effectivePolicies", len(eps))
	for _, epReconciler := range g.reconcilers {
		logrus.Infof("Starting cycle for %T", epReconciler)
		_, err := epReconciler.ReconcileEffectivePolicies(ctx, eps)
		if err != nil {
			errorList = append(errorList, errors.Wrap(err))
		}
	}
	return goerrors.Join(errorList...)
}

func (g *GroupReconciler) getAllServiceEffectivePolicies(ctx context.Context) ([]ServiceEffectivePolicy, error) {
	var intentsList v1alpha3.ClientIntentsList

	err := g.Client.List(ctx, &intentsList)
	if err != nil {
		return nil, err
	}

	serviceToIntent := make(map[serviceidentity.ServiceIdentity]v1alpha3.ClientIntents)
	// Extract all services from intents
	services := goset.NewSet[serviceidentity.ServiceIdentity]()
	for _, clientIntent := range intentsList.Items {
		if !clientIntent.DeletionTimestamp.IsZero() {
			continue
		}
		service := *clientIntent.ToServiceIdentity()
		services.Add(service)
		serviceToIntent[service] = clientIntent
		for _, intentCall := range clientIntent.GetCallsList() {
			if !g.shouldCreateEffectivePolicyForIntentTargetServer(intentCall, clientIntent.Namespace) {
				continue
			}
			services.Add(*intentCall.ToServiceIdentity(clientIntent.Namespace))
		}
	}

	// buildNetworkPolicy SEP for every service
	epSlice := make([]ServiceEffectivePolicy, 0)
	for _, service := range services.Items() {
		ep, err := g.buildServiceEffectivePolicy(ctx, service)
		if err != nil {
			return nil, err
		}
		// Ignore intents in deletion process
		if clientIntents, ok := serviceToIntent[service]; ok && clientIntents.DeletionTimestamp.IsZero() && clientIntents.Spec != nil {
			ep.Calls = append(ep.Calls, clientIntents.GetCallsList()...)
			ep.ClientIntentsEventRecorder = injectablerecorder.NewObjectEventRecorder(&g.InjectableRecorder, lo.ToPtr(clientIntents))
			ep.ClientIntentsStatus = clientIntents.Status
		}
		epSlice = append(epSlice, ep)
	}

	return epSlice, nil
}

// shouldCreateEffectivePolicyForIntentTargetServer that checks if we should create a SEP for a given intent target server
func (g *GroupReconciler) shouldCreateEffectivePolicyForIntentTargetServer(intent v1alpha3.Intent, clinetIntentNamespace string) bool {
	if intent.IsTargetOutOfCluster() {
		return false
	}
	// We are not treating the kubernetes API server as a service
	if intent.IsTargetTheKubernetesAPIServer(clinetIntentNamespace) {
		return false
	}
	return true
}

func (g *GroupReconciler) buildServiceEffectivePolicy(ctx context.Context, service serviceidentity.ServiceIdentity) (ServiceEffectivePolicy, error) {
	relevantClientIntents, err := g.getClientIntentsByServer(ctx, service)
	if err != nil {
		return ServiceEffectivePolicy{}, errors.Wrap(err)
	}
	ep := ServiceEffectivePolicy{Service: service}
	for _, clientIntent := range relevantClientIntents {
		if !clientIntent.DeletionTimestamp.IsZero() || clientIntent.Spec == nil {
			continue
		}
		clientCalls := g.filterAndTransformClientIntentsIntoClientCalls(clientIntent, func(intent v1alpha3.Intent) bool {
			if service.Kind == serviceidentity.KindService {
				return intent.IsTargetServerKubernetesService() && intent.GetTargetServerName() == service.Name && intent.GetTargetServerNamespace(clientIntent.Namespace) == service.Namespace
			}
			return !intent.IsTargetServerKubernetesService() && intent.GetTargetServerName() == service.Name && intent.GetTargetServerNamespace(clientIntent.Namespace) == service.Namespace
		})
		ep.CalledBy = append(ep.CalledBy, clientCalls...)
	}
	return ep, nil
}

func (g *GroupReconciler) filterAndTransformClientIntentsIntoClientCalls(clientIntent v1alpha3.ClientIntents, filter func(intent v1alpha3.Intent) bool) []ClientCall {
	clientService := serviceidentity.ServiceIdentity{Name: clientIntent.Spec.Service.Name, Namespace: clientIntent.Namespace}
	clientCalls := make([]ClientCall, 0)
	for _, intendedCall := range clientIntent.GetCallsList() {
		if !filter(intendedCall) {
			continue
		}
		objEventRecorder := injectablerecorder.NewObjectEventRecorder(&g.InjectableRecorder, lo.ToPtr(clientIntent))
		clientCalls = append(clientCalls, ClientCall{Service: clientService, IntendedCall: intendedCall, ObjectEventRecorder: objEventRecorder})
	}
	return clientCalls
}

func (g *GroupReconciler) getClientIntentsByServer(ctx context.Context, server serviceidentity.ServiceIdentity) ([]v1alpha3.ClientIntents, error) {
	var intentsList v1alpha3.ClientIntentsList
	matchFields := client.MatchingFields{v1alpha3.OtterizeFormattedTargetServerIndexField: server.GetFormattedOtterizeIdentity()}
	err := g.Client.List(
		ctx, &intentsList,
		&matchFields,
	)

	if err != nil {
		return nil, err
	}
	return intentsList.Items, nil
}
