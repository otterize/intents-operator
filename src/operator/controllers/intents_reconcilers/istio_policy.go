package intents_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"istio.io/client-go/pkg/apis/security/v1beta1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
	OtterizeIstioPolicyNameTemplate   = "authorization-policy-to-%s-from-%s"
)

//+kubebuilder:rbac:groups="security.istio.io",resources=authorizationpolicies,verbs=get;update;patch;list;watch;delete;create
//+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch

type IstioPolicyReconciler struct {
	client.Client
	Scheme                     *runtime.Scheme
	RestrictToNamespaces       []string
	enableIstioPolicyCreation  bool
	enforcementEnabledGlobally bool
	injectablerecorder.InjectableRecorder
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

	// Object is deleted, handle finalizer and network policy clean up
	if !intents.DeletionTimestamp.IsZero() {
		err := r.cleanFinalizerAndPolicies(ctx, intents)
		if err != nil {
			if k8serrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			r.RecordWarningEventf(intents, ReasonRemovingIstioPolicyFailed, "could not remove network policies: %s", err.Error())
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

	for _, intent := range intents.GetCallsList() {
		targetNamespace := intent.GetServerNamespace(req.Namespace)
		if len(r.RestrictToNamespaces) != 0 && !lo.Contains(r.RestrictToNamespaces, targetNamespace) {
			// Namespace is not in list of namespaces we're allowed to act in, so drop it.
			r.RecordWarningEventf(intents, ReasonNamespaceNotAllowed, "namespace %s was specified in intent, but is not allowed by configuration", targetNamespace)
			continue
		}
		err := r.handleNetworkPolicyCreation(ctx, intents, intent, req.Namespace)
		if err != nil {
			r.RecordWarningEventf(intents, ReasonCreatingIstioPolicyFailed, "could not create network policies: %s", err.Error())
			return ctrl.Result{}, err
		}
	}

	if len(intents.GetCallsList()) > 0 {
		r.RecordNormalEventf(intents, ReasonCreatedIstioPolicy, "NetworkPolicy reconcile complete, reconciled %d servers", len(intents.GetCallsList()))
	}

	logrus.Info("CreateAuthorizationPolicy istio client end")

	return ctrl.Result{}, nil
}

func (r *IstioPolicyReconciler) cleanFinalizerAndPolicies(ctx context.Context, intents *otterizev1alpha2.ClientIntents) error {
	if !controllerutil.ContainsFinalizer(intents, IstioPolicyFinalizerName) {
		return nil
	}

	logrus.Infof("Removing Istio policies for deleted intents for service: %s", intents.Spec.Service.Name)
	// TODO: remove authorization policies

	controllerutil.RemoveFinalizer(intents, IstioPolicyFinalizerName)
	if err := r.Update(ctx, intents); err != nil {
		return err
	}

	return nil
}

func (r *IstioPolicyReconciler) handleIstioPolicyCreation(ctx context.Context, intentsObj *otterizev1alpha2.ClientIntents, intent otterizev1alpha2.Intent, intentsObjNamespace string) error {
	if !r.enforcementEnabledGlobally {
		r.RecordNormalEvent(intentsObj, ReasonEnforcementGloballyDisabled, "Enforcement is disabled globally, istio policy creation skipped")
		return nil
	}
	if !r.enableIstioPolicyCreation {
		r.RecordNormalEvent(intentsObj, ReasonIstioPolicyCreationDisabled, "Istio policy creation is disabled, creation skipped")
		return nil
	}

	return nil
}

func (r *IstioPolicyReconciler) handleNetworkPolicyCreation(
	ctx context.Context,
	intentsObj *otterizev1alpha2.ClientIntents,
	intent otterizev1alpha2.Intent,
	intentsObjNamespace string,
) error {
	if !r.enforcementEnabledGlobally {
		r.RecordNormalEvent(intentsObj, ReasonEnforcementGloballyDisabled, "Enforcement is disabled globally, network policy creation skipped")
		return nil
	}
	if !r.enableIstioPolicyCreation {
		r.RecordNormalEvent(intentsObj, ReasonNetworkPolicyCreationDisabled, "Network policy creation is disabled, creation skipped")
		return nil
	}

	policyName := fmt.Sprintf(OtterizeIstioPolicyNameTemplate, intent.GetServerName(), intentsObj.Spec.Service.Name)
	existingPolicy := &v1beta1.AuthorizationPolicy{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      policyName,
		Namespace: intent.GetServerNamespace(intentsObjNamespace)},
		existingPolicy)
	if err != nil && !k8serrors.IsNotFound(err) {
		r.RecordWarningEventf(existingPolicy, ReasonGettingIstioPolicyFailed, "failed to get network policy: %s", err.Error())
		return err
	}

	// No matching network policy found, create one
	if k8serrors.IsNotFound(err) {
		return r.CreateAuthorizationPolicyFromIntent(ctx, intentsObj, intent, intentsObjNamespace, policyName)
	}

	logrus.Infof("Found existing network policy %s", existingPolicy.Name)

	//TODO: update network policy if needed

	return nil
}

func (r *IstioPolicyReconciler) CreateAuthorizationPolicyFromIntent(
	ctx context.Context,
	intentsObj *otterizev1alpha2.ClientIntents,
	intent otterizev1alpha2.Intent,
	intentsObjNamespace string,
	policyName string,
) error {
	logrus.Infof("Creating network policy %s for intent %s", policyName, intent.GetServerName())

	clientServiceAccountName, err := r.ResolveServiceNameToServiceAccountName(ctx, intentsObj.Spec.Service.Name, intentsObjNamespace)
	if err != nil {
		return err
	}

	newPolicy := CreateAuthorizationPolicy(intent, intentsObjNamespace, policyName, clientServiceAccountName)
	err = r.Create(ctx, newPolicy)
	if err != nil {
		return err
	}
	return nil
}

func (r *IstioPolicyReconciler) ResolveServiceNameToServiceAccountName(ctx context.Context, otterizeServiceName string, namespace string) (string, error) {
	logrus.Infof("Resolving otterize service name %s to service account name", otterizeServiceName)
	// Search for pods with service name annotation
	podsList := &corev1.PodList{}
	// TODO: Search in pods by annotation and not by label
	err := r.List(ctx, podsList, client.MatchingLabels{serviceidresolver.ServiceNameAnnotation: otterizeServiceName})
	if err != nil && !errors.IsNotFound(err) {
		logrus.Infof("Error searching for pods with service name annotation %s: %s", otterizeServiceName, err.Error())
		return "", err
	}

	logrus.Infof("Found pods with service name annotation %s: %v", otterizeServiceName, podsList.Items)
	if len(podsList.Items) > 0 {
		logrus.Infof("Found pod with service name annotation %s, using service account name %s", otterizeServiceName, podsList.Items[0].Spec.ServiceAccountName)
		return podsList.Items[0].Spec.ServiceAccountName, nil
	}

	logrus.Infof("No pods found with service name annotation %s, searching for deployment", otterizeServiceName)
	// TODO: Search for any pod owner object with the same name as the service name
	deployment := &v1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: otterizeServiceName, Namespace: namespace}, deployment)
	if err != nil {
		logrus.Infof("Error searching for deployment with name %s: %s", otterizeServiceName, err.Error())
		return "", err
	}

	logrus.Infof("Found deployment with name %s, using service account name %s", otterizeServiceName, deployment.Spec.Template.Spec.ServiceAccountName)

	return deployment.Spec.Template.Spec.ServiceAccountName, nil
}
