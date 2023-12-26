package effectivepolicy

import (
	"context"
	"github.com/amit7itz/goset"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/samber/lo"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ClientCall struct {
	Service             serviceidentity.ServiceIdentity
	IntendedCall        v1alpha3.Intent
	ObjectEventRecorder *injectablerecorder.ObjectEventRecorder
}

type ServiceEffectivePolicy struct {
	Service  serviceidentity.ServiceIdentity
	CalledBy []ClientCall
}

func GetServiceEffectivePoliciesAffectedByIntent(ctx context.Context, k8sClient client.Client, intents *v1alpha3.ClientIntents, eventRecorder *injectablerecorder.InjectableRecorder) ([]ServiceEffectivePolicy, error) {
	epSlice := make([]ServiceEffectivePolicy, 0)
	for _, intentCall := range intents.GetCallsList() {
		serverService := serviceidentity.ServiceIdentity{Name: intentCall.GetTargetServerName(), Namespace: intentCall.GetTargetServerNamespace(intents.Namespace)}
		ep, err := BuildServiceEffectivePolicy(ctx, k8sClient, serverService, eventRecorder)
		if err != nil {
			return nil, err
		}
		epSlice = append(epSlice, ep)
	}
	return epSlice, nil
}

func GetAllServiceEffectivePolicies(ctx context.Context, k8sClient client.Client, eventRecorder *injectablerecorder.InjectableRecorder) ([]ServiceEffectivePolicy, error) {
	var intentsList v1alpha3.ClientIntentsList

	err := k8sClient.List(ctx, &intentsList)
	if err != nil {
		return nil, err
	}

	// Extract all services from intents
	services := goset.NewSet[serviceidentity.ServiceIdentity]()
	for _, clientIntent := range intentsList.Items {
		services.Add(serviceidentity.ServiceIdentity{Name: clientIntent.Spec.Service.Name, Namespace: clientIntent.Namespace})
		for _, intentCall := range clientIntent.GetCallsList() {
			services.Add(serviceidentity.ServiceIdentity{Name: intentCall.GetTargetServerName(), Namespace: intentCall.GetTargetServerNamespace(clientIntent.Namespace)})
		}
	}

	// Build SEP for every service
	epSlice := make([]ServiceEffectivePolicy, 0)
	for _, service := range services.Items() {
		ep, err := BuildServiceEffectivePolicy(ctx, k8sClient, service, eventRecorder)
		if err != nil {
			return nil, err
		}
		epSlice = append(epSlice, ep)
	}

	return epSlice, nil
}

func BuildServiceEffectivePolicy(ctx context.Context, k8sClient client.Client, service serviceidentity.ServiceIdentity, eventRecorder *injectablerecorder.InjectableRecorder) (ServiceEffectivePolicy, error) {
	relevantClientIntents, err := getClientIntentsByServer(ctx, k8sClient, service)
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
			objEventRecorder := injectablerecorder.NewObjectEventRecorder(eventRecorder, lo.ToPtr(clientIntent))
			ep.CalledBy = append(ep.CalledBy, ClientCall{Service: clientService, IntendedCall: intendedCall, ObjectEventRecorder: objEventRecorder})
		}
	}

	return ep, nil
}

func getClientIntentsByServer(ctx context.Context, k8sClient client.Client, server serviceidentity.ServiceIdentity) ([]v1alpha3.ClientIntents, error) {
	var intentsList v1alpha3.ClientIntentsList
	matchFields := client.MatchingFields{v1alpha3.OtterizeFormattedTargetServerIndexField: v1alpha3.GetFormattedOtterizeIdentity(server.Name, server.Namespace)}
	err := k8sClient.List(
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
