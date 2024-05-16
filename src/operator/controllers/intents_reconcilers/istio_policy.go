package intents_reconcilers

import (
	"context"
	"github.com/amit7itz/goset"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	istiopolicy "github.com/otterize/intents-operator/src/operator/controllers/istiopolicy"
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

	intents := &otterizev1alpha3.ClientIntents{}
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
		intents.Spec.Service.Name, req.Namespace)

	if !intents.DeletionTimestamp.IsZero() {
		err := r.policyManager.DeleteAll(ctx, intents)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err)
		}
		if err != nil {
			if k8serrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			r.RecordWarningEventf(intents, consts.ReasonRemovingIstioPolicyFailed, "Could not remove Istio policies: %s", err.Error())
			return ctrl.Result{}, errors.Wrap(err)
		}
		return ctrl.Result{}, nil
	}

	serviceId := intents.ToServiceIdentity()
	pods, ok, err := otterizev1alpha3.ServiceIdentityToPodList(ctx, r.Client, *serviceId)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}
	if !ok {
		r.RecordWarningEventf(
			intents,
			consts.ReasonPodsNotFound,
			"Could not find non-terminating pods for service %s in namespace %s. Intents could not be reconciled now, but will be reconciled if pods appear later.",
			intents.Spec.Service.Name,
			intents.Namespace)
		return ctrl.Result{}, nil
	}

	var clientServiceAccountName string
	for _, pod := range pods.Items {
		if clientServiceAccountName != "" && clientServiceAccountName != pod.Spec.ServiceAccountName {

		}
		clientServiceAccountName = pod.Spec.ServiceAccountName
		missingSideCar := !istiopolicy.IsPodPartOfIstioMesh(pod)

		err = r.policyManager.UpdateIntentsStatus(ctx, intents, clientServiceAccountName, missingSideCar)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err)
		}

		if missingSideCar {
			r.RecordWarningEvent(intents, istiopolicy.ReasonMissingSidecar, "Client pod missing sidecar, will not create policies")
			logrus.Debugf("Pod %s/%s does not have a sidecar, skipping Istio policy creation", pod.Namespace, pod.Name)
			return ctrl.Result{}, nil
		}
	}

	err = r.updateServerSidecarStatus(ctx, intents)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	err = r.policyManager.Create(ctx, intents, clientServiceAccountName)
	if err != nil {
		if k8serrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, errors.Wrap(err)
	}

	return ctrl.Result{}, nil
}

func (r *IstioPolicyReconciler) updateServerSidecarStatus(ctx context.Context, intents *otterizev1alpha3.ClientIntents) error {
	for _, intent := range intents.Spec.Calls {
		serviceId := intent.ToServiceIdentity(intents.Namespace)
		podList, ok, err := otterizev1alpha3.ServiceIdentityToPodList(ctx, r.Client, *serviceId)
		if err != nil {
			return errors.Wrap(err)
		}
		if !ok {
			continue
		}
		for _, pod := range podList.Items {
			missingSideCar := !istiopolicy.IsPodPartOfIstioMesh(pod)
			formattedTargetServer := serviceId.GetFormattedOtterizeIdentityWithKind()
			err = r.policyManager.UpdateServerSidecar(ctx, intents, formattedTargetServer, missingSideCar)
			if missingSideCar {
				break
			}
			if err != nil {
				return errors.Wrap(err)
			}
		}

	}

	return nil
}
