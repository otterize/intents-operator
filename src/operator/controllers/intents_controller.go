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
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	"github.com/otterize/intents-operator/src/shared/reconcilergroup"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IntentsReconciler reconciles a Intents object
type IntentsReconciler struct {
	group *reconcilergroup.Group
}

func NewIntentsReconciler(client client.Client, scheme *runtime.Scheme, kafkaServerStore *kafkaacls.ServersStore, restrictToNamespaces []string) *IntentsReconciler {
	return &IntentsReconciler{
		group: reconcilergroup.NewGroup("intents-reconciler", client, scheme,
			&intents_reconcilers.IntentsValidatorReconciler{Client: client, Scheme: scheme},
			&intents_reconcilers.PodLabelReconciler{Client: client, Scheme: scheme},
			&intents_reconcilers.NetworkPolicyReconciler{Client: client, Scheme: scheme, RestrictToNamespaces: restrictToNamespaces},
			&intents_reconcilers.KafkaACLsReconciler{Client: client, Scheme: scheme, KafkaServersStore: kafkaServerStore},
		)}
}

//+kubebuilder:rbac:groups=otterize.com,resources=intents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=otterize.com,resources=intents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=otterize.com,resources=intents/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;update;patch;list;watch
//+kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;update;patch;list;watch;delete;create
//+kubebuilder:rbac:groups="admissionregistration.k8s.io",resources=validatingwebhookconfigurations,verbs=get;update;patch;list

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *IntentsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.group.Reconcile(ctx, req)
}

// SetupWithManager sets up the controller with the Manager.
func (r *IntentsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&otterizev1alpha1.Intents{}).
		Complete(r)
	if err != nil {
		return err
	}

	r.group.InjectRecorder(mgr.GetEventRecorderFor("intents-operator"))

	return nil
}

// InitIntentsServerIndices indexes intents by target server name
// This is used in finalizers to determine whether a network policy should be removed from the target namespace
func (r *IntentsReconciler) InitIntentsServerIndices(mgr ctrl.Manager) error {
	err := mgr.GetCache().IndexField(
		context.Background(),
		&otterizev1alpha1.Intents{},
		otterizev1alpha1.OtterizeTargetServerIndexField,
		func(object client.Object) []string {
			var res []string
			intents := object.(*otterizev1alpha1.Intents)
			if intents.Spec == nil {
				return nil
			}

			for _, intent := range intents.GetCallsList() {
				res = append(res, intent.Server)
			}

			return res
		})

	if err != nil {
		return err
	}

	return nil
}
