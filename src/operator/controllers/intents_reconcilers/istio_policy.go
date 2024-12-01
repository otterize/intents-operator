package intents_reconcilers

import (
	"context"
	"github.com/amit7itz/goset"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/otterize/intents-operator/src/operator/controllers/istiopolicy"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type IstioPolicyReconciler struct {
	client.Client
	Scheme                    *runtime.Scheme
	RestrictToNamespaces      []string
	enableIstioPolicyCreation bool
	enforcementDefaultState   bool
	injectablerecorder.InjectableRecorder
	serviceIdResolver serviceidresolver.ServiceResolver
	policyManager     istiopolicy.PolicyManager
}

func NewIstioPolicyReconciler(
	c client.Client,
	s *runtime.Scheme,
	restrictToNamespaces []string,
	enableIstioPolicyCreation bool,
	enforcementDefaultState bool,
	enforcedNamespaces *goset.Set[string],
) *IstioPolicyReconciler {
	reconciler := &IstioPolicyReconciler{
		Client:                    c,
		Scheme:                    s,
		RestrictToNamespaces:      restrictToNamespaces,
		enableIstioPolicyCreation: enableIstioPolicyCreation,
		enforcementDefaultState:   enforcementDefaultState,
		serviceIdResolver:         serviceidresolver.NewResolver(c),
	}

	reconciler.policyManager = istiopolicy.NewPolicyManager(c, &reconciler.InjectableRecorder, restrictToNamespaces,
		reconciler.enforcementDefaultState, reconciler.enableIstioPolicyCreation, enforcedNamespaces)

	return reconciler
}

func (r *IstioPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	isIstioInstalled, err := istiopolicy.IsIstioAuthorizationPoliciesInstalled(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	if !isIstioInstalled {
		logrus.Debug("Authorization policies CRD is not installed, Istio policy creation skipped")
		return ctrl.Result{}, nil
	}

	intents := &otterizev2alpha1.ClientIntents{}
	err = r.Get(ctx, req.NamespacedName, intents)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrap(err)
	}

	if intents.Spec == nil {
		return ctrl.Result{}, nil
	}

	logrus.Debugf("Reconciling Istio authorization policies for service %s in namespace %s",
		intents.Spec.Workload.Name, req.Namespace)

	if !intents.DeletionTimestamp.IsZero() {
		err := r.policyManager.DeleteAll(ctx, intents)
		if err != nil {
			if k8serrors.IsConflict(errors.Unwrap(err)) {
				return ctrl.Result{Requeue: true}, nil
			}
			r.RecordWarningEventf(intents, consts.ReasonRemovingIstioPolicyFailed, "Could not remove Istio policies: %s", err.Error())
			return ctrl.Result{}, errors.Wrap(err)
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
				intents.Spec.Workload.Name,
				intents.Namespace)
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, errors.Wrap(err)
	}

	missingSideCar := !istiopolicy.IsPodPartOfIstioMesh(pod)
	if missingSideCar {
		r.RecordWarningEvent(intents, istiopolicy.ReasonMissingSidecar, "Client pod missing sidecar, will not create policies")
		logrus.Debugf("Pod %s/%s does not have a sidecar, skipping Istio policy creation", pod.Namespace, pod.Name)
		return ctrl.Result{}, nil
	}

	clientServiceAccountName := pod.Spec.ServiceAccountName
	err = r.policyManager.UpdateIntentsStatus(ctx, intents, clientServiceAccountName, missingSideCar)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	err = r.updateServerSidecarStatus(ctx, intents)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	err = r.policyManager.Create(ctx, intents, clientServiceAccountName)
	if err != nil {
		if k8sErr := &(k8serrors.StatusError{}); errors.As(err, &k8sErr) {
			if k8serrors.IsConflict(k8sErr) || k8serrors.IsAlreadyExists(k8sErr) {
				return ctrl.Result{Requeue: true}, nil
			}
		}
		return ctrl.Result{}, errors.Wrap(err)
	}

	err = r.policyManager.RemoveDeprecatedPoliciesForClient(ctx, intents)
	if err != nil {
		logrus.WithError(err).Debugf("Failed to remove deprecated policies for client %s", intents.Spec.Workload.Name)
		return ctrl.Result{}, errors.Wrap(err)
	}

	return ctrl.Result{}, nil
}

func (r *IstioPolicyReconciler) updateServerSidecarStatus(ctx context.Context, intents *otterizev2alpha1.ClientIntents) error {
	for _, intent := range intents.Spec.Targets {
		serviceId := intent.ToServiceIdentity(intents.Namespace)
		pods, ok, err := r.serviceIdResolver.ResolveServiceIdentityToPodSlice(ctx, serviceId)
		if err != nil {
			return errors.Wrap(err)
		}
		if !ok {
			continue
		}
		missingSideCar := false
		for _, pod := range pods {
			missingSideCar = missingSideCar || !istiopolicy.IsPodPartOfIstioMesh(pod)
		}

		err = r.policyManager.UpdateServerSidecar(ctx, intents, serviceId.GetFormattedOtterizeIdentityWithKind(), missingSideCar)
		if err != nil {
			return errors.Wrap(err)
		}

	}
	return nil
}
