package intents_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	v1beta12 "istio.io/api/security/v1beta1"
	v1beta13 "istio.io/api/type/v1beta1"
	"istio.io/client-go/pkg/apis/security/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	IstioPolicyFinalizerName          = "intents.otterize.com/istio-policy-finalizer"
	ReasonIstioPolicyCreationDisabled = "IstioPolicyCreationDisabled"
	ReasonGettingIstioPolicyFailed    = "GettingIstioPolicyFailed"
	ReasonRemovingIstioPolicyFailed   = "RemovingIstioPolicyFailed"
	ReasonCreatingIstioPolicyFailed   = "CreatingIstioPolicyFailed"
	ReasonCreatedIstioPolicy          = "CreatedIstioPolicy"
	ReasonServiceAccountNotFound      = "ServiceAccountNotFound"
	OtterizeIstioPolicyNameTemplate   = "authorization-policy-to-%s-from-%s"
)

//+kubebuilder:rbac:groups="security.istio.io",resources=authorizationpolicies,verbs=get;update;patch;list;watch;delete;create

type IstioPolicyReconciler struct {
	client.Client
	Scheme                     *runtime.Scheme
	RestrictToNamespaces       []string
	enableIstioPolicyCreation  bool
	enforcementEnabledGlobally bool
	injectablerecorder.InjectableRecorder
	serviceIdResolver *serviceidresolver.Resolver
}

func NewIstioPolicyReconciler(
	c client.Client,
	s *runtime.Scheme,
	restrictToNamespaces []string,
	enableIstioPolicyCreation bool,
	enforcementEnabledGlobally bool) *IstioPolicyReconciler {
	return &IstioPolicyReconciler{
		Client:                     c,
		Scheme:                     s,
		RestrictToNamespaces:       restrictToNamespaces,
		enableIstioPolicyCreation:  enableIstioPolicyCreation,
		enforcementEnabledGlobally: enforcementEnabledGlobally,
		serviceIdResolver:          serviceidresolver.NewResolver(c),
	}
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

	return r.handleAuthorizationPolicy(ctx, req, intents)
}

func (r *IstioPolicyReconciler) handleAuthorizationPolicy(ctx context.Context, req ctrl.Request, intents *otterizev1alpha2.ClientIntents) (ctrl.Result, error) {
	clientServiceAccountName, err := r.serviceIdResolver.ResolveClientIntentToServiceAccountName(ctx, *intents)
	if err != nil {
		if err == serviceidresolver.ServiceAccountNotFond {
			// TODO: Handle missing service account
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

	for _, intent := range intents.GetCallsList() {
		if r.namespaceNotAllowed(intent, req.Namespace) {
			r.RecordWarningEventf(intents, ReasonNamespaceNotAllowed, "namespace %s was specified in intent, but is not allowed by configuration, authorization policy ignored", req.Namespace)
			continue
		}
		err := r.updateOrCreatePolicy(ctx, intents, intent, req.Namespace, clientServiceAccountName)
		if err != nil {
			r.RecordWarningEventf(intents, ReasonCreatingIstioPolicyFailed, "could not create network policies: %s", err.Error())
			return ctrl.Result{}, err
		}
	}

	if len(intents.GetCallsList()) > 0 {
		r.RecordNormalEventf(intents, ReasonCreatedIstioPolicy, "NetworkPolicy reconcile complete, reconciled %d servers", len(intents.GetCallsList()))
	}
	return ctrl.Result{}, nil
}

func (r *IstioPolicyReconciler) namespaceNotAllowed(intent otterizev1alpha2.Intent, requestNamespace string) bool {
	targetNamespace := intent.GetServerNamespace(requestNamespace)
	restrictedNamespacesExists := len(r.RestrictToNamespaces) != 0
	return restrictedNamespacesExists && !lo.Contains(r.RestrictToNamespaces, targetNamespace)
}

func (r *IstioPolicyReconciler) cleanFinalizerAndPolicies(ctx context.Context, intents *otterizev1alpha2.ClientIntents) error {
	if !controllerutil.ContainsFinalizer(intents, IstioPolicyFinalizerName) {
		return nil
	}

	logrus.Infof("Removing Istio policies for deleted intents for service: %s", intents.Spec.Service.Name)

	// TODO: remove authorization policies

	controllerutil.RemoveFinalizer(intents, IstioPolicyFinalizerName)
	return r.Update(ctx, intents)
}

func (r *IstioPolicyReconciler) updateOrCreatePolicy(
	ctx context.Context,
	intents *otterizev1alpha2.ClientIntents,
	intent otterizev1alpha2.Intent,
	objectNamespace string,
	clientServiceAccountName string,
) error {
	clientName := intents.Spec.Service.Name
	policyName := fmt.Sprintf(OtterizeIstioPolicyNameTemplate, intent.GetServerName(), clientName)
	existingPolicy := &v1beta1.AuthorizationPolicy{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      policyName,
		Namespace: intent.GetServerNamespace(objectNamespace)},
		existingPolicy)
	if err != nil && !k8serrors.IsNotFound(err) {
		r.RecordWarningEventf(existingPolicy, ReasonGettingIstioPolicyFailed, "failed to get network policy: %s", err.Error())
		return err
	}

	if k8serrors.IsNotFound(err) {
		return r.CreateAuthorizationPolicyFromIntent(ctx, intent, objectNamespace, policyName, clientServiceAccountName)
	}

	logrus.Infof("Found existing authorization policy %s", policyName)

	//TODO: update network policy if needed

	return nil
}

func (r *IstioPolicyReconciler) CreateAuthorizationPolicyFromIntent(
	ctx context.Context,
	intent otterizev1alpha2.Intent,
	objectNamespace string,
	policyName string,
	clientServiceAccountName string,
) error {
	logrus.Infof("Creating network policy %s for intent %s", policyName, intent.GetServerName())

	serverNamespace := intent.GetServerNamespace(objectNamespace)
	formattedTargetServer := otterizev1alpha2.GetFormattedOtterizeIdentity(intent.GetServerName(), serverNamespace)

	source := fmt.Sprintf("cluster.local/ns/%s/sa/%s", objectNamespace, clientServiceAccountName)
	newPolicy := v1beta1.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: serverNamespace,
		},
		Spec: v1beta12.AuthorizationPolicy{
			Selector: &v1beta13.WorkloadSelector{
				MatchLabels: map[string]string{
					otterizev1alpha2.OtterizeServerLabelKey: formattedTargetServer,
				},
			},
			Action: v1beta12.AuthorizationPolicy_ALLOW,
			Rules: []*v1beta12.Rule{
				{
					From: []*v1beta12.Rule_From{
						{
							Source: &v1beta12.Source{
								Principals: []string{
									source,
								},
							},
						},
					},
				},
			},
		},
	}

	return r.Create(ctx, &newPolicy)
}
