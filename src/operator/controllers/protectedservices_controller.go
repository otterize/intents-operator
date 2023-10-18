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
	"github.com/otterize/intents-operator/src/operator/controllers/protected_service_reconcilers"
	"github.com/otterize/intents-operator/src/operator/controllers/protected_service_reconcilers/consts"
	"github.com/otterize/intents-operator/src/shared/initonce"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/otterize/intents-operator/src/shared/reconcilergroup"
	"github.com/samber/lo"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	protectedServicesGroupName = "protected-services"
)

// ProtectedServiceReconciler reconciles a ProtectedService object
type ProtectedServiceReconciler struct {
	client.Client
	group                *reconcilergroup.Group
	finalizersForRemoval []string
	initOnce             initonce.InitOnce
}

//+kubebuilder:rbac:groups=k8s.otterize.com,resources=protectedservices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=protectedservices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=protectedservices/finalizers,verbs=update

func NewProtectedServiceReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	otterizeClient operator_cloud_client.CloudClient,
	extNetpolHandler protected_service_reconcilers.ExternalNepolHandler,
	enforcementDefaultState bool,
	netpolEnforcementEnabled bool,
	networkPolicyHandler protected_service_reconcilers.NetworkPolicyHandler,
) *ProtectedServiceReconciler {
	var finalizersForRemoval []string

	group := reconcilergroup.NewGroup(protectedServicesGroupName, client, scheme)

	if netpolEnforcementEnabled {
		defaultDenyReconciler := protected_service_reconcilers.NewDefaultDenyReconciler(client, extNetpolHandler, netpolEnforcementEnabled)
		group.AddToGroup(defaultDenyReconciler)
	} else {
		finalizersForRemoval = append(finalizersForRemoval, consts.DefaultDenyReconcilerFinalizerName)
	}

	if !enforcementDefaultState || !netpolEnforcementEnabled {
		policyCleaner := protected_service_reconcilers.NewPolicyCleanerReconciler(client, networkPolicyHandler)
		group.AddToGroup(policyCleaner)
	} else {
		finalizersForRemoval = append(finalizersForRemoval, consts.PolicyCleanerReconcilerFinalizerName)
	}

	if otterizeClient != nil {
		otterizeCloudReconciler := protected_service_reconcilers.NewCloudReconciler(client, scheme, otterizeClient)
		group.AddToGroup(otterizeCloudReconciler)
	} else {
		finalizersForRemoval = append(finalizersForRemoval, consts.CloudReconcilerFinalizerName)
	}

	return &ProtectedServiceReconciler{
		Client:               client,
		group:                group,
		finalizersForRemoval: finalizersForRemoval,
	}
}

func (r *ProtectedServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	err := r.initOnce.Do(func() error {
		return r.protectedServicesReconcilerInit(ctx)
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	return r.group.Reconcile(ctx, req)
}

func (r *ProtectedServiceReconciler) protectedServicesReconcilerInit(ctx context.Context) error {
	var protectedServices otterizev1alpha2.ProtectedServiceList
	err := r.List(ctx, &protectedServices)
	if err != nil {
		return err
	}

	for _, protectedService := range protectedServices.Items {
		for _, finalizer := range r.finalizersForRemoval {
			controllerutil.RemoveFinalizer(&protectedService, finalizer)
		}
		err = r.Update(ctx, &protectedService)
		if err != nil {
			return err
		}
	}
	return nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *ProtectedServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&otterizev1alpha2.ProtectedService{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
	if err != nil {
		return err
	}

	r.group.InjectRecorder(mgr.GetEventRecorderFor(protectedServicesGroupName))
	return nil
}
