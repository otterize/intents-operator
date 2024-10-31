package intents_reconcilers

import (
	"context"
	"errors"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetrysender"
	"k8s.io/client-go/tools/record"

	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	linkerdmanager "github.com/otterize/intents-operator/src/operator/controllers/linkerd"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var ErrPodNotFound = errors.New("pod not found")
var ErrNotPartOfMesh = errors.New("not part of mesh")

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

func (r *LinkerdReconciler) InjectRecorder(recorder record.EventRecorder) {}

func (r *LinkerdReconciler) ReconcileEffectivePolicies(ctx context.Context, eps []effectivepolicy.ServiceEffectivePolicy) (int, []error) {
	installed, err := linkerdmanager.IsLinkerdInstalled(ctx, r.Client)
	if err != nil {
		return 0, []error{err}
	}
	if !installed {
		return 0, nil
	}

	errorList := make([]error, 0)
	//linkerdPolicies := goset.NewSet[types.NamespacedName]()
	for _, ep := range eps {
		validResources, created, err := r.applyLinkerdServiceEffectivePolicy(ctx, ep)
		if err != nil {
			errorList = append(errorList, errors.Wrap(err))
			continue
		}
		if created {
			currentPolicies.Add(netpol)
		}
	}

	if currentPolicies.Len() != 0 {
		telemetrysender.SendIntentOperator(telemetriesgql.EventTypeNetworkPoliciesCreated, currentPolicies.Len())
	}
	return currentPolicies.Len(), nil
}

func (r *LinkerdReconciler) applyLinkerdServiceEffectivePolicy(
	ctx context.Context,
	ep effectivepolicy.ServiceEffectivePolicy,
) (bool, error) {
	pods, ok, err := r.serviceIdResolver.ResolveServiceIdentityToPodSlice(ctx, ep.Service)
	if err != nil {
		return false, errors.Wrap(err)
	}

	if !ok {
		return false, errors.Wrap(err)
	}
	pod := pods[0]

	if err != nil {
		if errors.Is(err, serviceidresolver.ErrPodNotFound) {
			r.RecordWarningEventf(
				intents,
				consts.ReasonPodsNotFound,
				"Could not find non-terminating pods for service %s in namespace %s. Intents could not be reconciled now, but will be reconciled if pods appear later.",
				intents.GetWorkloadName(),
				intents.Namespace)
			return ctrl.Result{}, nil
		}
	}

	clientServiceAccountName := pod.Spec.ServiceAccountName
	missingSideCar := !linkerdmanager.IsPodPartOfLinkerdMesh(pod)

	if missingSideCar {
		r.RecordWarningEvent(intents, linkerdmanager.ReasonMissingSidecar, "Client pod missing sidecar, will not create policies")
		logrus.Infof("Pod %s/%s does not have a sidecar, skipping Linkerd resource creation", pod.Namespace, pod.Name)
		return ctrl.Result{}, nil
	}

	validResources, err := r.linkerdManager.CreateResources(ctx, ep, clientServiceAccountName)
	if err != nil {
		if k8serrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
}

func (r *LinkerdReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logrus.Info("Starting linkerd reconcile logic")

	installed, err := linkerdmanager.IsLinkerdInstalled(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !installed {
		logrus.Debug("Server CRD is not installed, Linkerd resource creation is skipped")
		return ctrl.Result{}, nil
	}

	intents := &otterizev2alpha1.ClientIntents{}

	err = r.Get(ctx, req.NamespacedName, intents)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	logrus.Infof("Got intents object with name %s", intents.Name)

	if intents.Spec == nil {
		return ctrl.Result{}, nil
	}

	logrus.Infof("Reconciling Linkerd policies for service %s in namespace %s",
		intents.GetWorkloadName(), req.Namespace)

	if !intents.DeletionTimestamp.IsZero() {
		logrus.Info("initiate delete")
		err = r.linkerdManager.DeleteAll(ctx, intents)

		if err != nil {
			if k8serrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	pod, err := r.serviceIdResolver.ResolveClientIntentToPod(ctx, *intents)

	if err != nil {
		if errors.Is(err, serviceidresolver.ErrPodNotFound) {
			r.RecordWarningEventf(
				intents,
				consts.ReasonPodsNotFound,
				"Could not find non-terminating pods for service %s in namespace %s. Intents could not be reconciled now, but will be reconciled if pods appear later.",
				intents.GetWorkloadName(),
				intents.Namespace)
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	clientServiceAccountName := pod.Spec.ServiceAccountName
	missingSideCar := !linkerdmanager.IsPodPartOfLinkerdMesh(pod)

	if missingSideCar {
		r.RecordWarningEvent(intents, linkerdmanager.ReasonMissingSidecar, "Client pod missing sidecar, will not create policies")
		logrus.Infof("Pod %s/%s does not have a sidecar, skipping Linkerd resource creation", pod.Namespace, pod.Name)
		return ctrl.Result{}, nil
	}

	err = r.linkerdManager.Create(ctx, intents, clientServiceAccountName)
	if err != nil {
		if k8serrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *LinkerdReconciler) handleApplyErrors(err error, errorList []error) []error {
	switch {
	case errors.Is(err, ErrPodNotFound):
	}
}
