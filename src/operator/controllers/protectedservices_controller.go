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
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
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

func (r *ProtectedServicesReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var ProtectedServices otterizev1alpha2.ProtectedServices
	err := r.Get(ctx, req.NamespacedName, &ProtectedServices)
	if err != nil {
		return ctrl.Result{}, err
	}

	for _, service := range ProtectedServices.Spec.ProtectedServices {
		// TODO: Trigger network policy creation for each service
		logrus.Infof("Protected service: %s", service.Name)
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
