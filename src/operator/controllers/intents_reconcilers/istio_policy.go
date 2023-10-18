package intents_reconcilers

import (
	"context"
	"errors"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	istiopolicy "github.com/otterize/intents-operator/src/operator/controllers/istiopolicy"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	enforcementDefaultState bool) *IstioPolicyReconciler {
	reconciler := &IstioPolicyReconciler{
		Client:                    c,
		Scheme:                    s,
		RestrictToNamespaces:      restrictToNamespaces,
		enableIstioPolicyCreation: enableIstioPolicyCreation,
		enforcementDefaultState:   enforcementDefaultState,
		serviceIdResolver:         serviceidresolver.NewResolver(c),
	}

	reconciler.policyManager = istiopolicy.NewPolicyManager(c, &reconciler.InjectableRecorder, restrictToNamespaces,
		reconciler.enforcementDefaultState, reconciler.enableIstioPolicyCreation)

	return reconciler
}

func (r *IstioPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	isIstioInstalled, err := istiopolicy.IsIstioAuthorizationPoliciesInstalled(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !isIstioInstalled {
		logrus.Debug("Authorization policies CRD is not installed, Istio policy creation skipped")
		return ctrl.Result{}, nil
	}

	intents := &otterizev1alpha2.ClientIntents{}
	err = r.Get(ctx, req.NamespacedName, intents)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if intents.Spec == nil {
		return ctrl.Result{}, nil
	}

	logrus.Infof("Reconciling Istio authorization policies for service %s in namespace %s",
		intents.Spec.Service.Name, req.Namespace)

	if !intents.DeletionTimestamp.IsZero() {
		err := r.cleanFinalizerAndPolicies(ctx, intents)
		if err != nil {
			if k8serrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			r.RecordWarningEventf(intents, consts.ReasonRemovingIstioPolicyFailed, "Could not remove Istio policies: %s", err.Error())
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(intents, consts.IstioPolicyFinalizerName) {
		logrus.WithField("namespacedName", req.String()).Infof("Adding finalizer %s", consts.IstioPolicyFinalizerName)
		controllerutil.AddFinalizer(intents, consts.IstioPolicyFinalizerName)
		if err := r.Update(ctx, intents); err != nil {
			return ctrl.Result{}, err
		}
	}

	pod, err := r.serviceIdResolver.ResolveClientIntentToPod(ctx, *intents)
	if err != nil {
		if errors.Is(err, serviceidresolver.PodNotFound) {
			r.RecordWarningEventf(
				intents,
				consts.ReasonOtterizeServiceNotFound,
				"Could not find non-terminating pods for service %s in namespace %s",
				intents.Spec.Service.Name,
				intents.Namespace)
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	clientServiceAccountName := pod.Spec.ServiceAccountName
	missingSideCar := !istiopolicy.IsPodPartOfIstioMesh(pod)

	err = r.policyManager.UpdateIntentsStatus(ctx, intents, clientServiceAccountName, missingSideCar)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.updateServerSidecarStatus(ctx, intents)
	if err != nil {
		return ctrl.Result{}, err
	}

	if missingSideCar {
		r.RecordWarningEvent(intents, istiopolicy.ReasonMissingSidecar, "Client pod missing sidecar, will not create policies")
		logrus.Infof("Pod %s/%s does not have a sidecar, skipping Istio policy creation", pod.Namespace, pod.Name)
		return ctrl.Result{}, nil
	}

	err = r.policyManager.Create(ctx, intents, clientServiceAccountName)
	if err != nil {
		if k8serrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *IstioPolicyReconciler) updateServerSidecarStatus(ctx context.Context, intents *otterizev1alpha2.ClientIntents) error {
	for _, intent := range intents.Spec.Calls {
		serverNamespace := intent.GetTargetServerNamespace(intents.Namespace)
		pod, err := r.serviceIdResolver.ResolveIntentServerToPod(ctx, intent, serverNamespace)
		if err != nil {
			if errors.Is(err, serviceidresolver.PodNotFound) {
				continue
			}
			return err
		}

		missingSideCar := !istiopolicy.IsPodPartOfIstioMesh(pod)
		formattedTargetServer := otterizev1alpha2.GetFormattedOtterizeIdentity(intent.GetTargetServerName(), serverNamespace)
		err = r.policyManager.UpdateServerSidecar(ctx, intents, formattedTargetServer, missingSideCar)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *IstioPolicyReconciler) cleanFinalizerAndPolicies(ctx context.Context, intents *otterizev1alpha2.ClientIntents) error {
	if !controllerutil.ContainsFinalizer(intents, consts.IstioPolicyFinalizerName) {
		return nil
	}

	logrus.Infof("Removing Istio policies for deleted intents for service: %s", intents.Spec.Service.Name)

	err := r.policyManager.DeleteAll(ctx, intents)
	if err != nil {
		return err
	}
	RemoveIntentFinalizers(intents, consts.IstioPolicyFinalizerName)
	return r.Update(ctx, intents)
}
