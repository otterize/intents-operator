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

type EffectivePoliciesApplier interface {
	ApplyEffectivePolicies(ctx context.Context, eps []ServiceEffectivePolicy) (int, error)
	InjectRecorder(recorder record.EventRecorder)
}

type Syncer struct {
	client.Client
	Scheme   *runtime.Scheme
	appliers []EffectivePoliciesApplier
	injectablerecorder.InjectableRecorder
}

func NewSyncer(k8sClient client.Client, scheme *runtime.Scheme, appliers ...EffectivePoliciesApplier) *Syncer {
	return &Syncer{
		Client:   k8sClient,
		Scheme:   scheme,
		appliers: appliers,
	}
}

func (s *Syncer) AddApplier(applier EffectivePoliciesApplier) {
	s.appliers = append(s.appliers, applier)
}

func (s *Syncer) InjectRecorder(recorder record.EventRecorder) {
	s.Recorder = recorder
	for _, applier := range s.appliers {
		applier.InjectRecorder(recorder)
	}
}

func (s *Syncer) Sync(ctx context.Context) error {
	eps, err := s.getAllServiceEffectivePolicies(ctx)
	if err != nil {
		return errors.Wrap(err)
	}

	errorList := make([]error, 0)
	for _, applier := range s.appliers {
		_, err := applier.ApplyEffectivePolicies(ctx, eps)
		if err != nil {
			errorList = append(errorList, errors.Wrap(err))
		}
	}
	return goerrors.Join(errorList...)
}

func (s *Syncer) getAllServiceEffectivePolicies(ctx context.Context) ([]ServiceEffectivePolicy, error) {
	var intentsList v1alpha3.ClientIntentsList

	err := s.Client.List(ctx, &intentsList)
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
		ep, err := s.buildServiceEffectivePolicy(ctx, service)
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

func (s *Syncer) buildServiceEffectivePolicy(ctx context.Context, service serviceidentity.ServiceIdentity) (ServiceEffectivePolicy, error) {
	relevantClientIntents, err := s.getClientIntentsByServer(ctx, service)
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
			objEventRecorder := injectablerecorder.NewObjectEventRecorder(&s.InjectableRecorder, lo.ToPtr(clientIntent))
			ep.CalledBy = append(ep.CalledBy, ClientCall{Service: clientService, IntendedCall: intendedCall, ObjectEventRecorder: objEventRecorder})
		}
	}
	return ep, nil
}

func (s *Syncer) getClientIntentsByServer(ctx context.Context, server serviceidentity.ServiceIdentity) ([]v1alpha3.ClientIntents, error) {
	var intentsList v1alpha3.ClientIntentsList
	matchFields := client.MatchingFields{v1alpha3.OtterizeFormattedTargetServerIndexField: v1alpha3.GetFormattedOtterizeIdentity(server.Name, server.Namespace)}
	err := s.Client.List(
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
