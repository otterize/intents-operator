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
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/otterizecloud"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"time"
)

const (
	finalizerName                              = "intents.otterize.com/kafkaserverconfig-finalizer"
	ReasonIntentsOperatorIdentityResolveFailed = "IntentsOperatorIdentityResolveFailed"
	ReasonApplyingKafkaServerConfigFailed      = "ApplyingKafkaServerConfigFailed"
	ReasonAppliedKafkaServerConfigFailed       = "AppliedKafkaServerConfigFailed"
	IntentsOperatorSource                      = "intents-operator"
)

// KafkaServerConfigReconciler reconciles a KafkaServerConfig object
type KafkaServerConfigReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	ServersStore         kafkaacls.ServersStore
	operatorPodName      string
	operatorPodNamespace string
	otterizeClient       otterizecloud.CloudClient
	injectablerecorder.InjectableRecorder
}

func NewKafkaServerConfigReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	serversStore kafkaacls.ServersStore,
	operatorPodName string,
	operatorPodNameSpace string,
	cloudClient otterizecloud.CloudClient,
) *KafkaServerConfigReconciler {
	return &KafkaServerConfigReconciler{
		Client:               client,
		Scheme:               scheme,
		ServersStore:         serversStore,
		operatorPodName:      operatorPodName,
		operatorPodNamespace: operatorPodNameSpace,
		otterizeClient:       cloudClient,
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

func (r *KafkaServerConfigReconciler) ensureFinalizerRunForOperatorIntents(ctx context.Context, config *otterizev1alpha1.KafkaServerConfig) error {
	operatorPod := &v1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Name: r.operatorPodName, Namespace: r.operatorPodNamespace}, operatorPod)
	if err != nil {
		return err
	}
	operatorIntentsName := formatIntentsName(config)
	intents := &otterizev1alpha1.ClientIntents{}
	err = r.Get(ctx, types.NamespacedName{Name: operatorIntentsName, Namespace: operatorPod.Namespace}, intents)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return r.Delete(ctx, intents)
}

func (r *KafkaServerConfigReconciler) ensureFinalizerRun(ctx context.Context, kafkaServerConfig *otterizev1alpha1.KafkaServerConfig) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(kafkaServerConfig, finalizerName) {
		return ctrl.Result{}, nil
	}

	if err := r.removeKafkaServerFromStore(kafkaServerConfig); err != nil {
		return ctrl.Result{}, err
	}

	if !kafkaServerConfig.Spec.NoAutoCreateIntentsForOperator {
		err := r.ensureFinalizerRunForOperatorIntents(ctx, kafkaServerConfig)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	controllerutil.RemoveFinalizer(kafkaServerConfig, finalizerName)
	if err := r.Update(ctx, kafkaServerConfig); err != nil {
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
	if err := r.Update(ctx, kafkaServerConfig); err != nil {
		return err
	}

	return nil
}

func (r *KafkaServerConfigReconciler) createIntentsFromOperatorToKafkaServer(ctx context.Context, config *otterizev1alpha1.KafkaServerConfig) error {
	operatorPod := &v1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Name: r.operatorPodName, Namespace: r.operatorPodNamespace}, operatorPod)
	if err != nil {
		return err
	}

	annotatedServiceName, ok := serviceidresolver.ResolvePodToServiceIdentityUsingAnnotationOnly(operatorPod)
	if !ok {
		r.RecordWarningEventf(config, ReasonIntentsOperatorIdentityResolveFailed, "failed resolving intents operator identity - service name annotation required")
		return fmt.Errorf("failed resolving intents operator identity - service name annotation required")
	}

	newIntents := &otterizev1alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      formatIntentsName(config),
			Namespace: operatorPod.Namespace,
		},
		Spec: &otterizev1alpha1.IntentsSpec{
			Service: otterizev1alpha1.Service{
				Name: annotatedServiceName,
			},
			Calls: []otterizev1alpha1.Intent{{
				// HTTP is used here to indicate that this should only apply network policies.
				// In the future, should be updated as declaring type HTTP becomes unnecessary.
				Type:      otterizev1alpha1.IntentTypeHTTP,
				Name:      config.Spec.Service.Name,
				Namespace: config.Namespace,
			}},
		},
	}

	existingIntents := &otterizev1alpha1.ClientIntents{}
	err = r.Get(ctx, types.NamespacedName{Name: newIntents.Name, Namespace: newIntents.Namespace}, existingIntents)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			err := r.Create(ctx, newIntents)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		intentsCopy := existingIntents.DeepCopy()
		intentsCopy.Spec = newIntents.Spec

		err := r.Patch(ctx, intentsCopy, client.MergeFrom(existingIntents))
		if err != nil {
			return err
		}
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

	if !kafkaServerConfig.Spec.NoAutoCreateIntentsForOperator {
		err := r.createIntentsFromOperatorToKafkaServer(ctx, kafkaServerConfig)
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
		r.RecordWarningEventf(kafkaServerConfig, ReasonApplyingKafkaServerConfigFailed, "failed to apply server config to Kafka broker: %s", err.Error())
		return ctrl.Result{}, err
	}

	if err := r.uploadKafkaServerConfig(ctx, kafkaServerConfig); err != nil {
		logrus.WithError(err).Error("failed to upload KafkaServerConfig to cloud")
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	r.RecordNormalEvent(kafkaServerConfig, ReasonAppliedKafkaServerConfigFailed, "successfully applied server config")
	return ctrl.Result{}, nil
}

func (r *KafkaServerConfigReconciler) uploadKafkaServerConfig(ctx context.Context, kafkaServerConfig *otterizev1alpha1.KafkaServerConfig) error {
	if r.otterizeClient == nil {
		return nil
	}

	err := r.otterizeClient.ReportKubernetesNamespace(ctx, kafkaServerConfig.Namespace)
	if err != nil {
		return err
	}

	input := graphqlclient.KafkaServerConfigInput{
		Name:    kafkaServerConfig.Spec.Service.Name,
		Address: kafkaServerConfig.Spec.Addr,
		Topics: lo.Map(kafkaServerConfig.Spec.Topics, func(topic otterizev1alpha1.TopicConfig, _ int) graphqlclient.KafkaTopicInput {
			return graphqlclient.KafkaTopicInput{
				ClientIdentityRequired: topic.ClientIdentityRequired,
				IntentsRequired:        topic.IntentsRequired,
				Pattern:                string(topic.Pattern),
				Topic:                  topic.Topic,
			}
		}),
	}

	err = r.otterizeClient.ReportKafkaServerConfig(ctx, kafkaServerConfig.Namespace, input)
	if err != nil {
		return err
	}

	return nil
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

func formatIntentsName(conf *otterizev1alpha1.KafkaServerConfig) string {
	return fmt.Sprintf("operator-to-kafkaserverconfig-%s-namespace-%s", conf.Name, conf.Namespace)
}
