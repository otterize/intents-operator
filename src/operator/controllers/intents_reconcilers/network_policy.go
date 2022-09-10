package intents_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/consts"
	"github.com/otterize/intents-operator/src/operator/controllers/external_traffic"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const OtterizeNetworkPolicyNameTemplate = "access-to-%s-from-%s"

const NetworkPolicyFinalizerName = "otterize-intents.policies/finalizer"

type NetworkPolicyReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	endpointsReconciler  *external_traffic.EndpointsReconciler
	RestrictToNamespaces []string
	injectablerecorder.InjectableRecorder
}

func NewNetworkPolicyReconciler(c client.Client, s *runtime.Scheme, endpointsReconciler *external_traffic.EndpointsReconciler, restrictToNamespaces []string) *NetworkPolicyReconciler {
	return &NetworkPolicyReconciler{
		Client:               c,
		Scheme:               s,
		endpointsReconciler:  endpointsReconciler,
		RestrictToNamespaces: restrictToNamespaces,
	}
}

func (r *NetworkPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	intents := &otterizev1alpha1.ClientIntents{}
	err := r.Get(ctx, req.NamespacedName, intents)
	if k8serrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	if intents.Spec == nil {
		return ctrl.Result{}, nil
	}

	logrus.Infof("Reconciling network policies for service %s in namespace %s",
		intents.Spec.Service.Name, req.Namespace)

	// Object is deleted, handle finalizer and network policy clean up
	if !intents.DeletionTimestamp.IsZero() {
		err := r.cleanFinalizerAndPolicies(ctx, intents)
		if err != nil {
			if k8serrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			r.RecordWarningEvent(intents, "could not remove network policies", err.Error())
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(intents, NetworkPolicyFinalizerName) {
		logrus.WithField("namespacedName", req.String()).Infof("Adding finalizer %s", NetworkPolicyFinalizerName)
		controllerutil.AddFinalizer(intents, NetworkPolicyFinalizerName)
		if err := r.Update(ctx, intents); err != nil {
			return ctrl.Result{}, err
		}
	}
	for _, intent := range intents.GetCallsList() {
		if intent.Namespace == "" {
			// We never actually update the intent, we just set it here, so we can to access it later
			intent.Namespace = req.Namespace
		}
		if len(r.RestrictToNamespaces) != 0 && !lo.Contains(r.RestrictToNamespaces, intent.Namespace) {
			// Namespace is not in list of namespaces we're allowed to act in, so drop it.
			r.RecordWarningEventf(intents, "namespace not allowed", "namespace %s was specified in intent, but is not allowed by configuration", intent.Namespace)
			continue
		}
		err := r.handleNetworkPolicyCreation(ctx, intent, req.Namespace)
		if err != nil {
			r.RecordWarningEvent(intents, "could not create network policies", err.Error())
			return ctrl.Result{}, err
		}
	}

	if len(intents.GetCallsList()) > 0 {
		r.RecordNormalEventf(intents, "NetworkPolicy reconcile complete", "Reconciled %d servers", len(intents.GetCallsList()))
	}
	return ctrl.Result{}, nil
}

func (r *NetworkPolicyReconciler) handleNetworkPolicyCreation(
	ctx context.Context, intent otterizev1alpha1.Intent, intentsObjNamespace string) error {

	policyName := fmt.Sprintf(OtterizeNetworkPolicyNameTemplate, intent.Name, intentsObjNamespace)
	existingPolicy := &v1.NetworkPolicy{}
	newPolicy := r.buildNetworkPolicyObjectForIntent(intent, policyName, intentsObjNamespace)
	err := r.Get(ctx, types.NamespacedName{Name: policyName, Namespace: intent.Namespace}, existingPolicy)

	// No matching network policy found, create one
	if k8serrors.IsNotFound(err) {
		logrus.Infof(
			"Creating network policy to enable access from namespace %s to %s", intentsObjNamespace, intent.Name)
		err := r.Create(ctx, newPolicy)
		if err != nil {
			return err
		}

		selector, err := metav1.LabelSelectorAsSelector(&newPolicy.Spec.PodSelector)
		if err != nil {
			return err
		}
		// Are any pods affected right now? If so, we need to request a Reconcile from the EndpointsReconciler, since
		// it doesn't get notified on network policy changes.
		podList := &corev1.PodList{}
		err = r.List(ctx, podList,
			&client.ListOptions{Namespace: newPolicy.Namespace},
			client.MatchingLabelsSelector{Selector: selector})
		if err != nil {
			return err
		}

		affectedEndpointsList := sets.NewString()

		// If so, check whether they belong to endpoints (= are used by a service), and send those to the EndpointsReconciler.
		for _, pod := range podList.Items {
			var endpointsList corev1.EndpointsList
			err = r.List(
				ctx, &endpointsList,
				&client.MatchingFields{otterizev1alpha1.EndpointsPodNamesIndexField: pod.Name},
				&client.ListOptions{Namespace: pod.Namespace})

			if err != nil {
				return err
			}
			for _, endpoints := range endpointsList.Items {
				affectedEndpointsList.Insert(endpoints.Name)
			}
		}

		for affectedEndpointsName := range affectedEndpointsList {
			_, err := r.endpointsReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: affectedEndpointsName, Namespace: newPolicy.Namespace}})
			if err != nil {
				return err
			}
		}

		return nil

	} else if err != nil {
		r.RecordWarningEvent(existingPolicy, "failed to get network policy", err.Error())
		return err
	}

	// Found network policy, check for diff
	if !reflect.DeepEqual(existingPolicy.Spec, newPolicy.Spec) {
		policyCopy := existingPolicy.DeepCopy()
		policyCopy.Labels = newPolicy.Labels
		policyCopy.Annotations = newPolicy.Annotations
		policyCopy.Spec = newPolicy.Spec

		err = r.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy))
		if err != nil {
			return err
		}

		err = r.handleNetworkPolicyRemoval(ctx, intent, intentsObjNamespace)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *NetworkPolicyReconciler) cleanFinalizerAndPolicies(
	ctx context.Context, intents *otterizev1alpha1.ClientIntents) error {
	if !controllerutil.ContainsFinalizer(intents, NetworkPolicyFinalizerName) {
		return nil
	}
	logrus.Infof("Removing network policies for deleted intents for service: %s", intents.Spec.Service.Name)
	for _, intent := range intents.GetCallsList() {
		if intent.Namespace == "" {
			intent.Namespace = intents.Namespace
		}

		err := r.handleNetworkPolicyRemoval(ctx, intent, intents.Namespace)
		if err != nil {
			return err
		}
	}

	controllerutil.RemoveFinalizer(intents, NetworkPolicyFinalizerName)
	if err := r.Update(ctx, intents); err != nil {
		return err
	}

	return nil
}

func (r *NetworkPolicyReconciler) handleNetworkPolicyRemoval(
	ctx context.Context,
	intent otterizev1alpha1.Intent,
	intentsObjNamespace string) error {

	var intentsList otterizev1alpha1.ClientIntentsList
	err := r.List(
		ctx, &intentsList,
		&client.MatchingFields{otterizev1alpha1.OtterizeTargetServerIndexField: intent.Name},
		&client.ListOptions{Namespace: intentsObjNamespace})

	if err != nil {
		return err
	}

	if len(intentsList.Items) == 1 {
		// We have only 1 intents resource that has this server as its target - and it's the current one
		// We need to delete the network policy that allows access from this namespace, as there are no other
		// clients in that namespace that need to access the target server
		logrus.Infof("No other intents in the namespace reference target server: %s", intent.Name)
		logrus.Infoln("Removing matching network policy for server")
		if err = r.deleteNetworkPolicy(ctx, intent, intentsObjNamespace); err != nil {
			return err
		}
	}
	return nil
}

func (r *NetworkPolicyReconciler) deleteNetworkPolicy(
	ctx context.Context,
	intent otterizev1alpha1.Intent,
	intentsObjNamespace string) error {

	policyName := fmt.Sprintf(OtterizeNetworkPolicyNameTemplate, intent.Name, intentsObjNamespace)
	policy := &v1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: policyName, Namespace: intent.Namespace}, policy)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	// Remove network policies created by the external traffic reconcilers.
	// Once no more Otterize network policies are present, there's no longer need for them.
	externalPolicyList := &v1.NetworkPolicyList{}
	serviceNameLabel := policy.Labels[consts.OtterizeNetworkPolicy]
	err = r.List(ctx, externalPolicyList, client.MatchingLabels{consts.OtterizeNetworkPolicyExternalTraffic: serviceNameLabel},
		&client.ListOptions{Namespace: policy.Namespace})
	if err != nil {
		return err
	}

	for _, externalPolicy := range externalPolicyList.Items {
		err := r.Delete(ctx, externalPolicy.DeepCopy())
		if err != nil {
			return err
		}
	}

	return r.Delete(ctx, policy)
}

// buildNetworkPolicyObjectForIntent builds the network policy that represents the intent from the parameter
func (r *NetworkPolicyReconciler) buildNetworkPolicyObjectForIntent(
	intent otterizev1alpha1.Intent, policyName, intentsObjNamespace string) *v1.NetworkPolicy {
	// The intent's target server made of name + namespace + hash
	formattedTargetServer := otterizev1alpha1.GetFormattedOtterizeIdentity(intent.Name, intent.Namespace)

	return &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: intent.Namespace,
			Labels: map[string]string{
				consts.OtterizeNetworkPolicy: formattedTargetServer,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha1.OtterizeServerLabelKey: formattedTargetServer,
				},
			},
			Ingress: []v1.NetworkPolicyIngressRule{
				{
					From: []v1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									fmt.Sprintf(
										otterizev1alpha1.OtterizeAccessLabelKey, formattedTargetServer): "true",
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									otterizev1alpha1.OtterizeNamespaceLabelKey: intentsObjNamespace,
								},
							},
						},
					},
				},
			},
		},
	}
}
