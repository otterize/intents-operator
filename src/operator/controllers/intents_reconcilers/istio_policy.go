package intents_reconcilers

import (
	"context"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/istiopolicy"
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
			r.RecordWarningEventf(intents, ReasonRemovingIstioPolicyFailed, "could not remove istio policies: %s", err.Error())
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
		r.RecordNormalEvent(intents, ReasonEnforcementGloballyDisabled, "Enforcement is disabled globally, istio policy creation skipped")
		return ctrl.Result{}, nil
	}

	if !r.enableIstioPolicyCreation {
		r.RecordNormalEvent(intents, ReasonIstioPolicyCreationDisabled, "Istio policy creation is disabled, creation skipped")
		return ctrl.Result{}, nil
	}

	clientServiceAccountName, err := r.serviceIdResolver.ResolveClientIntentToServiceAccountName(ctx, *intents)
	if err != nil {
		if err == serviceidresolver.ServiceAccountNotFond {
			r.RecordWarningEventf(
				intents,
				ReasonServiceAccountNotFound,
				"could not find service account for service %s in namespace %s",
				intents.Spec.Service.Name,
				intents.Namespace)
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	err = r.policyCreator.UpdateIntentsStatus(ctx, intents, clientServiceAccountName)
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
