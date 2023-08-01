package protected_services_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DefaultDenyReconciler reconciles a ProtectedServices object
type DefaultDenyReconciler struct {
	client.Client
	extNetpolHandler ExternalNepolHandler
	injectablerecorder.InjectableRecorder
}

type ExternalNepolHandler interface {
	HandlePodsByNamespace(ctx context.Context, namespace string) error
	HandleAllPods(ctx context.Context) error
}

func NewDefaultDenyReconciler(client client.Client, extNetpolHandler ExternalNepolHandler) *DefaultDenyReconciler {
	return &DefaultDenyReconciler{
		Client:           client,
		extNetpolHandler: extNetpolHandler,
	}
}

func (r *DefaultDenyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if !viper.GetBool(operatorconfig.EnableProtectedServicesKey) {
		res, err := r.DeleteAllDefaultDeny(ctx, req.Namespace)
		if err != nil || !res.IsZero() {
			return res, err
		}
		return ctrl.Result{}, r.extNetpolHandler.HandleAllPods(ctx)

	}

	var ProtectedServicesResources otterizev1alpha2.ProtectedServicesList

	err := r.List(ctx, &ProtectedServicesResources, client.InNamespace(req.Namespace))
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.blockAccessToServices(ctx, ProtectedServicesResources, req.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.extNetpolHandler.HandlePodsByNamespace(ctx, req.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *DefaultDenyReconciler) blockAccessToServices(ctx context.Context, ProtectedServicesResources otterizev1alpha2.ProtectedServicesList, namespace string) error {
	serversToProtect := map[string]v1.NetworkPolicy{}
	for _, list := range ProtectedServicesResources.Items {
		if list.DeletionTimestamp != nil {
			continue
		}

		for _, service := range list.Spec.ProtectedServices {
			formattedServerName := otterizev1alpha2.GetFormattedOtterizeIdentity(service.Name, namespace)
			policy := r.buildNetworkPolicyObjectForIntent(formattedServerName, service.Name, namespace)
			serversToProtect[formattedServerName] = policy
		}
	}

	var networkPolicies v1.NetworkPolicyList
	err := r.List(ctx, &networkPolicies, client.InNamespace(namespace), client.MatchingLabels{
		otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
	})
	if err != nil {
		return err
	}

	for _, existingPolicy := range networkPolicies.Items {
		existingPolicyServerName := existingPolicy.Labels[otterizev1alpha2.OtterizeNetworkPolicy]
		_, found := serversToProtect[existingPolicyServerName]
		if found {
			desiredPolicy := serversToProtect[existingPolicyServerName]
			err = r.updateIfNeeded(existingPolicy, desiredPolicy)
			if err != nil {
				return err
			}
			delete(serversToProtect, existingPolicyServerName)
		} else {
			err = r.Delete(ctx, &existingPolicy)
			if err != nil {
				return err
			}
			logrus.Infof("Deleted network policy %s", existingPolicy.Name)
		}
	}

	for _, networkPolicy := range serversToProtect {
		err = r.Create(ctx, &networkPolicy)
		if err != nil {
			return err
		}
		logrus.Infof("Created network policy %s", networkPolicy.Name)
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
		return err
	}

	logrus.Infof("Updated network policy %s", existingPolicy.Name)
	return nil
}

func (r *DefaultDenyReconciler) buildNetworkPolicyObjectForIntent(
	formattedServerName string,
	serviceName string,
	namespace string,
) v1.NetworkPolicy {
	policyName := fmt.Sprintf("default-deny-%s", serviceName)
	return v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: namespace,
			Labels: map[string]string{
				otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:                   formattedServerName,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha2.OtterizeServerLabelKey: formattedServerName,
				},
			},
			Ingress: []v1.NetworkPolicyIngressRule{},
		},
	}
}

func (r *DefaultDenyReconciler) DeleteAllDefaultDeny(ctx context.Context, namespace string) (ctrl.Result, error) {
	var networkPolicies v1.NetworkPolicyList
	err := r.List(ctx, &networkPolicies, client.InNamespace(namespace), client.MatchingLabels{
		otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	for _, existingPolicy := range networkPolicies.Items {
		err = r.Delete(ctx, &existingPolicy)
		if err != nil {
			return ctrl.Result{}, err
		}
		logrus.Infof("Deleted network policy %s", existingPolicy.Name)
	}

	return ctrl.Result{}, nil
}
