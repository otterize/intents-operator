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

const OtterizeNetworkPolicyNameTemplate = "%s-to-%s-at-%s"
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

	for _, intent := range callsList {
		if intent.Namespace == "" {
			// We never actually update the intent, we just set it here, so we can to access it later
			intent.Namespace = req.Namespace
		}
		res, err := r.handleNetworkPolicyCreation(ctx, intents.GetServiceName(), intent)
		if err != nil {
			return res, err
		}
		if res.Requeue {
			return res, nil
		}
	}

	return ctrl.Result{}, nil
}

func (r *NetworkPolicyReconciler) formatNetworkPolicyResourceName(client string, intent otterizev1alpha1.Intent) string {

	return fmt.Sprintf(OtterizeNetworkPolicyNameTemplate, client, intent.Server, intent.Namespace)
}

func (r *NetworkPolicyReconciler) handleNetworkPolicyCreation(
	ctx context.Context,
	client string,
	intent otterizev1alpha1.Intent) (ctrl.Result, error) {

	policy := &v1.NetworkPolicy{}
	policyName := r.formatNetworkPolicyResourceName(client, intent)
	err := r.Get(ctx, types.NamespacedName{Name: policyName, Namespace: intent.Namespace}, policy)

	// No matching network policy found, create one
	if k8serrors.IsNotFound(err) {
		logrus.Infof("Updating network policy for %s", client)
		policy := r.buildNetworkPolicyObjectForIntent(intent, policyName)
		err := r.Create(ctx, policy)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil

	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Found network policy, skip creation
	// TODO: Add code to compensate for changes in intents vs existing policies
	return ctrl.Result{}, nil

}

// buildNetworkPolicyObjectForIntent builds the network policy that represents the intent from the parameter
func (r *NetworkPolicyReconciler) buildNetworkPolicyObjectForIntent(intent otterizev1alpha1.Intent, objName string) *v1.NetworkPolicy {
	otterizeIdentityStr := otterizev1alpha1.GetFormattedOtterizeIdentity(intent.Server, intent.Namespace)

	return &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: objName,
			// NOTE: this does NOT mean the intent's namespace - but rather the target server's namespace
			// Since we enforce on Ingress, this makes sense
			Namespace: intent.Namespace,
			Labels: map[string]string{
				OtterizeNetworkPolicy: "true",
			},
		},
		Spec: v1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha1.OtterizeServerLabelKey: fmt.Sprintf("%s-%s", intent.Server, intent.Namespace),
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
						},
					},
				},
			},
		},
	}
}
