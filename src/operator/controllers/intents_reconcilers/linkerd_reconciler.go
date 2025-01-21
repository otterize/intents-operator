package intents_reconcilers

import (
	"context"
	"fmt"
	"github.com/amit7itz/goset"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	linkerdmanager "github.com/otterize/intents-operator/src/operator/controllers/linkerd"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var ErrNotPartOfMesh = errors.NewSentinelError("not part of mesh")

type LinkerdReconciler struct {
	client.Client
	Scheme                 *runtime.Scheme
	RestrictedToNamespaces []string
	linkerdManager         linkerdmanager.LinkerdPolicyManager
	serviceIdResolver      serviceidresolver.ServiceResolver
	injectablerecorder.InjectableRecorder
}

func NewLinkerdReconciler(c client.Client, s *runtime.Scheme, namespaces []string, enforcementDefaultState bool) *LinkerdReconciler {
	linkerdreconciler := &LinkerdReconciler{
		Client:                 c,
		Scheme:                 s,
		RestrictedToNamespaces: namespaces,
		serviceIdResolver:      serviceidresolver.NewResolver(c),
	}

	linkerdreconciler.linkerdManager = linkerdmanager.NewLinkerdManager(c, namespaces, &linkerdreconciler.InjectableRecorder, enforcementDefaultState)
	return linkerdreconciler
}

func (r *LinkerdReconciler) InjectRecorder(recorder record.EventRecorder) {
	r.Recorder = recorder
}

func (r *LinkerdReconciler) ReconcileEffectivePolicies(ctx context.Context, eps []effectivepolicy.ServiceEffectivePolicy) (int, []error) {
	installed, err := linkerdmanager.IsLinkerdInstalled(ctx, r.Client)
	if err != nil {
		return 0, []error{err}
	}
	if !installed {
		return 0, nil
	}

	errorList := make([]error, 0)
	validResources := linkerdmanager.LinkerdResourceMapping{
		Servers:               goset.NewSet[types.UID](),
		AuthorizationPolicies: goset.NewSet[types.UID](),
		Routes:                goset.NewSet[types.UID](),
	}

	for _, ep := range eps {
		result, err := r.applyLinkerdServiceEffectivePolicy(ctx, ep)
		if err != nil {
			errorList = r.handleApplyErrors(err, errorList)
			continue
		}
		validResources.AuthorizationPolicies.Update(result.AuthorizationPolicies)
		validResources.Servers.Update(result.Servers)
		validResources.Routes.Update(result.Routes)
	}

	if len(errorList) > 0 {
		return 0, errorList
	}

	if err := r.linkerdManager.DeleteOutdatedResources(ctx, eps, validResources); err != nil {
		return 0, []error{err}
	}

	return validResources.AuthorizationPolicies.Len(), nil
}

func (r *LinkerdReconciler) applyLinkerdServiceEffectivePolicy(
	ctx context.Context,
	ep effectivepolicy.ServiceEffectivePolicy,
) (*linkerdmanager.LinkerdResourceMapping, error) {
	pods, ok, err := r.serviceIdResolver.ResolveServiceIdentityToPodSlice(ctx, ep.Service)
	if err != nil {
		if errors.Is(err, serviceidresolver.ErrPodNotFound) {
			r.RecordAgnosticWarningEvent(ep, consts.ReasonPodsNotFound,
				fmt.Sprintf("Could not find non-terminating pods for service %s in namespace %s. Intents could not be reconciled now, but will be reconciled if pods appear later.",
					ep.Service.Name, ep.Service.Namespace))
		}
		return nil, errors.Wrap(err)
	}

	if !ok {
		return nil, errors.Wrap(serviceidresolver.ErrPodNotFound)
	}
	pod := pods[0]

	clientServiceAccountName := pod.Spec.ServiceAccountName
	if !linkerdmanager.IsPodPartOfLinkerdMesh(pod) {
		r.RecordAgnosticWarningEvent(ep, linkerdmanager.ReasonNotPartOfLinkerdMesh,
			fmt.Sprintf("Pod %s in namespace %s is not part of the Linkerd mesh, skipped policy creation", pod.Name, pod.Namespace))

		logrus.Warningf("Pod %s.%s is not part of the Linkerd mesh, skipping policy creation", pod.Name, pod.Namespace)
		return nil, ErrNotPartOfMesh
	}

	validResources, err := r.linkerdManager.CreateResources(ctx, ep, clientServiceAccountName)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	return validResources, nil
}

func (r *LinkerdReconciler) handleApplyErrors(err error, errorList []error) []error {
	// Handle "ok" errors here so we won't retry reconciliation forever
	switch {
	case errors.Is(err, serviceidresolver.ErrPodNotFound):
	case errors.Is(err, ErrNotPartOfMesh):
		return errorList
	}
	return append(errorList, err)
}

func (r *LinkerdReconciler) RecordAgnosticWarningEvent(ep effectivepolicy.ServiceEffectivePolicy, reason, msg string) {
	// Record event on ClientIntents if the service is the Caller, otherwise record on ClientIntents for those who call the service
	if len(ep.Calls) > 0 {
		ep.ClientIntentsEventRecorder.RecordWarningEvent(reason, msg)
	} else {
		ep.RecordOnClientsWarningEventf(reason, msg)
	}
}
