package intents_reconcilers

import (
	"context"
	"fmt"
	"github.com/Shopify/sarama"
	"github.com/golang/mock/gomock"
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	kafkaaclsmocks "github.com/otterize/intents-operator/src/operator/controllers/kafkaacls/mocks"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"testing"
)

const (
	kafkaServiceName  string = "kafka"
	kafkaTopicName    string = "test-topic"
	clientName        string = "test-client"
	intentsObjectName string = "test-client-intents"
	usernameMapping   string = "user-name-mapping-test"
)

type KafkaACLReconcilerTestSuite struct {
	testbase.ControllerManagerTestSuiteBase
	Reconciler     *KafkaACLReconciler
	mockKafkaAdmin *kafkaaclsmocks.MockClusterAdmin
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

	err = otterizev1alpha1.AddToScheme(s.TestEnv.Scheme)
	s.Require().NoError(err)
}

func (s *KafkaACLReconcilerTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()
}

func (s *KafkaACLReconcilerTestSuite) setupServerStore(serviceName string) *kafkaacls.ServersStoreImpl {
	serverConfig := &otterizev1alpha1.KafkaServerConfig{
		Spec: otterizev1alpha1.KafkaServerConfigSpec{
			Service: otterizev1alpha1.Service{
				Name: serviceName,
			},
			Topics: []otterizev1alpha1.TopicConfig{{
				Topic:                  "*",
				Pattern:                otterizev1alpha1.ResourcePatternTypePrefix,
				ClientIdentityRequired: false,
				IntentsRequired:        false,
			},
			},
		},
	}

	serverConfig.SetNamespace(s.TestNamespace)
	emptyTls := otterizev1alpha1.TLSSource{}
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

	s.initKafkaIntentsAdmin(true)
}

func (s *KafkaACLReconcilerTestSuite) initKafkaIntentsAdmin(enableAclCreation bool) {
	kafkaServersStore := s.setupServerStore(kafkaServiceName)
	newTestKafkaIntentsAdmin := getMockIntentsAdminFactory(s.mockKafkaAdmin, usernameMapping)
	s.Reconciler = NewKafkaACLReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme, kafkaServersStore, enableAclCreation, newTestKafkaIntentsAdmin, true)
	recorder := s.Mgr.GetEventRecorderFor("intents-operator")
	s.Reconciler.InjectRecorder(recorder)
}

func (s *KafkaACLReconcilerTestSuite) principal() string {
	return fmt.Sprintf("User:%s", usernameMapping)
}

func getMockIntentsAdminFactory(clusterAdmin sarama.ClusterAdmin, usernameMapping string) kafkaacls.IntentsAdminFactoryFunction {
	return func(kafkaServer otterizev1alpha1.KafkaServerConfig, _ otterizev1alpha1.TLSSource, enableKafkaACLCreation bool, globalEnforceSetting bool) (kafkaacls.KafkaIntentsAdmin, error) {
		return kafkaacls.NewKafkaIntentsAdminImpl(kafkaServer, clusterAdmin, usernameMapping, enableKafkaACLCreation, globalEnforceSetting), nil
	}
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
	intentsConfig := s.generateIntents(otterizev1alpha1.KafkaOperationProduce)

	intentsConsume := []otterizev1alpha1.Intent{intentsConfig}
	_, err := s.AddIntents(intentsObjectName, clientName, intentsConsume)
	s.Require().NoError(err)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
	namespacedName := types.NamespacedName{
		Namespace: s.TestNamespace,
		Name:      intentsObjectName,
	}

	s.reconcile(namespacedName)

	intentsFromReconciler := &otterizev1alpha1.ClientIntents{}
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
	intentsConfigCopy.Topics[0].Operations = append(intentsConfigCopy.Topics[0].Operations, otterizev1alpha1.KafkaOperationConsume)
	intentsWithConsumeAndProduce := []otterizev1alpha1.Intent{intentsConfigCopy}

	// Update the intents object and reconcile
	err = s.UpdateIntents(intentsObjectName, intentsWithConsumeAndProduce)
	s.Require().NoError(err)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
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
	intentsConfig := s.generateIntents(otterizev1alpha1.KafkaOperationConsume)
	intents := []otterizev1alpha1.Intent{intentsConfig}

	clientIntents, err := s.AddIntents(intentsObjectName, clientName, intents)
	s.Require().NoError(err)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

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
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	s.reconcile(namespacedName)
}

func (s *KafkaACLReconcilerTestSuite) TestKafkaACLCreationDisabled() {
	// Override the default reconciler with ACL creation disabled
	s.initKafkaIntentsAdmin(false)

	// Expect only to check the ACL list and close, with not creation
	s.mockKafkaAdmin.EXPECT().ListAcls(gomock.Any()).Return([]sarama.ResourceAcls{}, nil).Times(2)
	s.mockKafkaAdmin.EXPECT().Close().Times(1)

	// Create intents object with Consume operation
	intentsConfig := s.generateIntents(otterizev1alpha1.KafkaOperationConsume)
	intents := []otterizev1alpha1.Intent{intentsConfig}

	clientIntents, err := s.AddIntents(intentsObjectName, clientName, intents)
	s.Require().NoError(err)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	namespacedName := types.NamespacedName{
		Namespace: s.TestNamespace,
		Name:      clientIntents.Name,
	}

	s.reconcile(namespacedName)
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
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
}

func (s *KafkaACLReconcilerTestSuite) generateIntents(operation otterizev1alpha1.KafkaOperation) otterizev1alpha1.Intent {
	intentsConfig := otterizev1alpha1.Intent{
		Name:      kafkaServiceName,
		Type:      otterizev1alpha1.IntentTypeKafka,
		Namespace: s.TestNamespace,
		Topics: []otterizev1alpha1.KafkaTopic{{
			Name:       kafkaTopicName,
			Operations: []otterizev1alpha1.KafkaOperation{operation},
		},
		},
	}
	return intentsConfig
}

func TestKafkaACLReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(KafkaACLReconcilerTestSuite))
}
