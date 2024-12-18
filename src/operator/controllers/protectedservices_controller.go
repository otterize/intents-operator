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
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/protected_service_reconcilers"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/otterize/intents-operator/src/shared/reconcilergroup"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesconfig"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

const (
	protectedServicesGroupName = "protected-services"
)

var protectedServiceLegacyFinalizers = []string{
	"protectedservice.otterize.com/cloudfinalizer",
	"protectedservice.otterize.com/defaultdenyfinalizer",
	"protectedservice.otterize.com/policycleanerfinalizer",
}

// ProtectedServiceReconciler reconciles a ProtectedService object
type ProtectedServiceReconciler struct {
	client.Client
	group *reconcilergroup.Group
}

//+kubebuilder:rbac:groups=k8s.otterize.com,resources=protectedservices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=protectedservices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=protectedservices/finalizers,verbs=update

func NewProtectedServiceReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	otterizeClient operator_cloud_client.CloudClient,
	enforcementDefaultState bool,
	netpolEnforcementEnabled bool,
	effectivePolicySyncer protected_service_reconcilers.EffectivePolicyReconcilerGroup,
) *ProtectedServiceReconciler {
	group := reconcilergroup.NewGroup(
		protectedServicesGroupName,
		client,
		scheme,
		&otterizev2alpha1.ProtectedService{},
		otterizev2alpha1.ProtectedServicesFinalizerName,
		protectedServiceLegacyFinalizers,
	)

	if netpolEnforcementEnabled {
		defaultDenyReconciler := protected_service_reconcilers.NewDefaultDenyReconciler(client, netpolEnforcementEnabled)
		group.AddToGroup(defaultDenyReconciler)
	}

	if !enforcementDefaultState || !netpolEnforcementEnabled {
		policyCleaner := protected_service_reconcilers.NewPolicyCleanerReconciler(client, effectivePolicySyncer)
		group.AddToGroup(policyCleaner)
	}

	if otterizeClient != nil {
		otterizeCloudReconciler := protected_service_reconcilers.NewCloudReconciler(client, scheme, otterizeClient)
		group.AddToGroup(otterizeCloudReconciler)
	}

	if telemetriesconfig.IsUsageTelemetryEnabled() {
		telemetryReconciler := protected_service_reconcilers.NewTelemetryReconciler(client)
		group.AddToGroup(telemetryReconciler)
	}

	return &ProtectedServiceReconciler{
		Client: client,
		group:  group,
	}
}

func (r *ProtectedServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.group.Reconcile(ctx, req)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProtectedServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&otterizev2alpha1.ProtectedService{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
	if err != nil {
		return errors.Wrap(err)
	}

	r.group.InjectRecorder(mgr.GetEventRecorderFor(protectedServicesGroupName))
	return nil
}
