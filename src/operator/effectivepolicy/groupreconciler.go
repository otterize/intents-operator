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
	for _, epReconciler := range g.reconcilers {
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
		service := serviceidentity.ServiceIdentity{Name: clientIntent.Spec.Service.Name, Namespace: clientIntent.Namespace}
		services.Add(service)
		serviceToIntent[service] = clientIntent
		for _, intentCall := range clientIntent.GetCallsList() {
			services.Add(serviceidentity.ServiceIdentity{Name: intentCall.GetTargetServerName(), Namespace: intentCall.GetTargetServerNamespace(clientIntent.Namespace)})
		}
	}

	// Build SEP for every service
	epSlice := make([]ServiceEffectivePolicy, 0)
	for _, service := range services.Items() {
		ep, err := g.buildServiceEffectivePolicy(ctx, service)
		if err != nil {
			return nil, err
		}
		if intent, ok := serviceToIntent[service]; ok {
			ep.ClientIntent = lo.ToPtr(intent)
		}
		epSlice = append(epSlice, ep)
	}

	return epSlice, nil
}

func (g *GroupReconciler) buildServiceEffectivePolicy(ctx context.Context, service serviceidentity.ServiceIdentity) (ServiceEffectivePolicy, error) {
	relevantClientIntents, err := g.getClientIntentsByServer(ctx, service)
	if err != nil {
		return ServiceEffectivePolicy{}, errors.Wrap(err)
	}
	ep := ServiceEffectivePolicy{Service: service}
	for _, clientIntent := range relevantClientIntents {
		if !clientIntent.DeletionTimestamp.IsZero() {
			continue
		}
		clientService := serviceidentity.ServiceIdentity{Name: clientIntent.Spec.Service.Name, Namespace: clientIntent.Namespace}
		intendedCalls := getCallsListByServer(service, clientIntent)
		for _, intendedCall := range intendedCalls {
			objEventRecorder := injectablerecorder.NewObjectEventRecorder(&g.InjectableRecorder, lo.ToPtr(clientIntent))
			ep.CalledBy = append(ep.CalledBy, ClientCall{Service: clientService, IntendedCall: intendedCall, ObjectEventRecorder: objEventRecorder})
		}
	}
	return ep, nil
}

func (g *GroupReconciler) getClientIntentsByServer(ctx context.Context, server serviceidentity.ServiceIdentity) ([]v1alpha3.ClientIntents, error) {
	var intentsList v1alpha3.ClientIntentsList
	matchFields := client.MatchingFields{v1alpha3.OtterizeFormattedTargetServerIndexField: v1alpha3.GetFormattedOtterizeIdentity(server.Name, server.Namespace)}
	err := g.Client.List(
		ctx, &intentsList,
		&matchFields,
	)

	if err != nil {
		return nil, err
	}
	return intentsList.Items, nil
}

func getCallsListByServer(server serviceidentity.ServiceIdentity, clientIntent v1alpha3.ClientIntents) []v1alpha3.Intent {
	calls := make([]v1alpha3.Intent, 0)
	for _, intent := range clientIntent.GetCallsList() {
		if intent.GetTargetServerName() == server.Name && intent.GetTargetServerNamespace(clientIntent.Namespace) == server.Namespace {
			calls = append(calls, intent)
		}
	}
	return calls
}
