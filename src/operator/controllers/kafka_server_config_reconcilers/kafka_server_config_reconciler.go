package kafka_server_config_reconcilers

import (
	"context"
	"errors"
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetrysender"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
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
	ReasonSuccessfullyAppliedKafkaServerConfig = "SuccessfullyAppliedKafkaServerConfig"
)

// KafkaServerConfigReconciler reconciles a KafkaServerConfig object
type KafkaServerConfigReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	ServersStore         kafkaacls.ServersStore
	operatorPodName      string
	operatorPodNamespace string
	otterizeClient       operator_cloud_client.CloudClient
	injectablerecorder.InjectableRecorder
	serviceResolver serviceidresolver.ServiceResolver
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
	return &KafkaServerConfigReconciler{
		Client:               client,
		Scheme:               scheme,
		ServersStore:         serversStore,
		operatorPodName:      operatorPodName,
		operatorPodNamespace: operatorPodNameSpace,
		otterizeClient:       cloudClient,
		serviceResolver:      serviceResolver,
	}
}

//+kubebuilder:rbac:groups=k8s.otterize.com,resources=kafkaserverconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=clientintents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=kafkaserverconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=kafkaserverconfigs/finalizers,verbs=update

func (r *KafkaServerConfigReconciler) removeKafkaServerFromStore(kafkaServerConfig *otterizev1alpha3.KafkaServerConfig) error {
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
	if err := intentsAdmin.RemoveServerIntents(kafkaServerConfig.Spec.Topics); err != nil {
		return err
	}

	logger.Info("Removing Kafka server from store")
	r.ServersStore.Remove(kafkaServerConfig.Spec.Service.Name, kafkaServerConfig.Namespace)
	return nil
}

func (r *KafkaServerConfigReconciler) ensureFinalizerRunForOperatorIntents(ctx context.Context, config *otterizev1alpha3.KafkaServerConfig) error {
	operatorPod := &v1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Name: r.operatorPodName, Namespace: r.operatorPodNamespace}, operatorPod)
	if err != nil {
		return err
	}
	operatorIntentsName := FormatIntentsName(config)
	intents := &otterizev1alpha3.ClientIntents{}
	err = r.Get(ctx, types.NamespacedName{Name: operatorIntentsName, Namespace: operatorPod.Namespace}, intents)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return r.Delete(ctx, intents)
}

func (r *KafkaServerConfigReconciler) ensureFinalizerRun(ctx context.Context, kafkaServerConfig *otterizev1alpha3.KafkaServerConfig) (ctrl.Result, error) {
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

	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeKafkaServerConfigDeleted, 1)

	return ctrl.Result{}, nil
}

func (r *KafkaServerConfigReconciler) ensureFinalizerRegistered(
	ctx context.Context, kafkaServerConfig *otterizev1alpha3.KafkaServerConfig) error {
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

func (r *KafkaServerConfigReconciler) createIntentsFromOperatorToKafkaServer(ctx context.Context, config *otterizev1alpha3.KafkaServerConfig) error {
	annotatedServiceName, ok, err := r.serviceResolver.GetPodAnnotatedName(ctx, r.operatorPodName, r.operatorPodNamespace)
	if err != nil {
		return err
	}

	if !ok {
		r.RecordWarningEventf(config, ReasonIntentsOperatorIdentityResolveFailed, "failed resolving intents operator identity - service name annotation required")
		return fmt.Errorf("failed resolving intents operator identity - service name annotation required")
	}

	newIntents := &otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      FormatIntentsName(config),
			Namespace: r.operatorPodNamespace,
		},
		Spec: &otterizev1alpha3.IntentsSpec{
			Service: otterizev1alpha3.Service{
				Name: annotatedServiceName,
			},
			Calls: []otterizev1alpha3.Intent{{
				Type: otterizev1alpha3.IntentTypeKafka,
				Name: fmt.Sprintf("%s.%s", config.Spec.Service.Name, config.Namespace),
				Topics: []otterizev1alpha3.KafkaTopic{{
					Name: "*",
					Operations: []otterizev1alpha3.KafkaOperation{
						otterizev1alpha3.KafkaOperationDescribe,
						otterizev1alpha3.KafkaOperationAlter,
					},
				}},
			}},
		},
	}

	existingIntents := &otterizev1alpha3.ClientIntents{}
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

	kafkaServerConfig := &otterizev1alpha3.KafkaServerConfig{}

	err := r.Get(ctx, req.NamespacedName, kafkaServerConfig)
	if err != nil && k8serrors.IsNotFound(err) {
		logger.Info("No kafka server config found")
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	result, err := r.reconcileObject(ctx, kafkaServerConfig)
	if err != nil {
		return result, err
	}

	if err := r.uploadKafkaServerConfigs(ctx, req.Namespace); err != nil {
		logrus.WithError(err).Error("failed to upload KafkaServerConfig to cloud")
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	return ctrl.Result{}, nil
}

func (r *KafkaServerConfigReconciler) reconcileObject(ctx context.Context, kafkaServerConfig *otterizev1alpha3.KafkaServerConfig) (ctrl.Result, error) {
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

	r.RecordNormalEvent(kafkaServerConfig, ReasonSuccessfullyAppliedKafkaServerConfig, "successfully applied server config")
	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeKafkaServerConfigApplied, len(kafkaServerConfig.Spec.Topics))
	return ctrl.Result{}, nil
}

func (r *KafkaServerConfigReconciler) uploadKafkaServerConfigs(ctx context.Context, namespace string) error {
	if r.otterizeClient == nil {
		return nil
	}

	kafkaServerConfigs := &otterizev1alpha3.KafkaServerConfigList{}
	err := r.List(ctx, kafkaServerConfigs, client.InNamespace(namespace), &client.ListOptions{Namespace: namespace})
	if err != nil {
		return err
	}

	inputs := make([]graphqlclient.KafkaServerConfigInput, 0)
	for _, kafkaServerConfig := range kafkaServerConfigs.Items {
		if kafkaServerConfig.DeletionTimestamp != nil {
			continue
		}
		input, err := kafkaServerConfigCRDToCloudModel(kafkaServerConfig)
		if err != nil {
			return err
		}

		inputs = append(inputs, input)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, viper.GetDuration(otterizecloudclient.CloudClientTimeoutKey))
	defer cancel()

	return r.otterizeClient.ReportKafkaServerConfig(timeoutCtx, namespace, inputs)
}

func kafkaServerConfigCRDToCloudModel(kafkaServerConfig otterizev1alpha3.KafkaServerConfig) (graphqlclient.KafkaServerConfigInput, error) {
	topics := make([]graphqlclient.KafkaTopicInput, 0)
	for _, topic := range kafkaServerConfig.Spec.Topics {
		pattern, err := crdPatternToCloudPattern(topic.Pattern)
		if err != nil {
			return graphqlclient.KafkaServerConfigInput{}, err
		}

		topics = append(topics, graphqlclient.KafkaTopicInput{
			ClientIdentityRequired: topic.ClientIdentityRequired,
			IntentsRequired:        topic.IntentsRequired,
			Pattern:                pattern,
			Topic:                  topic.Topic,
		})
	}

	input := graphqlclient.KafkaServerConfigInput{
		Name:      kafkaServerConfig.Spec.Service.Name,
		Namespace: kafkaServerConfig.Namespace,
		Address:   kafkaServerConfig.Spec.Addr,
		Topics:    topics,
	}

	return input, nil
}

func crdPatternToCloudPattern(pattern otterizev1alpha3.ResourcePatternType) (graphqlclient.KafkaTopicPattern, error) {
	var result graphqlclient.KafkaTopicPattern
	switch pattern {
	case otterizev1alpha3.ResourcePatternTypePrefix:
		result = graphqlclient.KafkaTopicPatternPrefix
	case otterizev1alpha3.ResourcePatternTypeLiteral:
		result = graphqlclient.KafkaTopicPatternLiteral
	default:
		return "", fmt.Errorf("unknown pattern type: %s", pattern)
	}

	return result, nil
}

func FormatIntentsName(conf *otterizev1alpha3.KafkaServerConfig) string {
	return fmt.Sprintf("operator-to-kafkaserverconfig-%s-namespace-%s", conf.Name, conf.Namespace)
}
