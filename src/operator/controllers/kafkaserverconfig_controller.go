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
	"fmt"
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	finalizerName = "intents.otterize.com/kafkaserverconfig-finalizer"
)

// KafkaServerConfigReconciler reconciles a KafkaServerConfig object
type KafkaServerConfigReconciler struct {
	client               client.Client
	Scheme               *runtime.Scheme
	ServersStore         *kafkaacls.ServersStore
	operatorPodName      string
	operatorPodNamespace string
	injectablerecorder.InjectableRecorder
}

func NewKafkaServerConfigReconciler(client client.Client, scheme *runtime.Scheme, serversStore *kafkaacls.ServersStore,
	operatorPodName string, operatorPodNameSpace string) *KafkaServerConfigReconciler {
	return &KafkaServerConfigReconciler{
		client:               client,
		Scheme:               scheme,
		ServersStore:         serversStore,
		operatorPodName:      operatorPodName,
		operatorPodNamespace: operatorPodNameSpace,
	}
}

//+kubebuilder:rbac:groups=k8s.otterize.com,resources=kafkaserverconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=clientintents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=kafkaserverconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=kafkaserverconfigs/finalizers,verbs=update

func (r *KafkaServerConfigReconciler) removeKafkaServerFromStore(kafkaServerConfig *otterizev1alpha1.KafkaServerConfig) error {
	logger := logrus.WithFields(
		logrus.Fields{
			"name":      kafkaServerConfig.Name,
			"namespace": kafkaServerConfig.Namespace,
		},
	)

	intentsAdmin, err := r.ServersStore.Get(kafkaServerConfig.Spec.Service.Name, kafkaServerConfig.Namespace)
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
	r.ServersStore.Remove(kafkaServerConfig.Spec.Service.Name, kafkaServerConfig.Namespace)
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
	if err := r.client.Update(ctx, kafkaServerConfig); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *KafkaServerConfigReconciler) ensureFinalizerRegistered(
	ctx context.Context, kafkaServerConfig *otterizev1alpha1.KafkaServerConfig) error {
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
	if err := r.client.Update(ctx, kafkaServerConfig); err != nil {
		return err
	}

	return nil
}

func (r *KafkaServerConfigReconciler) createAutomaticIntentsToKafkaServer(ctx context.Context, config *otterizev1alpha1.KafkaServerConfig) error {
	operatorPod := &v1.Pod{}
	err := r.client.Get(ctx, types.NamespacedName{Name: r.operatorPodName, Namespace: r.operatorPodNamespace}, operatorPod)
	if err != nil {
		return err
	}

	annotatedServiceName, ok := operatorPod.Annotations[serviceidresolver.ServiceNameAnnotation]
	if !ok {
		r.RecordWarningEventf(config, "IntentsOperatorIdentityResolveFailed", "failed resolving intents operator identity - '%s' label required", annotatedServiceName)
		return fmt.Errorf("failed resolving intents operator identity - '%s' label required", annotatedServiceName)
	}

	newIntents := &otterizev1alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.Name,
			Namespace: operatorPod.Namespace,
		},
		Spec: &otterizev1alpha1.IntentsSpec{
			Service: otterizev1alpha1.Service{
				Name: annotatedServiceName,
			},
			Calls: []otterizev1alpha1.Intent{{
				Type:      otterizev1alpha1.IntentTypeHTTP,
				Name:      config.Spec.Service.Name,
				Namespace: config.Namespace,
			}},
		},
	}

	existingIntents := &otterizev1alpha1.ClientIntents{}
	err = r.client.Get(ctx, types.NamespacedName{Name: config.Name, Namespace: config.Namespace}, existingIntents)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			err := r.client.Create(ctx, newIntents)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		intentsCopy := existingIntents.DeepCopy()
		intentsCopy.Spec = newIntents.Spec
		intentsCopy.SetOwnerReferences(newIntents.GetOwnerReferences())
		err := r.client.Patch(ctx, intentsCopy, client.MergeFrom(existingIntents))
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *KafkaServerConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logrus.WithField("namespaced_name", req.NamespacedName.String())

	kafkaServerConfig := &otterizev1alpha1.KafkaServerConfig{}

	err := r.client.Get(ctx, req.NamespacedName, kafkaServerConfig)
	if err != nil && k8serrors.IsNotFound(err) {
		logger.Info("No kafka server config found")
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	if !kafkaServerConfig.Spec.NoAutoCreateIntentsForOperator {
		err := r.createAutomaticIntentsToKafkaServer(ctx, kafkaServerConfig)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if !kafkaServerConfig.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is being deleted
		return r.ensureFinalizerRun(ctx, kafkaServerConfig)
	}

	if err := r.ensureFinalizerRegistered(ctx, kafkaServerConfig); err != nil {
		return ctrl.Result{}, err
	}

	r.ServersStore.Add(kafkaServerConfig)

	kafkaIntentsAdmin, err := r.ServersStore.Get(kafkaServerConfig.Spec.Service.Name, kafkaServerConfig.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer kafkaIntentsAdmin.Close()

	if err := kafkaIntentsAdmin.ApplyServerTopicsConf(kafkaServerConfig.Spec.Topics); err != nil {
		r.RecordWarningEvent(kafkaServerConfig, "failed to apply server config to Kafka broker", err.Error())
		return ctrl.Result{}, err
	}

	r.RecordNormalEvent(kafkaServerConfig, "successfully applied server config", "")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KafkaServerConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&otterizev1alpha1.KafkaServerConfig{}).
		Complete(r)
	if err != nil {
		return err
	}

	r.InjectRecorder(mgr.GetEventRecorderFor("intents-operator"))
	return nil
}
