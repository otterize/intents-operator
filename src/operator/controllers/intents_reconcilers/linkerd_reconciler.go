package intents_reconcilers

import (
	"context"
	"errors"

	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
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

type LinkerdReconciler struct {
	client.Client
	Scheme                 *runtime.Scheme
	RestrictedToNamespaces []string
	linkerdManager         *linkerdmanager.LinkerdManager
	serviceIdResolver      serviceidresolver.ServiceResolver
	injectablerecorder.InjectableRecorder
}

func NewLinkerdReconciler(c client.Client,
	s *runtime.Scheme,
	namespaces []string,
	enforcementDefaultState,
	enableLinkerdPolicyCreation bool,
) *LinkerdReconciler {
	linkerdreconciler := &LinkerdReconciler{
		Client:                 c,
		Scheme:                 s,
		RestrictedToNamespaces: namespaces,
		serviceIdResolver:      serviceidresolver.NewResolver(c),
	}

	linkerdreconciler.linkerdManager = linkerdmanager.NewLinkerdManager(c, namespaces, enforcementDefaultState, enableLinkerdPolicyCreation)
	return linkerdreconciler
}

func (r *LinkerdReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	installed, err := linkerdmanager.IsLinkerdServerInstalled(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !installed {
		logrus.Debug("Server CRD is not installed, Linkerd resource creation is skipped")
		return ctrl.Result{}, nil
	}

	intents := &otterizev1alpha3.ClientIntents{}
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
		// replace with a manager for linkerd policy

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
				intents.Spec.Service.Name,
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
