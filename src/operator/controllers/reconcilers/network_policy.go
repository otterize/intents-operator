package reconcilers

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/operator/api/v1alpha1"
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
	intents := &v1alpha1.Intents{}
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

		err := r.handleNetworkPolicyCreation(ctx, intent, req.Namespace)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *NetworkPolicyReconciler) handleNetworkPolicyCreation(
	ctx context.Context, intent v1alpha1.Intent, intentsObjNamespace string) error {

	policyName := fmt.Sprintf(OtterizeNetworkPolicyNameTemplate, intent.Server, intentsObjNamespace)
	logrus.Infof("Looking for policy: %s", policyName)
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

// buildNetworkPolicyObjectForIntent builds the network policy that represents the intent from the parameter
func (r *NetworkPolicyReconciler) buildNetworkPolicyObjectForIntent(
	intent v1alpha1.Intent, policyName, intentsObjNamespace string) *v1.NetworkPolicy {
	// The intent's target server made of name + namespace + hash
	formattedTargetServer := v1alpha1.GetFormattedOtterizeIdentity(intent.Server, intent.Namespace)

	return &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: intent.Namespace,
			Labels: map[string]string{
				OtterizeNetworkPolicy: "true",
			},
		},
		Spec: v1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					v1alpha1.OtterizeServerLabelKey: formattedTargetServer,
				},
			},
			Ingress: []v1.NetworkPolicyIngressRule{
				{
					From: []v1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									fmt.Sprintf(
										v1alpha1.OtterizeAccessLabelKey, formattedTargetServer): "true",
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									v1alpha1.OtterizeNamespaceLabelKey: intentsObjNamespace,
								},
							},
						},
					},
				},
			},
		},
	}
}
