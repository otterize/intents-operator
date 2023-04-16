package intents_reconcilers

import (
	"context"
	"github.com/otterize/intents-operator/src/exp/istiopolicy"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	IstioPolicyFinalizerName          = "intents.otterize.com/istio-policy-finalizer"
	ReasonIstioPolicyCreationDisabled = "IstioPolicyCreationDisabled"
	ReasonRemovingIstioPolicyFailed   = "RemovingIstioPolicyFailed"
	ReasonServiceAccountNotFound      = "ServiceAccountNotFound"
)

type IstioPolicyReconciler struct {
	client.Client
	Scheme                     *runtime.Scheme
	RestrictToNamespaces       []string
	enableIstioPolicyCreation  bool
	enforcementEnabledGlobally bool
	injectablerecorder.InjectableRecorder
	serviceIdResolver *serviceidresolver.Resolver
	policyCreator     *istiopolicy.Creator
}

func NewIstioPolicyReconciler(
	c client.Client,
	s *runtime.Scheme,
	restrictToNamespaces []string,
	enableIstioPolicyCreation bool,
	enforcementEnabledGlobally bool) *IstioPolicyReconciler {
	reconciler := &IstioPolicyReconciler{
		Client:                     c,
		Scheme:                     s,
		RestrictToNamespaces:       restrictToNamespaces,
		enableIstioPolicyCreation:  enableIstioPolicyCreation,
		enforcementEnabledGlobally: enforcementEnabledGlobally,
		serviceIdResolver:          serviceidresolver.NewResolver(c),
	}

	reconciler.policyCreator = istiopolicy.NewCreator(c, &reconciler.InjectableRecorder, restrictToNamespaces)

	return reconciler
}

func (r *IstioPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	intents := &otterizev1alpha2.ClientIntents{}
	err := r.Get(ctx, req.NamespacedName, intents)
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
			r.RecordWarningEventf(intents, ReasonRemovingIstioPolicyFailed, "Could not remove Istio policies: %s", err.Error())
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(intents, IstioPolicyFinalizerName) {
		logrus.WithField("namespacedName", req.String()).Infof("Adding finalizer %s", IstioPolicyFinalizerName)
		controllerutil.AddFinalizer(intents, IstioPolicyFinalizerName)
		if err := r.Update(ctx, intents); err != nil {
			return ctrl.Result{}, err
		}
	}

	if !r.enforcementEnabledGlobally {
		r.RecordNormalEvent(intents, ReasonEnforcementGloballyDisabled, "Enforcement is disabled globally, Istio policy creation skipped")
		return ctrl.Result{}, nil
	}

	if !r.enableIstioPolicyCreation {
		r.RecordNormalEvent(intents, ReasonIstioPolicyCreationDisabled, "Istio policy creation is disabled, creation skipped")
		return ctrl.Result{}, nil
	}

	pod, err := r.serviceIdResolver.ResolveClientIntentToPod(ctx, *intents)
	if err != nil {
		if err == serviceidresolver.PodNotFound {
			r.RecordWarningEventf(
				intents,
				ReasonServiceAccountNotFound,
				"Could not find pod for service %s in namespace %s",
				intents.Spec.Service.Name,
				intents.Namespace)
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	clientServiceAccountName := pod.Spec.ServiceAccountName
	missingSideCar := !istiopolicy.IsPodPartOfIstioMesh(pod)

	err = r.policyCreator.UpdateIntentsStatus(ctx, intents, clientServiceAccountName, missingSideCar)
	if err != nil {
		return ctrl.Result{}, err
	}

	if missingSideCar {
		r.RecordWarningEvent(intents, istiopolicy.ReasonMissingSidecar, "Client pod missing sidecar, will not create policies")
		logrus.Infof("Pod %s/%s does not have a sidecar, skipping Istio policy creation", pod.Namespace, pod.Name)
		return ctrl.Result{}, nil
	}

	err = r.updateServerSidecarStatus(ctx, intents)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.policyCreator.Create(ctx, intents, req.Namespace, clientServiceAccountName)
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
		serverNamespace := intent.GetServerNamespace(intents.Namespace)
		pod, err := r.serviceIdResolver.ResolveIntentServerToPod(ctx, intent, serverNamespace)
		if err != nil {
			if err == serviceidresolver.PodNotFound {
				continue
			}
			return err
		}

		missingSideCar := !istiopolicy.IsPodPartOfIstioMesh(pod)
		formattedTargetServer := otterizev1alpha2.GetFormattedOtterizeIdentity(intent.GetServerName(), serverNamespace)
		err = r.policyCreator.UpdateServerSidecar(ctx, intents, formattedTargetServer, missingSideCar)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *IstioPolicyReconciler) cleanFinalizerAndPolicies(ctx context.Context, intents *otterizev1alpha2.ClientIntents) error {
	if !controllerutil.ContainsFinalizer(intents, IstioPolicyFinalizerName) {
		return nil
	}

	logrus.Infof("Removing Istio policies for deleted intents for service: %s", intents.Spec.Service.Name)

	err := r.policyCreator.DeleteAll(ctx, intents)
	if err != nil {
		return err
	}
	controllerutil.RemoveFinalizer(intents, IstioPolicyFinalizerName)
	return r.Update(ctx, intents)
}
