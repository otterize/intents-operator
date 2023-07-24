/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ProtectedServicesReconciler reconciles a ProtectedServices object
type ProtectedServicesReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=k8s.otterize.com,resources=protectedservices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=protectedservices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=protectedservices/finalizers,verbs=update

func NewProtectedServicesReconciler(client client.Client, scheme *runtime.Scheme) *ProtectedServicesReconciler {
	return &ProtectedServicesReconciler{
		Client: client,
		Scheme: scheme,
	}
}

func (r *ProtectedServicesReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if !viper.GetBool(operatorconfig.EnableProtectedServicesKey) {
		return r.DeleteAllDefaultDeny(ctx, req.Namespace)
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

	return ctrl.Result{}, nil
}

func (r *ProtectedServicesReconciler) blockAccessToServices(ctx context.Context, ProtectedServicesResources otterizev1alpha2.ProtectedServicesList, namespace string) error {
	serversToProtect := map[string]v1.NetworkPolicy{}
	for _, list := range ProtectedServicesResources.Items {
		if list.DeletionTimestamp != nil {
			logrus.Infof("Protected service %s is being deleted and ignored", list.Name)
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
		otterizev1alpha2.OtterizeNetworkPolicyDefaultDeny: "true",
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

func (r *ProtectedServicesReconciler) updateIfNeeded(
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

func (r *ProtectedServicesReconciler) buildNetworkPolicyObjectForIntent(
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
				otterizev1alpha2.OtterizeNetworkPolicyDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:            formattedServerName,
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

func (r *ProtectedServicesReconciler) DeleteAllDefaultDeny(ctx context.Context, namespace string) (ctrl.Result, error) {
	var networkPolicies v1.NetworkPolicyList
	err := r.List(ctx, &networkPolicies, client.InNamespace(namespace), client.MatchingLabels{
		otterizev1alpha2.OtterizeNetworkPolicyDefaultDeny: "true",
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

// SetupWithManager sets up the controller with the Manager.
func (r *ProtectedServicesReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&otterizev1alpha2.ProtectedServices{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}
