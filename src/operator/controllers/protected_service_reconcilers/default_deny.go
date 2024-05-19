package protected_service_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DefaultDenyReconciler reconciles a ProtectedService object
type DefaultDenyReconciler struct {
	client.Client
	extNetpolHandler ExternalNepolHandler
	injectablerecorder.InjectableRecorder
	netpolEnforcementEnabled bool
}

type ExternalNepolHandler interface {
	HandlePodsByNamespace(ctx context.Context, namespace string) error
	HandleAllPods(ctx context.Context) error
}

func NewDefaultDenyReconciler(client client.Client, extNetpolHandler ExternalNepolHandler, netpolEnforcementEnabled bool) *DefaultDenyReconciler {
	return &DefaultDenyReconciler{
		Client:                   client,
		extNetpolHandler:         extNetpolHandler,
		netpolEnforcementEnabled: netpolEnforcementEnabled,
	}
}

func (r *DefaultDenyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	err := r.handleDefaultDenyInNamespace(ctx, req)
	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	return ctrl.Result{}, nil
}

func (r *DefaultDenyReconciler) handleDefaultDenyInNamespace(ctx context.Context, req ctrl.Request) error {
	var protectedServices otterizev1alpha3.ProtectedServiceList

	err := r.List(ctx, &protectedServices, client.InNamespace(req.Namespace))
	if err != nil {
		return errors.Wrap(err)
	}

	err = r.blockAccessToServices(ctx, protectedServices, req.Namespace)
	if err != nil {
		return errors.Wrap(err)
	}

	return r.extNetpolHandler.HandleAllPods(ctx)
}

func (r *DefaultDenyReconciler) blockAccessToServices(ctx context.Context, protectedServices otterizev1alpha3.ProtectedServiceList, namespace string) error {
	serversToProtect := map[string]v1.NetworkPolicy{}
	for _, protectedService := range protectedServices.Items {
		if protectedService.DeletionTimestamp != nil {
			continue
		}

		serverServiceIdentity := protectedService.ToServiceIdentity()
		formattedServerName := serverServiceIdentity.GetFormattedOtterizeIdentityWithKind()
		policy, shouldCreate, err := r.buildNetworkPolicyObjectForIntent(ctx, serverServiceIdentity)
		if err != nil {
			return errors.Wrap(err)
		}
		if !shouldCreate {
			continue
		}
		if r.netpolEnforcementEnabled {
			serversToProtect[formattedServerName] = policy
		}
	}

	var networkPolicies v1.NetworkPolicyList
	err := r.List(ctx, &networkPolicies, client.InNamespace(namespace), client.MatchingLabels{
		otterizev1alpha3.OtterizeNetworkPolicyServiceDefaultDeny: "true",
	})
	if err != nil {
		return errors.Wrap(err)
	}

	for _, existingPolicy := range networkPolicies.Items {
		existingPolicyServerName := existingPolicy.Labels[otterizev1alpha3.OtterizeNetworkPolicy]
		_, found := serversToProtect[existingPolicyServerName]
		if found {
			desiredPolicy := serversToProtect[existingPolicyServerName]
			err = r.updateIfNeeded(existingPolicy, desiredPolicy)
			if err != nil {
				return errors.Wrap(err)
			}
			delete(serversToProtect, existingPolicyServerName)
		} else {
			err = r.Delete(ctx, &existingPolicy)
			if err != nil {
				return errors.Wrap(err)
			}
			logrus.Debugf("Deleted network policy %s", existingPolicy.Name)
		}
	}

	for _, networkPolicy := range serversToProtect {
		err = r.Create(ctx, &networkPolicy)
		if err != nil {
			return errors.Wrap(err)
		}
		logrus.Debugf("Created network policy %s", networkPolicy.Name)
	}

	return nil
}

func (r *DefaultDenyReconciler) updateIfNeeded(
	existingPolicy v1.NetworkPolicy,
	newPolicy v1.NetworkPolicy,
) error {
	if reflect.DeepEqual(existingPolicy.Spec, newPolicy.Spec) && reflect.DeepEqual(existingPolicy.Labels, newPolicy.Labels) {
		return nil
	}

	existingPolicy.Spec = newPolicy.Spec
	existingPolicy.Labels = newPolicy.Labels

	err := r.Update(context.Background(), &existingPolicy)
	if err != nil {
		return errors.Wrap(err)
	}

	logrus.Debugf("Updated network policy %s", existingPolicy.Name)
	return nil
}

func (r *DefaultDenyReconciler) buildNetworkPolicyObjectForIntent(ctx context.Context, serviceId *serviceidentity.ServiceIdentity) (v1.NetworkPolicy, bool, error) {
	policyName := fmt.Sprintf("default-deny-%s", serviceId.GetNameWithKind())
	podSelectorLabels, ok, err := otterizev1alpha3.ServiceIdentityToLabelsForWorkloadSelection(ctx, r.Client, serviceId)
	if err != nil {
		return v1.NetworkPolicy{}, false, errors.Wrap(err)
	}
	if !ok {
		return v1.NetworkPolicy{}, false, nil
	}
	netpol := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: serviceId.Namespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeNetworkPolicyServiceDefaultDeny: "true",
				otterizev1alpha3.OtterizeNetworkPolicy:                   serviceId.GetFormattedOtterizeIdentityWithoutKind(),
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: podSelectorLabels,
			},
			Ingress: []v1.NetworkPolicyIngressRule{},
		},
	}

	return netpol, true, nil
}

func (r *DefaultDenyReconciler) DeleteAllDefaultDeny(ctx context.Context, namespace string) (ctrl.Result, error) {
	var networkPolicies v1.NetworkPolicyList
	err := r.List(ctx, &networkPolicies, client.InNamespace(namespace), client.MatchingLabels{
		otterizev1alpha3.OtterizeNetworkPolicyServiceDefaultDeny: "true",
	})
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	for _, existingPolicy := range networkPolicies.Items {
		err = r.Delete(ctx, &existingPolicy)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err)
		}
		logrus.Debugf("Deleted network policy %s", existingPolicy.Name)
	}

	return ctrl.Result{}, nil
}
