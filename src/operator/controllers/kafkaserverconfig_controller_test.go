package controllers

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang/mock/gomock"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	kafkaaclsmocks "github.com/otterize/intents-operator/src/operator/controllers/kafkaacls/mocks"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/mocks"
	serviceidresolvermocks "github.com/otterize/intents-operator/src/shared/serviceidresolver/mocks"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"testing"
)

const (
	kafkaServiceName           string = "kafka"
	kafkaTopicName             string = "test-topic"
	clientName                 string = "test-client"
	intentsObjectName          string = "test-client-intents"
	usernameMapping            string = "user-name-mapping-test"
	operatorPodName            string = "operator-pod-name"
	operatorPodNamespacePrefix string = "otterize"
)

type KafkaServerConfigReconcilerTestSuite struct {
	testbase.ControllerManagerTestSuiteBase
	reconciler          *KafkaServerConfigReconciler
	mockCloudClient     *otterizecloudmocks.MockCloudClient
	mockIntentsAdmin    *kafkaaclsmocks.MockKafkaIntentsAdmin
	mockServiceResolver *serviceidresolvermocks.MockServiceResolver
	operatorNamespace   string
}

func (s *KafkaServerConfigReconcilerTestSuite) SetupSuite() {
	s.TestEnv = &envtest.Environment{}
	var err error
	s.TestEnv.CRDDirectoryPaths = []string{filepath.Join("..", "config", "crd")}

	s.RestConfig, err = s.TestEnv.Start()
	s.Require().NoError(err)
	s.Require().NotNil(s.RestConfig)

	s.K8sDirectClient, err = kubernetes.NewForConfig(s.RestConfig)
	s.Require().NoError(err)
	s.Require().NotNil(s.K8sDirectClient)

	utilruntime.Must(apiextensionsv1.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(istiosecurityscheme.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(otterizev1alpha2.AddToScheme(s.TestEnv.Scheme))
}

func (s *KafkaServerConfigReconcilerTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()
}

func (s *KafkaServerConfigReconcilerTestSuite) setupServerStore(serviceName string, controller *gomock.Controller) kafkaacls.ServersStore {
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

	serverConfig.SetNamespace(s.TestNamespace)
	emptyTls := otterizev1alpha2.TLSSource{}
	s.mockIntentsAdmin = kafkaaclsmocks.NewMockKafkaIntentsAdmin(controller)
	factory := getMockIntentsAdminFactory(s.mockIntentsAdmin)
	kafkaServersStore := kafkaacls.NewServersStore(emptyTls, false, factory, true)
	kafkaServersStore.Add(serverConfig)
	return kafkaServersStore
}

func (s *KafkaServerConfigReconcilerTestSuite) TearDownSuite() {
	s.ControllerManagerTestSuiteBase.TearDownSuite()
}

func (s *KafkaServerConfigReconcilerTestSuite) BeforeTest(_, testName string) {
	s.ControllerManagerTestSuiteBase.BeforeTest("", testName)
	controller := gomock.NewController(s.T())
	kafkaServersStore := s.setupServerStore(kafkaServiceName, controller)
	s.mockCloudClient = otterizecloudmocks.NewMockCloudClient(controller)

	s.mockServiceResolver = serviceidresolvermocks.NewMockServiceResolver(controller)

	s.reconciler = NewKafkaServerConfigReconciler(
		s.Mgr.GetClient(),
		s.TestEnv.Scheme,
		kafkaServersStore,
		operatorPodName,
		s.operatorNamespace,
		s.mockCloudClient,
		s.mockServiceResolver,
	)

	recorder := s.Mgr.GetEventRecorderFor("intents-operator")
	s.reconciler.InjectRecorder(recorder)
}

func (s *KafkaServerConfigReconcilerTestSuite) initOperatorNamespace() {
	s.operatorNamespace = operatorPodNamespacePrefix + "-" + s.TestNamespace
	s.CreateNamespace(s.operatorNamespace)
	s.reconciler.operatorPodNamespace = s.operatorNamespace
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
			Namespace: s.TestNamespace,
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

func (s *KafkaServerConfigReconcilerTestSuite) reconcile(namespacedName types.NamespacedName) {
	res := ctrl.Result{Requeue: true}
	var err error
	for res.Requeue || res.RequeueAfter > 0 {
		res, err = s.reconciler.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: namespacedName,
		})
		if k8serrors.IsConflict(err) {
			res.Requeue = true
		}
	}

	s.Require().NoError(err)
	s.Require().Empty(res)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
}

func (s *KafkaServerConfigReconcilerTestSuite) TestKafkaServerConfigUpload() {
	// Create kafka server config resource
	kafkaServerConfig := s.generateKafkaServerConfig()
	kafkaServerConfig.SetNamespace(s.TestNamespace)
	s.AddKafkaServerConfig(&kafkaServerConfig)

	// Set go mock expectations
	expectedConfigs := s.getExpectedKafkaServerConfigs(kafkaServerConfig)
	s.mockCloudClient.EXPECT().ReportKafkaServerConfig(gomock.Any(), s.TestNamespace, gomock.Eq(expectedConfigs)).Return(nil)
	s.mockIntentsAdmin.EXPECT().ApplyServerTopicsConf(kafkaServerConfig.Spec.Topics).Return(nil)
	s.mockIntentsAdmin.EXPECT().Close()

	s.reconcile(types.NamespacedName{
		Name:      kafkaServiceName,
		Namespace: s.TestNamespace,
	})
}

func (s *KafkaServerConfigReconcilerTestSuite) getExpectedKafkaServerConfigs(kafkaServerConfig otterizev1alpha2.KafkaServerConfig) []graphqlclient.KafkaServerConfigInput {
	ksc, err := kafkaServerConfigCRDToCloudModel(kafkaServerConfig)
	s.Require().NoError(err)

	return []graphqlclient.KafkaServerConfigInput{ksc}
}

func (s *KafkaServerConfigReconcilerTestSuite) TestReUploadKafkaServerConfigOnFailure() {
	// Create kafka server config resource
	kafkaServerConfig := s.generateKafkaServerConfig()
	kafkaServerConfig.SetNamespace(s.TestNamespace)
	s.AddKafkaServerConfig(&kafkaServerConfig)

	// Make the mock return error to the reconciler, so it thinks the report failed
	expectedConfigs := s.getExpectedKafkaServerConfigs(kafkaServerConfig)
	s.mockIntentsAdmin.EXPECT().ApplyServerTopicsConf(kafkaServerConfig.Spec.Topics).Return(nil).Times(1)
	s.mockCloudClient.EXPECT().ReportKafkaServerConfig(
		gomock.Any(),
		s.TestNamespace,
		gomock.Eq(expectedConfigs),
	).Return(errors.New("failed to upload kafka server config"))

	s.mockIntentsAdmin.EXPECT().Close()

	// We expect the reconciler to retry to report, this time we don't return an error, simulating success
	s.mockIntentsAdmin.EXPECT().ApplyServerTopicsConf(kafkaServerConfig.Spec.Topics).Return(nil).Times(1)
	s.mockCloudClient.EXPECT().ReportKafkaServerConfig(
		gomock.Any(),
		s.TestNamespace,
		gomock.Eq(expectedConfigs),
	).Return(nil).Times(1)
	s.mockIntentsAdmin.EXPECT().Close().Times(1)

	s.reconcile(types.NamespacedName{
		Name:      kafkaServiceName,
		Namespace: s.TestNamespace,
	})

}

func (s *KafkaServerConfigReconcilerTestSuite) TestKafkaServerConfigDelete() {
	// Create kafka server config resource
	kafkaServerConfig := s.generateKafkaServerConfig()
	kafkaServerConfig.SetNamespace(s.TestNamespace)
	s.AddKafkaServerConfig(&kafkaServerConfig)

	// Set go mock expectations
	expectedConfigs := s.getExpectedKafkaServerConfigs(kafkaServerConfig)

	gomock.InOrder(
		s.mockCloudClient.EXPECT().ReportKafkaServerConfig(gomock.Any(), s.TestNamespace, gomock.Eq(expectedConfigs)).Return(nil),
		s.mockCloudClient.EXPECT().ReportKafkaServerConfig(gomock.Any(), s.TestNamespace, gomock.Eq([]graphqlclient.KafkaServerConfigInput{})).Return(nil),
	)

	gomock.InOrder(
		s.mockIntentsAdmin.EXPECT().ApplyServerTopicsConf(kafkaServerConfig.Spec.Topics).Return(nil).Times(1),
		s.mockIntentsAdmin.EXPECT().Close().Times(1),
		s.mockIntentsAdmin.EXPECT().RemoveAllIntents().Return(nil).Times(1),
		s.mockIntentsAdmin.EXPECT().Close().Times(1),
	)

	s.reconcile(types.NamespacedName{
		Name:      kafkaServerConfig.Name,
		Namespace: s.TestNamespace,
	})

	// Delete kafka server config resource
	s.RemoveKafkaServerConfig(kafkaServerConfig.Name)

	// Set go mock expectations

	s.reconcile(types.NamespacedName{
		Name:      kafkaServiceName,
		Namespace: s.TestNamespace,
	})
}

func (s *KafkaServerConfigReconcilerTestSuite) TestIntentsGeneratedForOperator() {
	s.initOperatorNamespace()

	operatorServiceName := "intents-operator-service"
	s.mockServiceResolver.EXPECT().GetPodAnnotatedName(gomock.Any(), operatorPodName, s.operatorNamespace).Return(operatorServiceName, true, nil)

	kafkaServerConfig := s.generateKafkaServerConfig()
	kafkaServerConfig.SetNamespace(s.TestNamespace)
	kafkaServerConfig.Spec.NoAutoCreateIntentsForOperator = false
	s.AddKafkaServerConfig(&kafkaServerConfig)

	// Set go mock expectations
	expectedConfigs := s.getExpectedKafkaServerConfigs(kafkaServerConfig)
	s.mockCloudClient.EXPECT().ReportKafkaServerConfig(gomock.Any(), s.TestNamespace, gomock.Eq(expectedConfigs)).Return(nil)
	s.mockIntentsAdmin.EXPECT().ApplyServerTopicsConf(kafkaServerConfig.Spec.Topics).Return(nil)
	s.mockIntentsAdmin.EXPECT().Close()

	s.reconcile(types.NamespacedName{
		Name:      kafkaServiceName,
		Namespace: s.TestNamespace,
	})

	operatorIntentsObjectName := fmt.Sprintf("operator-to-kafkaserverconfig-kafka-namespace-%s", s.TestNamespace)
	intents := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorIntentsObjectName,
			Namespace: s.operatorNamespace,
		},
		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: operatorServiceName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: fmt.Sprintf("%s.%s", kafkaServiceName, s.TestNamespace),
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

	var actualIntents otterizev1alpha2.ClientIntents
	err := s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{
		Name:      operatorIntentsObjectName,
		Namespace: s.operatorNamespace,
	}, &actualIntents)
	s.Require().NoError(err)

	s.Require().Equal(intents.ObjectMeta.Name, actualIntents.ObjectMeta.Name)
	s.Require().Equal(intents.ObjectMeta.Namespace, actualIntents.ObjectMeta.Namespace)
	s.Require().Equal(intents.Spec.Service.Name, actualIntents.Spec.Service.Name)
	s.Require().Equal(intents.Spec.Calls[0].Name, actualIntents.Spec.Calls[0].Name)
	s.Require().Equal(intents.Spec.Calls[0].Type, actualIntents.Spec.Calls[0].Type)
	s.Require().Equal(intents.Spec.Calls[0].Topics[0].Name, actualIntents.Spec.Calls[0].Topics[0].Name)
	s.Require().Equal(intents.Spec.Calls[0].Topics[0].Operations[0], actualIntents.Spec.Calls[0].Topics[0].Operations[0])
	s.Require().Equal(intents.Spec.Calls[0].Topics[0].Operations[1], actualIntents.Spec.Calls[0].Topics[0].Operations[1])
}

func TestKafkaACLReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(KafkaServerConfigReconcilerTestSuite))
}
