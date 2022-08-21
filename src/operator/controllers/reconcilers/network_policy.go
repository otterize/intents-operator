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
)

const OtterizeNetworkPolicyNameTemplate = "access-to-%s-from-%s"
const OtterizeNetworkPolicy = "otterize/network-policy"

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
	callsList := intents.GetCallsList()

	logrus.Infof("Reconciling network policies for service: %s in namespace %s",
		intents.Spec.Service.Name, req.Namespace)

	for _, intent := range callsList {
		if intent.Namespace == "" {
			// We never actually update the intent, we just set it here, so we can to access it later
			intent.Namespace = req.Namespace
		}
		res, err := r.handleNetworkPolicyCreation(ctx, intent, req.Namespace)
		if err != nil {
			return res, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *NetworkPolicyReconciler) handleNetworkPolicyCreation(
	ctx context.Context, intent otterizev1alpha1.Intent, clientNamespace string) (ctrl.Result, error) {

	policyName := fmt.Sprintf(OtterizeNetworkPolicyNameTemplate, intent.Server, clientNamespace)
	logrus.Infof("Looking for policy: %s", policyName)
	policy := &v1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: policyName, Namespace: intent.Namespace}, policy)

	// No matching network policy found, create one
	if k8serrors.IsNotFound(err) {
		logrus.Infof(
			"Creating network policy to enable access from namespace %s to %s", intent.Server, clientNamespace)
		policy := r.buildNetworkPolicyObjectForIntent(intent, policyName, clientNamespace)
		err := r.Create(ctx, policy)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil

	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Found network policy, check for diff
	// TODO: Add code to compensate for changes in intents vs existing policies
	return ctrl.Result{}, nil
}

// buildNetworkPolicyObjectForIntent builds the network policy that represents the intent from the parameter
func (r *NetworkPolicyReconciler) buildNetworkPolicyObjectForIntent(
	intent otterizev1alpha1.Intent, objName, clientNamespace string) *v1.NetworkPolicy {
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
									otterizev1alpha1.OtterizeNamespaceLabelKey: clientNamespace,
								},
							},
						},
					},
				},
			},
		},
	}
}
