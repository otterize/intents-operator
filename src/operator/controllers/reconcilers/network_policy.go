package reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha1 "github.com/otterize/intents-operator/shared/api/v1alpha1"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const OtterizeNetworkPolicyNameTemplate = "access-to-%s-from-%s"
const OtterizeNetworkPolicy = "otterize/network-policy"
const NetworkPolicyFinalizerName = "otterize-intents.policies/finalizer"

type NetworkPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *NetworkPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	intents := &otterizev1alpha1.Intents{}
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

	logrus.Infof("Reconciling network policies for service: %s in namespace %s",
		intents.Spec.Service.Name, req.Namespace)

	res, err := r.handleFinalizerForIntents(ctx, intents)
	if err != nil {
		return ctrl.Result{}, err
	} else if res.Requeue {
		return res, nil
	}

	callsList := intents.GetCallsList()
	for _, intent := range callsList {
		if intent.Namespace == "" {
			// We never actually update the intent, we just set it here, so we can to access it later
			intent.Namespace = req.Namespace
		}
		err := r.handleNetworkPolicyCreation(ctx, intent, req.Namespace)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *NetworkPolicyReconciler) handleNetworkPolicyCreation(
	ctx context.Context, intent otterizev1alpha1.Intent, intentsObjNamespace string) error {

	policyName := fmt.Sprintf(OtterizeNetworkPolicyNameTemplate, intent.Server, intentsObjNamespace)
	policy := &v1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: policyName, Namespace: intent.Namespace}, policy)

	// No matching network policy found, create one
	if k8serrors.IsNotFound(err) {
		logrus.Infof(
			"Creating network policy to enable access from namespace %s to %s", intent.Server, intentsObjNamespace)
		policy := r.buildNetworkPolicyObjectForIntent(intent, policyName, intentsObjNamespace)
		err := r.Create(ctx, policy)
		if err != nil {
			return err
		}
		return nil

	} else if err != nil {
		return err
	}

	// Found network policy, check for diff
	// TODO: Add code to compensate for changes in intents vs existing policies
	return nil
}

func (r *NetworkPolicyReconciler) handleFinalizerForIntents(
	ctx context.Context, intents *otterizev1alpha1.Intents) (ctrl.Result, error) {

	if intents.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(intents, NetworkPolicyFinalizerName) {
			logrus.Infof("Adding finalizer %s", NetworkPolicyFinalizerName)
			controllerutil.AddFinalizer(intents, NetworkPolicyFinalizerName)
			if err := r.Update(ctx, intents); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(intents, NetworkPolicyFinalizerName) {
			logrus.Infof("Removing network policies for deleted intents for service: %s", intents.Spec.Service.Name)
			callsList := intents.GetCallsList()
			for _, intent := range callsList {
				if intent.Namespace == "" {
					intent.Namespace = intents.ObjectMeta.Namespace
				}

				res, err := r.removeNetworkPolicy(ctx, intent, intents.ObjectMeta.Namespace)
				if err != nil {
					return ctrl.Result{}, err
				} else if res.Requeue {
					return res, nil
				}
			}

			controllerutil.RemoveFinalizer(intents, NetworkPolicyFinalizerName)
			if err := r.Update(ctx, intents); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *NetworkPolicyReconciler) removeNetworkPolicy(
	ctx context.Context,
	intent otterizev1alpha1.Intent,
	intentsObjNamespace string) (ctrl.Result, error) {

	policyName := fmt.Sprintf(OtterizeNetworkPolicyNameTemplate, intent.Server, intentsObjNamespace)
	policy := &v1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: policyName, Namespace: intent.Namespace}, policy)

	if k8serrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else {
		err := r.Delete(ctx, policy)
		if err != nil {
			return ctrl.Result{}, nil
		}
		// We requeue to make sure the object is removed in the next iteration, we don't want to leave ghost policies
		return ctrl.Result{Requeue: true}, nil
	}
}

// buildNetworkPolicyObjectForIntent builds the network policy that represents the intent from the parameter
func (r *NetworkPolicyReconciler) buildNetworkPolicyObjectForIntent(
	intent otterizev1alpha1.Intent, objName, intentsObjNamespace string) *v1.NetworkPolicy {
	otterizeIdentityStr := otterizev1alpha1.GetFormattedOtterizeIdentity(intent.Server, intent.Namespace)

	return &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objName,
			Namespace: intent.Namespace,
			Labels: map[string]string{
				OtterizeNetworkPolicy: "true",
			},
		},
		Spec: v1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha1.OtterizeServerLabelKey: otterizeIdentityStr,
				},
			},
			Ingress: []v1.NetworkPolicyIngressRule{
				{
					From: []v1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									fmt.Sprintf(
										otterizev1alpha1.OtterizeAccessLabelKey, otterizeIdentityStr): "true",
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
