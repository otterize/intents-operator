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
	otterizev2 "github.com/otterize/intents-operator/src/operator/api/v2"
	"github.com/otterize/intents-operator/src/operator/controllers/kafka_server_config_reconcilers"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/otterize/intents-operator/src/shared/reconcilergroup"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesconfig"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	finalizerName = "intents.otterize.com/kafkaserverconfig-finalizer"
	groupName     = "kafka-server-config-reconciler"
)

// KafkaServerConfigReconciler reconciles a KafkaServerConfig object
type KafkaServerConfigReconciler struct {
	client.Client
	injectablerecorder.InjectableRecorder
	group *reconcilergroup.Group
}

func NewKafkaServerConfigReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	serversStore kafkaacls.ServersStore,
	operatorPodName string,
	operatorPodNameSpace string,
	cloudClient operator_cloud_client.CloudClient,
	serviceResolver serviceidresolver.ServiceResolver,
) *KafkaServerConfigReconciler {
	kscReconciler := kafka_server_config_reconcilers.NewKafkaServerConfigReconciler(
		client,
		scheme,
		serversStore,
		operatorPodName,
		operatorPodNameSpace,
		cloudClient,
		serviceResolver,
	)

	group := reconcilergroup.NewGroup(
		groupName,
		client,
		scheme,
		&otterizev2.KafkaServerConfig{},
		finalizerName,
		nil,
		kscReconciler,
	)

	if telemetriesconfig.IsUsageTelemetryEnabled() {
		telemetryReconciler := kafka_server_config_reconcilers.NewTelemetryReconciler(client)
		group.AddToGroup(telemetryReconciler)
	}

	return &KafkaServerConfigReconciler{
		Client: client,
		group:  group,
	}
}

func (r *KafkaServerConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.group.Reconcile(ctx, req)
}

// SetupWithManager sets up the controller with the Manager.
func (r *KafkaServerConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&otterizev2.KafkaServerConfig{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Watches(&otterizev2.ProtectedService{}, handler.EnqueueRequestsFromMapFunc(r.mapProtectedServiceToKafkaServerConfig)).
		Complete(r)
	if err != nil {
		return errors.Wrap(err)
	}

	r.group.InjectRecorder(mgr.GetEventRecorderFor(groupName))

	return nil
}

func (r *KafkaServerConfigReconciler) InitKafkaServerConfigIndices(mgr ctrl.Manager) error {
	return mgr.GetCache().IndexField(
		context.Background(),
		&otterizev2.KafkaServerConfig{},
		otterizev2.OtterizeKafkaServerConfigServiceNameField,
		func(object client.Object) []string {
			ksc := object.(*otterizev2.KafkaServerConfig)
			return []string{ksc.Spec.Service.Name}
		})
}

func (r *KafkaServerConfigReconciler) mapProtectedServiceToKafkaServerConfig(ctx context.Context, obj client.Object) []reconcile.Request {
	protectedService := obj.(*otterizev2.ProtectedService)
	logrus.Debugf("Enqueueing KafkaServerConfigs for protected service %s", protectedService.Name)

	kscsToReconcile := r.getKSCsForProtectedService(ctx, protectedService)
	return lo.Map(kscsToReconcile, func(ksc otterizev2.KafkaServerConfig, _ int) reconcile.Request {
		return reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      ksc.Name,
				Namespace: ksc.Namespace,
			},
		}
	})
}

func (r *KafkaServerConfigReconciler) getKSCsForProtectedService(ctx context.Context, protectedService *otterizev2.ProtectedService) []otterizev2.KafkaServerConfig {
	kscsToReconcile := make([]otterizev2.KafkaServerConfig, 0)
	var kafkaServerConfigs otterizev2.KafkaServerConfigList
	err := r.Client.List(ctx,
		&kafkaServerConfigs,
		&client.MatchingFields{otterizev2.OtterizeKafkaServerConfigServiceNameField: protectedService.Spec.Name},
		&client.ListOptions{Namespace: protectedService.Namespace},
	)
	if err != nil {
		logrus.Errorf("Failed to list KSCs for server %s: %v", protectedService.Spec.Name, err)
		// Intentionally no return - we are not able to return errors in this flow currently
	}

	kscsToReconcile = append(kscsToReconcile, kafkaServerConfigs.Items...)
	return kscsToReconcile
}
