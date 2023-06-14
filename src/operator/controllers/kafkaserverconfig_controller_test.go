package controllers

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang/mock/gomock"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	kafkaaclsmocks "github.com/otterize/intents-operator/src/operator/controllers/kafkaacls/mocks"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/mocks"
	serviceidresolvermocks "github.com/otterize/intents-operator/src/shared/serviceidresolver/mocks"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"testing"
	"time"
)

const (
	testNamespace                     = "test-namespace"
	operatorPodNamespace              = "otterize-system"
	kafkaServiceName           string = "kafka"
	kafkaTopicName             string = "test-topic"
	clientName                 string = "test-client"
	intentsObjectName          string = "test-client-intents"
	usernameMapping            string = "user-name-mapping-test"
	operatorPodName            string = "operator-pod-name"
	operatorPodNamespacePrefix string = "otterize"
)

type KafkaServerConfigReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	mockServiceResolver *serviceidresolvermocks.MockServiceResolver
	operatorNamespace   string
	reconciler          *KafkaServerConfigReconciler
	mockCloudClient     *otterizecloudmocks.MockCloudClient
	mockIntentsAdmin    *kafkaaclsmocks.MockKafkaIntentsAdmin
	scheme              *runtime.Scheme
}

func (s *KafkaServerConfigReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.mockCloudClient = otterizecloudmocks.NewMockCloudClient(s.Controller)
	s.mockServiceResolver = serviceidresolvermocks.NewMockServiceResolver(s.Controller)
	s.mockIntentsAdmin = kafkaaclsmocks.NewMockKafkaIntentsAdmin(s.Controller)
	kafkaServersStore := s.setupServerStore(kafkaServiceName)

	s.scheme = runtime.NewScheme()
	s.reconciler = NewKafkaServerConfigReconciler(
		s.Client,
		s.scheme,
		kafkaServersStore,
		operatorPodName,
		operatorPodNamespace,
		s.mockCloudClient,
		s.mockServiceResolver,
	)

	s.reconciler.InjectRecorder(s.Recorder)
}

func (s *KafkaServerConfigReconcilerTestSuite) TearDownTest() {
	s.reconciler = nil
	s.mockCloudClient = nil
	s.mockServiceResolver = nil
	s.mockIntentsAdmin = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *KafkaServerConfigReconcilerTestSuite) setupServerStore(serviceName string) kafkaacls.ServersStore {
	serverConfig := &otterizev1alpha2.KafkaServerConfig{
		Spec: otterizev1alpha2.KafkaServerConfigSpec{
			Service: otterizev1alpha2.Service{
				Name: serviceName,
			},
			Topics: []otterizev1alpha2.TopicConfig{{
				Topic:                  "*",
				Pattern:                otterizev1alpha2.ResourcePatternTypePrefix,
				ClientIdentityRequired: false,
				IntentsRequired:        false,
			},
			},
		},
	}

	serverConfig.SetNamespace(testNamespace)
	emptyTls := otterizev1alpha2.TLSSource{}
	factory := getMockIntentsAdminFactory(s.mockIntentsAdmin)
	kafkaServersStore := kafkaacls.NewServersStore(emptyTls, false, factory, true)
	kafkaServersStore.Add(serverConfig)
	return kafkaServersStore
}

func getMockIntentsAdminFactory(mockIntentsAdmin *kafkaaclsmocks.MockKafkaIntentsAdmin) kafkaacls.IntentsAdminFactoryFunction {
	return func(kafkaServer otterizev1alpha2.KafkaServerConfig, _ otterizev1alpha2.TLSSource, enableKafkaACLCreation bool, enforcementEnabledGlobally bool) (kafkaacls.KafkaIntentsAdmin, error) {
		return mockIntentsAdmin, nil
	}
}

func (s *KafkaServerConfigReconcilerTestSuite) generateKafkaServerConfig() otterizev1alpha2.KafkaServerConfig {
	return otterizev1alpha2.KafkaServerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kafkaServiceName,
			Namespace: testNamespace,
		},
		Spec: otterizev1alpha2.KafkaServerConfigSpec{
			NoAutoCreateIntentsForOperator: true,
			Service: otterizev1alpha2.Service{
				Name: kafkaServiceName,
			},
			Topics: []otterizev1alpha2.TopicConfig{
				{
					Topic:                  kafkaTopicName,
					Pattern:                otterizev1alpha2.ResourcePatternTypeLiteral,
					ClientIdentityRequired: true,
					IntentsRequired:        true,
				},
			},
		},
	}
}

func (s *KafkaServerConfigReconcilerTestSuite) TestKafkaServerConfigUpload() {
	// Create kafka server config resource
	kafkaServerConfig := s.generateKafkaServerConfig()
	kafkaServerConfig.SetNamespace(testNamespace)
	controllerutil.AddFinalizer(&kafkaServerConfig, finalizerName)

	// Get the resource
	emptyKSC := otterizev1alpha2.KafkaServerConfig{}
	objectName := types.NamespacedName{
		Name:      kafkaServiceName,
		Namespace: testNamespace,
	}
	s.Client.EXPECT().Get(gomock.Any(), objectName, &emptyKSC).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, actualKSC *otterizev1alpha2.KafkaServerConfig, _ ...client.GetOption) error {
			kafkaServerConfig.DeepCopyInto(actualKSC)
			return nil
		})

	// Set go mock expectations
	expectedConfigs := s.getExpectedKafkaServerConfigs(kafkaServerConfig)
	s.mockIntentsAdmin.EXPECT().ApplyServerTopicsConf(kafkaServerConfig.Spec.Topics).Return(nil)
	s.mockIntentsAdmin.EXPECT().Close()

	emptyList := &otterizev1alpha2.KafkaServerConfigList{}
	s.Client.EXPECT().List(gomock.Any(), emptyList, client.InNamespace(testNamespace), &client.ListOptions{Namespace: testNamespace}).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.KafkaServerConfigList, _ ...client.ListOption) error {
			list.Items = append(list.Items, kafkaServerConfig)
			return nil
		})

	s.mockCloudClient.EXPECT().ReportKafkaServerConfig(gomock.Any(), testNamespace, gomock.Eq(expectedConfigs)).Return(nil)

	res, err := s.reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      kafkaServiceName,
			Namespace: testNamespace,
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)
	s.ExpectEvent(ReasonSuccessfullyAppliedKafkaServerConfig)
}

func (s *KafkaServerConfigReconcilerTestSuite) getExpectedKafkaServerConfigs(kafkaServerConfig otterizev1alpha2.KafkaServerConfig) []graphqlclient.KafkaServerConfigInput {
	ksc, err := kafkaServerConfigCRDToCloudModel(kafkaServerConfig)
	s.Require().NoError(err)

	return []graphqlclient.KafkaServerConfigInput{ksc}
}

func (s *KafkaServerConfigReconcilerTestSuite) TestRequeueKafkaServerConfigOnFailure() {
	// Create kafka server config resource
	kafkaServerConfig := s.generateKafkaServerConfig()
	kafkaServerConfig.SetNamespace(testNamespace)
	controllerutil.AddFinalizer(&kafkaServerConfig, finalizerName)

	// Get the resource
	emptyKSC := otterizev1alpha2.KafkaServerConfig{}
	objectName := types.NamespacedName{
		Name:      kafkaServiceName,
		Namespace: testNamespace,
	}
	s.Client.EXPECT().Get(gomock.Any(), objectName, &emptyKSC).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, actualKSC *otterizev1alpha2.KafkaServerConfig, _ ...client.GetOption) error {
			kafkaServerConfig.DeepCopyInto(actualKSC)
			return nil
		})

	// Set go mock expectations
	expectedConfigs := s.getExpectedKafkaServerConfigs(kafkaServerConfig)
	s.mockIntentsAdmin.EXPECT().ApplyServerTopicsConf(kafkaServerConfig.Spec.Topics).Return(nil)
	s.mockIntentsAdmin.EXPECT().Close()

	emptyList := &otterizev1alpha2.KafkaServerConfigList{}
	s.Client.EXPECT().List(gomock.Any(), emptyList, client.InNamespace(testNamespace), &client.ListOptions{Namespace: testNamespace}).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.KafkaServerConfigList, _ ...client.ListOption) error {
			list.Items = append(list.Items, kafkaServerConfig)
			return nil
		})

	gomock.InOrder(
		s.mockCloudClient.EXPECT().ReportKafkaServerConfig(gomock.Any(), testNamespace, gomock.Eq(expectedConfigs)).Return(errors.New("something bad happened")),
	)

	res, err := s.reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: objectName,
	})

	s.Require().Error(err)
	s.Require().Equal(ctrl.Result{RequeueAfter: time.Minute}, res)

	s.ExpectEvent(ReasonSuccessfullyAppliedKafkaServerConfig)
}

func (s *KafkaServerConfigReconcilerTestSuite) TestKafkaServerConfigDelete() {
	// Return deleted kafka server config resource
	kscWithoutFinializer := s.generateKafkaServerConfig()
	kscWithoutFinializer.SetNamespace(testNamespace)
	kscWithoutFinializer.DeletionTimestamp = &metav1.Time{Time: time.Date(2022, 9, 16, 0, 55, 0, 0, time.UTC)}
	controllerutil.AddFinalizer(&kscWithoutFinializer, finalizerName)
	controllerutil.RemoveFinalizer(&kscWithoutFinializer, finalizerName)

	// Get the resource
	emptyKSC := otterizev1alpha2.KafkaServerConfig{}
	objectName := types.NamespacedName{
		Name:      kafkaServiceName,
		Namespace: testNamespace,
	}
	s.Client.EXPECT().Get(gomock.Any(), objectName, &emptyKSC).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, actualKSC *otterizev1alpha2.KafkaServerConfig, _ ...client.GetOption) error {
			kscWithoutFinializer.DeepCopyInto(actualKSC)
			controllerutil.AddFinalizer(actualKSC, finalizerName)
			return nil
		})

	// Return deleted kafka server config resource for the upload
	emptyList := &otterizev1alpha2.KafkaServerConfigList{}
	s.Client.EXPECT().List(gomock.Any(), emptyList, client.InNamespace(testNamespace), &client.ListOptions{Namespace: testNamespace}).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.KafkaServerConfigList, _ ...client.ListOption) error {
			list.Items = append(list.Items, kscWithoutFinializer)
			return nil
		})

	gomock.InOrder(
		s.mockCloudClient.EXPECT().ReportKafkaServerConfig(gomock.Any(), testNamespace, gomock.Eq([]graphqlclient.KafkaServerConfigInput{})).Return(nil),
	)

	gomock.InOrder(
		s.mockIntentsAdmin.EXPECT().RemoveServerIntents(kscWithoutFinializer.Spec.Topics).Return(nil),
		s.mockIntentsAdmin.EXPECT().Close().Times(1),
		s.Client.EXPECT().Update(gomock.Any(), gomock.Eq(&kscWithoutFinializer)).Return(nil),
	)

	res, err := s.reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      kafkaServiceName,
			Namespace: testNamespace,
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)
}

func (s *KafkaServerConfigReconcilerTestSuite) TestIntentsGeneratedForOperator() {
	operatorServiceName := "intents-operator-service"
	s.mockServiceResolver.EXPECT().GetPodAnnotatedName(gomock.Any(), operatorPodName, operatorPodNamespace).Return(operatorServiceName, true, nil)

	// Create kafka server config resource
	kafkaServerConfig := s.generateKafkaServerConfig()
	kafkaServerConfig.SetNamespace(testNamespace)
	controllerutil.AddFinalizer(&kafkaServerConfig, finalizerName)
	kafkaServerConfig.Spec.NoAutoCreateIntentsForOperator = false

	// Handle operator intents for the new kafka server
	operatorIntentsObjectName := fmt.Sprintf("operator-to-kafkaserverconfig-kafka-namespace-%s", testNamespace)
	operatorIntents := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorIntentsObjectName,
			Namespace: operatorPodNamespace,
		},
		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: operatorServiceName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: fmt.Sprintf("%s.%s", kafkaServiceName, testNamespace),
					Type: otterizev1alpha2.IntentTypeKafka,
					Topics: []otterizev1alpha2.KafkaTopic{{
						Name: "*",
						Operations: []otterizev1alpha2.KafkaOperation{
							otterizev1alpha2.KafkaOperationDescribe,
							otterizev1alpha2.KafkaOperationAlter,
						},
					}},
				},
			},
		},
	}
	emptyIntents := otterizev1alpha2.ClientIntents{}
	operatorIntentsRequest := types.NamespacedName{
		Name:      operatorIntentsObjectName,
		Namespace: operatorPodNamespace,
	}
	s.Client.EXPECT().Get(gomock.Any(), operatorIntentsRequest, &emptyIntents).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, _ ...client.GetOption) error {
			return k8serrors.NewNotFound(schema.GroupResource{}, "client intents")
		})

	s.Client.EXPECT().Create(gomock.Any(), &operatorIntents).Return(nil)

	// Get the resource
	emptyKSC := otterizev1alpha2.KafkaServerConfig{}
	objectName := types.NamespacedName{
		Name:      kafkaServiceName,
		Namespace: testNamespace,
	}

	s.Client.EXPECT().Get(gomock.Any(), objectName, &emptyKSC).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, actualKSC *otterizev1alpha2.KafkaServerConfig, _ ...client.GetOption) error {
			kafkaServerConfig.DeepCopyInto(actualKSC)
			return nil
		})

	// Expect sending the resource for Intents Admin
	expectedConfigs := s.getExpectedKafkaServerConfigs(kafkaServerConfig)
	s.mockIntentsAdmin.EXPECT().ApplyServerTopicsConf(kafkaServerConfig.Spec.Topics).Return(nil)
	s.mockIntentsAdmin.EXPECT().Close()

	// Expect uploading the resource to Cloud
	emptyList := &otterizev1alpha2.KafkaServerConfigList{}
	s.Client.EXPECT().List(gomock.Any(), emptyList, client.InNamespace(testNamespace), &client.ListOptions{Namespace: testNamespace}).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.KafkaServerConfigList, _ ...client.ListOption) error {
			list.Items = append(list.Items, kafkaServerConfig)
			return nil
		})

	s.mockCloudClient.EXPECT().ReportKafkaServerConfig(gomock.Any(), testNamespace, gomock.Eq(expectedConfigs)).Return(nil)

	res, err := s.reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      kafkaServiceName,
			Namespace: testNamespace,
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)
	s.ExpectEvent(ReasonSuccessfullyAppliedKafkaServerConfig)
}

func (s *KafkaServerConfigReconcilerTestSuite) TestUpdateIntentsGeneratedForOperator() {
	operatorServiceName := "intents-operator-service"
	s.mockServiceResolver.EXPECT().GetPodAnnotatedName(gomock.Any(), operatorPodName, operatorPodNamespace).Return(operatorServiceName, true, nil)

	// Create kafka server config resource
	kafkaServerConfig := s.generateKafkaServerConfig()
	kafkaServerConfig.SetNamespace(testNamespace)
	controllerutil.AddFinalizer(&kafkaServerConfig, finalizerName)
	kafkaServerConfig.Spec.NoAutoCreateIntentsForOperator = false

	// Set operator intents for the updated kafka server
	operatorIntentsObjectName := fmt.Sprintf("operator-to-kafkaserverconfig-kafka-namespace-%s", testNamespace)
	existingOperatorIntents := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorIntentsObjectName,
			Namespace: operatorPodNamespace,
		},
		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: operatorServiceName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: "old-kafka-server.another-namespace",
					Type: otterizev1alpha2.IntentTypeKafka,
					Topics: []otterizev1alpha2.KafkaTopic{{
						Name: "*",
						Operations: []otterizev1alpha2.KafkaOperation{
							otterizev1alpha2.KafkaOperationDescribe,
							otterizev1alpha2.KafkaOperationAlter,
						},
					}},
				},
			},
		},
	}

	updatedOperatorIntents := otterizev1alpha2.ClientIntents{}
	existingOperatorIntents.DeepCopyInto(&updatedOperatorIntents)
	updatedOperatorIntents.Spec.Calls = []otterizev1alpha2.Intent{{
		Name: fmt.Sprintf("%s.%s", kafkaServiceName, testNamespace),
		Type: otterizev1alpha2.IntentTypeKafka,
		Topics: []otterizev1alpha2.KafkaTopic{{
			Name: "*",
			Operations: []otterizev1alpha2.KafkaOperation{
				otterizev1alpha2.KafkaOperationDescribe,
				otterizev1alpha2.KafkaOperationAlter,
			},
		}},
	}}

	emptyIntents := otterizev1alpha2.ClientIntents{}
	operatorIntentsRequest := types.NamespacedName{
		Name:      operatorIntentsObjectName,
		Namespace: operatorPodNamespace,
	}
	s.Client.EXPECT().Get(gomock.Any(), operatorIntentsRequest, &emptyIntents).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, _ ...client.GetOption) error {
			existingOperatorIntents.DeepCopyInto(intents)
			return nil
		})

	s.Client.EXPECT().Patch(gomock.Any(), gomock.Eq(&updatedOperatorIntents), intents_reconcilers.MatchPatch(client.MergeFrom(&existingOperatorIntents))).Return(nil)

	// Get the resource
	emptyKSC := otterizev1alpha2.KafkaServerConfig{}
	objectName := types.NamespacedName{
		Name:      kafkaServiceName,
		Namespace: testNamespace,
	}

	s.Client.EXPECT().Get(gomock.Any(), objectName, &emptyKSC).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, actualKSC *otterizev1alpha2.KafkaServerConfig, _ ...client.GetOption) error {
			kafkaServerConfig.DeepCopyInto(actualKSC)
			return nil
		})

	// Expect sending the resource for Intents Admin
	expectedConfigs := s.getExpectedKafkaServerConfigs(kafkaServerConfig)
	s.mockIntentsAdmin.EXPECT().ApplyServerTopicsConf(kafkaServerConfig.Spec.Topics).Return(nil)
	s.mockIntentsAdmin.EXPECT().Close()

	// Expect uploading the resource to Cloud
	emptyList := &otterizev1alpha2.KafkaServerConfigList{}
	s.Client.EXPECT().List(gomock.Any(), emptyList, client.InNamespace(testNamespace), &client.ListOptions{Namespace: testNamespace}).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.KafkaServerConfigList, _ ...client.ListOption) error {
			list.Items = append(list.Items, kafkaServerConfig)
			return nil
		})

	s.mockCloudClient.EXPECT().ReportKafkaServerConfig(gomock.Any(), testNamespace, gomock.Eq(expectedConfigs)).Return(nil)

	res, err := s.reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      kafkaServiceName,
			Namespace: testNamespace,
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)
	s.ExpectEvent(ReasonSuccessfullyAppliedKafkaServerConfig)
}

func TestKafkaACLReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(KafkaServerConfigReconcilerTestSuite))
}
