package intents_reconcilers

import (
	"context"
	"fmt"
	"github.com/Shopify/sarama"
	"github.com/google/uuid"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	intentsreconcilersmocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	kafkaaclsmocks "github.com/otterize/intents-operator/src/operator/controllers/kafkaacls/mocks"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	istiosecurityscheme "istio.io/client-go/pkg/apis/security/v1beta1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
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
	operatorServiceName        string = "intents-operator"
	operatorPodName            string = "operator-pod-name"
	operatorPodNamespacePrefix string = "otterize-ns"
)

type KafkaACLReconcilerTestSuite struct {
	testbase.ControllerManagerTestSuiteBase
	Reconciler          *KafkaACLReconciler
	mockKafkaAdmin      *kafkaaclsmocks.MockClusterAdmin
	recorder            *record.FakeRecorder
	mockServiceResolver *intentsreconcilersmocks.MockServiceResolver
	operatorNamespace   string
}

func (s *KafkaACLReconcilerTestSuite) SetupSuite() {
	s.TestEnv = &envtest.Environment{}
	var err error
	s.TestEnv.CRDDirectoryPaths = []string{filepath.Join("..", "..", "config", "crd")}

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
	utilruntime.Must(otterizev1alpha3.AddToScheme(s.TestEnv.Scheme))
}

func (s *KafkaACLReconcilerTestSuite) setupServerStore(serviceName string) *kafkaacls.ServersStoreImpl {
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
	kafkaServersStore := kafkaacls.NewServersStore(emptyTls, true, kafkaacls.NewKafkaIntentsAdmin, true)
	kafkaServersStore.Add(serverConfig)
	return kafkaServersStore
}

func (s *KafkaACLReconcilerTestSuite) TearDownSuite() {
	s.ControllerManagerTestSuiteBase.TearDownSuite()
}

func (s *KafkaACLReconcilerTestSuite) BeforeTest(_, testName string) {
	s.ControllerManagerTestSuiteBase.BeforeTest("", testName)

	controller := gomock.NewController(s.T())
	s.mockKafkaAdmin = kafkaaclsmocks.NewMockClusterAdmin(controller)
	s.mockServiceResolver = intentsreconcilersmocks.NewMockServiceResolver(controller)

	s.initKafkaIntentsAdmin(true, true)
}

func (s *KafkaACLReconcilerTestSuite) initKafkaIntentsAdmin(enableAclCreation bool, enforcementDefaultState bool) {
	kafkaServersStore := s.setupServerStore(kafkaServiceName)
	newTestKafkaIntentsAdmin := getMockIntentsAdminFactory(s.mockKafkaAdmin, usernameMapping)
	s.Reconciler = NewKafkaACLReconciler(
		s.Mgr.GetClient(),
		s.TestEnv.Scheme,
		kafkaServersStore,
		enableAclCreation,
		newTestKafkaIntentsAdmin,
		enforcementDefaultState,
		operatorPodName,
		s.operatorNamespace,
		s.mockServiceResolver,
	)
	s.recorder = record.NewFakeRecorder(100)
	s.Reconciler.InjectRecorder(s.recorder)
}

func (s *KafkaACLReconcilerTestSuite) principal() string {
	return fmt.Sprintf("User:%s", usernameMapping)
}

func getMockIntentsAdminFactory(clusterAdmin sarama.ClusterAdmin, usernameMapping string) kafkaacls.IntentsAdminFactoryFunction {
	return func(kafkaServer otterizev1alpha2.KafkaServerConfig, _ otterizev1alpha2.TLSSource, enableKafkaACLCreation bool, enforcementDefaultState bool) (kafkaacls.KafkaIntentsAdmin, error) {
		return kafkaacls.NewKafkaIntentsAdminImpl(kafkaServer, clusterAdmin, usernameMapping, enableKafkaACLCreation, enforcementDefaultState), nil
	}
}

func (s *KafkaACLReconcilerTestSuite) TestNoACLCreatedForIntentsOperator() {
	s.initOperatorNamespace()

	intentsName := "intents-operator-calls-to-kafka"
	operatorIntents := []otterizev1alpha3.Intent{{
		Name: kafkaServiceName,
		Type: otterizev1alpha3.IntentTypeKafka,
		Topics: []otterizev1alpha3.KafkaTopic{{
			Name: "*",
			Operations: []otterizev1alpha3.KafkaOperation{
				otterizev1alpha3.KafkaOperationAlter,
				otterizev1alpha3.KafkaOperationDescribe,
			},
		}},
	}}

	_, err := s.AddIntentsInNamespace(intentsName, operatorServiceName, s.operatorNamespace, operatorIntents)
	s.Require().NoError(err)

	s.mockServiceResolver.EXPECT().GetPodAnnotatedName(gomock.Any(), operatorPodName, s.operatorNamespace).Return(operatorServiceName, true, nil)

	// Shouldn't creat ACLs for the operator intents therefore not expecting any call to mockKafkaAdmin

	operatorNamespacedName := types.NamespacedName{
		Name:      intentsName,
		Namespace: s.operatorNamespace,
	}
	s.reconcile(operatorNamespacedName)
}

func (s *KafkaACLReconcilerTestSuite) initOperatorNamespace() {
	s.operatorNamespace = operatorPodNamespacePrefix + "-" + uuid.New().String()
	s.CreateNamespace(s.operatorNamespace)
	s.Reconciler.operatorPodNamespace = s.operatorNamespace
}

func (s *KafkaACLReconcilerTestSuite) TestKafkaACLGetCreatedAndUpdatedBasedOnIntents() {
	// ACL Objects for produce-write and consume-read
	resource := sarama.Resource{
		ResourceType:        sarama.AclResourceTopic,
		ResourceName:        kafkaTopicName,
		ResourcePatternType: sarama.AclPatternLiteral,
	}

	writeOperation := sarama.Acl{
		Principal:      s.principal(),
		Host:           "*",
		Operation:      sarama.AclOperationWrite,
		PermissionType: sarama.AclPermissionAllow,
	}

	writeAcl := sarama.ResourceAcls{
		Resource: resource,
		Acls:     []*sarama.Acl{&writeOperation},
	}

	readOperation := sarama.Acl{
		Principal:      s.principal(),
		Host:           "*",
		Operation:      sarama.AclOperationRead,
		PermissionType: sarama.AclPermissionAllow,
	}

	readAcl := sarama.ResourceAcls{
		Resource: resource,
		Acls:     []*sarama.Acl{&readOperation},
	}

	aclForProduce := []*sarama.ResourceAcls{&writeAcl}
	aclForConsume := []*sarama.ResourceAcls{&readAcl}
	aclFullList := sarama.ResourceAcls{
		Resource: resource,
		Acls: []*sarama.Acl{
			&writeOperation,
			&readOperation,
		},
	}

	// Expected arguments sent to sarama for the produce-write
	s.mockKafkaAdmin.EXPECT().ListAcls(gomock.Any()).Return([]sarama.ResourceAcls{}, nil).Times(1)
	s.mockKafkaAdmin.EXPECT().CreateACLs(MatchSaramaResource(aclForProduce)).Return(nil).Times(1)
	s.mockKafkaAdmin.EXPECT().ListAcls(gomock.Any()).Return([]sarama.ResourceAcls{writeAcl}, nil).Times(1)
	s.mockKafkaAdmin.EXPECT().Close().Times(1)

	// Generate intents for produce and reconcile
	intentsConfig := s.generateIntents(otterizev1alpha3.KafkaOperationProduce)

	intentsConsume := []otterizev1alpha3.Intent{intentsConfig}
	_, err := s.AddIntents(intentsObjectName, clientName, intentsConsume)
	s.Require().NoError(err)
	namespacedName := types.NamespacedName{
		Namespace: s.TestNamespace,
		Name:      intentsObjectName,
	}

	s.reconcile(namespacedName)

	intentsFromReconciler := &otterizev1alpha3.ClientIntents{}
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		_ = s.Mgr.GetClient().Get(context.Background(), namespacedName, intentsFromReconciler)
		f := intentsFromReconciler.GetFinalizers()
		assert.Equal(len(f), 1)
	})

	// Expected arguments sent to sarama for the consume-read
	s.mockKafkaAdmin.EXPECT().ListAcls(gomock.Any()).Return([]sarama.ResourceAcls{writeAcl}, nil).Times(1)
	s.mockKafkaAdmin.EXPECT().CreateACLs(MatchSaramaResource(aclForConsume)).Return(nil).Times(1)
	s.mockKafkaAdmin.EXPECT().ListAcls(gomock.Any()).Return([]sarama.ResourceAcls{aclFullList}, nil).Times(1)
	s.mockKafkaAdmin.EXPECT().Close()

	// Add consume operation to intents object
	intentsConfigCopy := *intentsConfig.DeepCopy()
	intentsConfigCopy.Topics[0].Operations = append(intentsConfigCopy.Topics[0].Operations, otterizev1alpha3.KafkaOperationConsume)
	intentsWithConsumeAndProduce := []otterizev1alpha3.Intent{intentsConfigCopy}

	// Update the intents object and reconcile
	err = s.UpdateIntents(intentsObjectName, intentsWithConsumeAndProduce)
	s.Require().NoError(err)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), namespacedName, intentsFromReconciler)
		assert.Equal(len(intentsFromReconciler.Spec.Calls[0].Topics[0].Operations), 2)
	})

	s.reconcile(namespacedName)
}

func (s *KafkaACLReconcilerTestSuite) TestKafkaACLDeletedAfterIntentsRemoved() {
	// Expected Acl for consume operation
	resource := sarama.Resource{
		ResourceType:        sarama.AclResourceTopic,
		ResourceName:        kafkaTopicName,
		ResourcePatternType: sarama.AclPatternLiteral,
	}

	createAclOperation := sarama.Acl{
		Principal:      s.principal(),
		Host:           "*",
		Operation:      sarama.AclOperationRead,
		PermissionType: sarama.AclPermissionAllow,
	}

	createACL := sarama.ResourceAcls{
		Resource: resource,
		Acls:     []*sarama.Acl{&createAclOperation},
	}

	aclForConsume := []*sarama.ResourceAcls{&createACL}

	s.mockKafkaAdmin.EXPECT().ListAcls(gomock.Any()).Return([]sarama.ResourceAcls{}, nil).Times(1)
	s.mockKafkaAdmin.EXPECT().CreateACLs(MatchSaramaResource(aclForConsume)).Return(nil)
	s.mockKafkaAdmin.EXPECT().ListAcls(gomock.Any()).Return([]sarama.ResourceAcls{createACL}, nil).Times(1)
	s.mockKafkaAdmin.EXPECT().Close().Times(1)

	// Create intents object with Consume operation
	intentsConfig := s.generateIntents(otterizev1alpha3.KafkaOperationConsume)
	intents := []otterizev1alpha3.Intent{intentsConfig}

	clientIntents, err := s.AddIntents(intentsObjectName, clientName, intents)
	s.Require().NoError(err)

	namespacedName := types.NamespacedName{
		Namespace: s.TestNamespace,
		Name:      clientIntents.Name,
	}

	s.reconcile(namespacedName)

	// Expected results when the ACL is deleted
	deleteResult := []sarama.MatchingAcl{{
		Err:      sarama.ErrNoError,
		ErrMsg:   nil,
		Resource: createACL.Resource,
		Acl:      *createACL.Acls[0],
	}}

	s.mockKafkaAdmin.EXPECT().ListAcls(gomock.Any()).Return([]sarama.ResourceAcls{createACL}, nil).Times(1)
	s.mockKafkaAdmin.EXPECT().DeleteACL(sarama.AclFilter{
		ResourceType:              sarama.AclResourceTopic,
		ResourcePatternTypeFilter: sarama.AclPatternAny,
		PermissionType:            sarama.AclPermissionAllow,
		Operation:                 sarama.AclOperationAny,
		Principal:                 lo.ToPtr(s.principal()),
		Host:                      lo.ToPtr("*"),
	}, true).Return(deleteResult, nil)

	s.mockKafkaAdmin.EXPECT().Close().Times(1)

	// Remove the intents object
	err = s.RemoveIntents(intentsObjectName)
	s.Require().NoError(err)

	s.reconcile(namespacedName)
}

func (s *KafkaACLReconcilerTestSuite) TestKafkaACLCreationDisabled() {
	// Override the default reconciler with ACL creation disabled
	s.initKafkaIntentsAdmin(false, true)

	// Expect only to check the ACL list and close, with not creation
	s.mockKafkaAdmin.EXPECT().ListAcls(gomock.Any()).Return([]sarama.ResourceAcls{}, nil).Times(2)
	s.mockKafkaAdmin.EXPECT().Close().Times(1)

	// Create intents object with Consume operation
	intentsConfig := s.generateIntents(otterizev1alpha3.KafkaOperationConsume)
	intents := []otterizev1alpha3.Intent{intentsConfig}

	clientIntents, err := s.AddIntents(intentsObjectName, clientName, intents)
	s.Require().NoError(err)

	namespacedName := types.NamespacedName{
		Namespace: s.TestNamespace,
		Name:      clientIntents.Name,
	}

	s.reconcile(namespacedName)
}

func (s *KafkaACLReconcilerTestSuite) TestKafkaACLEnforcementGloballyDisabled() {
	// Override the default reconciler with ACL creation disabled
	s.initKafkaIntentsAdmin(true, false)

	// Expect only to check the ACL list and close, with not creation
	s.mockKafkaAdmin.EXPECT().ListAcls(gomock.Any()).Return([]sarama.ResourceAcls{}, nil).Times(2)
	s.mockKafkaAdmin.EXPECT().Close().Times(1)

	// Create intents object with Consume operation
	intentsConfig := s.generateIntents(otterizev1alpha3.KafkaOperationConsume)
	intents := []otterizev1alpha3.Intent{intentsConfig}

	clientIntents, err := s.AddIntents(intentsObjectName, clientName, intents)
	s.Require().NoError(err)

	namespacedName := types.NamespacedName{
		Namespace: s.TestNamespace,
		Name:      clientIntents.Name,
	}

	s.reconcile(namespacedName)
	// the actual test is that there are not unexpected calls to the mockKafkaAdmin
	select {
	case event := <-s.recorder.Events:
		s.Require().Contains(event, consts.ReasonEnforcementDefaultOff)
	default:
		s.Fail("event not raised")
	}
}

func (s *KafkaACLReconcilerTestSuite) reconcile(namespacedName types.NamespacedName) {
	res := ctrl.Result{Requeue: true}
	var err error

	for res.Requeue {
		res, err = s.Reconciler.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: namespacedName,
		})
	}

	s.Require().NoError(err)
	s.Require().Empty(res)
}

func (s *KafkaACLReconcilerTestSuite) generateIntents(operation otterizev1alpha3.KafkaOperation) otterizev1alpha3.Intent {
	intentsConfig := otterizev1alpha3.Intent{
		Name: kafkaServiceName,
		Type: otterizev1alpha3.IntentTypeKafka,
		Topics: []otterizev1alpha3.KafkaTopic{{
			Name:       kafkaTopicName,
			Operations: []otterizev1alpha3.KafkaOperation{operation},
		},
		},
	}
	return intentsConfig
}

func TestKafkaACLReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(KafkaACLReconcilerTestSuite))
}
