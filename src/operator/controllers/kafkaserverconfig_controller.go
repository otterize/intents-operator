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
	"errors"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	otterizev1alpha1 "github.com/otterize/intents-operator/src/shared/api/v1alpha1"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	finalizerName = "kafkaserverconfig.otterize/finalizer"
)

// KafkaServerConfigReconciler reconciles a KafkaServerConfig object
type KafkaServerConfigReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	ServersStore *kafkaacls.ServersStore
}

//+kubebuilder:rbac:groups=otterize.com,resources=kafkaserverconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=otterize.com,resources=kafkaserverconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=otterize.com,resources=kafkaserverconfigs/finalizers,verbs=update

func (r *KafkaServerConfigReconciler) removeKafkaServerFromStore(kafkaServerConfig *otterizev1alpha1.KafkaServerConfig) error {
	logger := logrus.WithFields(
		logrus.Fields{
			"name":      kafkaServerConfig.Name,
			"namespace": kafkaServerConfig.Namespace,
		},
	)

	intentsAdmin, err := r.ServersStore.Get(kafkaServerConfig.Spec.ServerName, kafkaServerConfig.Namespace)
	if err != nil && errors.Is(err, kafkaacls.ServerSpecNotFound) {
		logger.Info("Kafka server not registered to servers store")
		return nil
	} else if err != nil {
		return err
	}

	defer intentsAdmin.Close()

	logger.Info("Removing associated ACLs")
	if err := intentsAdmin.RemoveAllIntents(); err != nil {
		return err
	}

	logger.Info("Removing Kafka server from store")
	r.ServersStore.Remove(kafkaServerConfig.Spec.ServerName, kafkaServerConfig.Namespace)
	return nil
}

func (r *KafkaServerConfigReconciler) ensureFinalizerRun(ctx context.Context, kafkaServerConfig *otterizev1alpha1.KafkaServerConfig) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(kafkaServerConfig, finalizerName) {
		return ctrl.Result{}, nil
	}

	if err := r.removeKafkaServerFromStore(kafkaServerConfig); err != nil {
		return ctrl.Result{}, err
	}

	controllerutil.RemoveFinalizer(kafkaServerConfig, finalizerName)
	if err := r.Update(ctx, kafkaServerConfig); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *KafkaServerConfigReconciler) ensureFinalizerRegistered(ctx context.Context, kafkaServerConfig *otterizev1alpha1.KafkaServerConfig) error {
	logger := logrus.WithFields(
		logrus.Fields{
			"name":      kafkaServerConfig.Name,
			"namespace": kafkaServerConfig.Namespace,
		},
	)

	if controllerutil.ContainsFinalizer(kafkaServerConfig, finalizerName) {
		return nil
	}

	logger.Infof("Adding finalizer %s", finalizerName)
	controllerutil.AddFinalizer(kafkaServerConfig, finalizerName)
	if err := r.Update(ctx, kafkaServerConfig); err != nil {
		return err
	}

	return nil
}

func (r *KafkaServerConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logrus.WithField("namespaced_name", req.NamespacedName.String())

	kafkaServerConfig := &otterizev1alpha1.KafkaServerConfig{}

	err := r.Get(ctx, req.NamespacedName, kafkaServerConfig)
	if err != nil && k8serrors.IsNotFound(err) {
		logger.Info("No kafka server config found")
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	if !kafkaServerConfig.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is being deleted
		return r.ensureFinalizerRun(ctx, kafkaServerConfig)
	}

	if err := r.ensureFinalizerRegistered(ctx, kafkaServerConfig); err != nil {
		return ctrl.Result{}, err
	}

	r.ServersStore.Add(kafkaServerConfig)

	kafkaIntentsAdmin, err := r.ServersStore.Get(kafkaServerConfig.Spec.ServerName, kafkaServerConfig.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer kafkaIntentsAdmin.Close()

	if err := kafkaIntentsAdmin.ApplyServerTopicsConf(kafkaServerConfig.Spec.Topics); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KafkaServerConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&otterizev1alpha1.KafkaServerConfig{}).
		Complete(r)
}
