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
	"github.com/otterize/intents-operator/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/operator/controllers/kafkaacls"
	otterizev1alpha1 "github.com/otterize/intents-operator/shared/api/v1alpha1"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IntentsReconciler reconciles a Intents object
type IntentsReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	KafkaServersStore *kafkaacls.ServersStore
}

//+kubebuilder:rbac:groups=otterize.com,resources=intents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=otterize.com,resources=intents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=otterize.com,resources=intents/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;update;patch;list;watch
//+kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;update;patch;list;watch;delete;create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *IntentsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reconcilersList, err := r.buildReconcilersList(r.Client, r.Scheme)
	if err != nil {
		return ctrl.Result{}, err
	}

	logrus.Infoln("## Starting new Otterize reconciliation cycle ##")
	for _, r := range reconcilersList {
		logrus.Infof("Starting cycle for %T", r)
		res, err := r.Reconcile(ctx, req)
		if res.Requeue == true || err != nil {
			return res, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IntentsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&otterizev1alpha1.Intents{}).
		Complete(r)
}

func (r *IntentsReconciler) buildReconcilersList(c client.Client, scheme *runtime.Scheme) ([]reconcile.Reconciler, error) {
	l := make([]reconcile.Reconciler, 0)

	l = append(l, &intents_reconcilers.IntentsValidatorReconciler{Client: c, Scheme: scheme})
	l = append(l, &intents_reconcilers.PodLabelReconciler{Client: c, Scheme: scheme})
	l = append(l, &intents_reconcilers.NetworkPolicyReconciler{Client: c, Scheme: scheme})
	l = append(l, &intents_reconcilers.KafkaACLsReconciler{Client: c, Scheme: scheme, KafkaServersStores: r.KafkaServersStore})

	return l, nil
}
