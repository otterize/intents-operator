package intents_reconcilers

import (
	"context"
	"fmt"
	"github.com/Shopify/sarama"
	"github.com/google/uuid"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	otterizev1beta1 "github.com/otterize/intents-operator/src/operator/api/v1beta1"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	intentsreconcilersmocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/operator/controllers/kafkaacls"
	kafkaaclsmocks "github.com/otterize/intents-operator/src/operator/controllers/kafkaacls/mocks"
	"github.com/otterize/intents-operator/src/operator/webhooks"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/podownerresolver"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	istiosecurityscheme "istio.io/client-go/pkg/apis/security/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
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
	logrus.Info("Setting up test suite")
	s.TestEnv = &envtest.Environment{Scheme: scheme.Scheme}
	var err error
	s.TestEnv.CRDDirectoryPaths = []string{filepath.Join("..", "..", "config", "crd")}
	s.TestEnv.WebhookInstallOptions = envtest.WebhookInstallOptions{
		Paths:            []string{filepath.Join("..", "..", "config", "webhook")},
		LocalServingHost: "localhost",
	}
	utilruntime.Must(apiextensionsv1.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(istiosecurityscheme.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(otterizev1alpha2.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(otterizev1alpha3.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(otterizev1beta1.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(otterizev2alpha1.AddToScheme(s.TestEnv.Scheme))

	s.RestConfig, err = s.TestEnv.Start()
	s.Require().NoError(err)
	s.Require().NotNil(s.RestConfig)

	s.K8sDirectClient, err = kubernetes.NewForConfig(s.RestConfig)
	s.Require().NoError(err)
	s.Require().NotNil(s.K8sDirectClient)
}

func (s *KafkaACLReconcilerTestSuite) setupServerStore(serviceName string) *kafkaacls.ServersStoreImpl {
	serverConfig := &otterizev2alpha1.KafkaServerConfig{
		Spec: otterizev2alpha1.KafkaServerConfigSpec{
			Service: otterizev2alpha1.Workload{
				Name: serviceName,
			},
			Topics: []otterizev2alpha1.TopicConfig{{
				Topic:                  "*",
				Pattern:                otterizev2alpha1.ResourcePatternTypePrefix,
				ClientIdentityRequired: false,
				IntentsRequired:        false,
			},
			},
		},
	}

	serverConfig.SetNamespace(s.TestNamespace)
	emptyTls := otterizev2alpha1.TLSSource{}
	kafkaServersStore := kafkaacls.NewServersStore(emptyTls, true, kafkaacls.NewKafkaIntentsAdmin, true)
	kafkaServersStore.Add(serverConfig)
	return kafkaServersStore
}

func (s *KafkaACLReconcilerTestSuite) TearDownSuite() {
	s.ControllerManagerTestSuiteBase.TearDownSuite()
}

func (s *KafkaACLReconcilerTestSuite) BeforeTest(_, testName string) {
	logrus.Infof("Running test: %s", testName)
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
		nil,
	)
	s.recorder = record.NewFakeRecorder(100)
	s.Reconciler.InjectRecorder(s.recorder)
}

func (s *KafkaACLReconcilerTestSuite) principal() string {
	return fmt.Sprintf("User:%s", usernameMapping)
}

func getMockIntentsAdminFactory(clusterAdmin sarama.ClusterAdmin, usernameMapping string) kafkaacls.IntentsAdminFactoryFunction {
	return func(kafkaServer otterizev2alpha1.KafkaServerConfig, _ otterizev2alpha1.TLSSource, enableKafkaACLCreation bool, enforcementDefaultState bool) (kafkaacls.KafkaIntentsAdmin, error) {
		return kafkaacls.NewKafkaIntentsAdminImpl(kafkaServer, clusterAdmin, usernameMapping, enableKafkaACLCreation, enforcementDefaultState), nil
	}
}

func (s *KafkaACLReconcilerTestSuite) TestNoACLCreatedForIntentsOperator() {
	s.initOperatorNamespace()

	intentsName := "intents-operator-calls-to-kafka"
	operatorIntents := []otterizev2alpha1.Target{{
		Kafka: &otterizev2alpha1.KafkaTarget{
			Name: kafkaServiceName,
			Topics: []otterizev2alpha1.KafkaTopic{{
				Name: "*",
				Operations: []otterizev2alpha1.KafkaOperation{
					otterizev2alpha1.KafkaOperationAlter,
					otterizev2alpha1.KafkaOperationDescribe,
				},
			}},
		},
	}}

	_, err := s.AddIntentsInNamespace(intentsName, operatorServiceName, "", s.operatorNamespace, operatorIntents)
	s.Require().NoError(err)

	operatorPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				podownerresolver.WorkloadNameOverrideAnnotationKey: operatorServiceName,
			},
			Name:      operatorPodName,
			Namespace: s.operatorNamespace,
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{
				Name:            operatorPodName,
				Image:           "nginx",
				ImagePullPolicy: "Always",
			},
		},
		},
	}
	err = s.Mgr.GetClient().Create(context.Background(), &operatorPod)
	s.Require().NoError(err)
	s.WaitForObjectToBeCreated(&operatorPod)

	s.mockServiceResolver.EXPECT().ResolvePodToServiceIdentity(gomock.Any(), &operatorPod).Return(serviceidentity.ServiceIdentity{Name: operatorServiceName, Namespace: s.operatorNamespace}, nil)

	// Shouldn't creat ACLs for the operator intents therefore not expecting any call to mockKafkaAdmin

	operatorNamespacedName := types.NamespacedName{
		Name:      intentsName,
		Namespace: s.operatorNamespace,
	}
	s.reconcile(operatorNamespacedName, false)
}

func (s *KafkaACLReconcilerTestSuite) initOperatorNamespace() {
	s.operatorNamespace = operatorPodNamespacePrefix + "-" + uuid.New().String()
	s.CreateNamespace(s.operatorNamespace)
	s.Reconciler.operatorPodNamespace = s.operatorNamespace
}

func (s *KafkaACLReconcilerTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()
	intentsValidator := webhooks.NewIntentsValidatorV1alpha2(s.Mgr.GetClient())
	s.Require().NoError((&otterizev1alpha2.ClientIntents{}).SetupWebhookWithManager(s.Mgr, intentsValidator))
	intentsValidator13 := webhooks.NewIntentsValidatorV1alpha3(s.Mgr.GetClient())
	s.Require().NoError((&otterizev1alpha3.ClientIntents{}).SetupWebhookWithManager(s.Mgr, intentsValidator13))
	intentsValidator2 := webhooks.NewIntentsValidatorV2alpha1(s.Mgr.GetClient())
	s.Require().NoError((&otterizev2alpha1.ClientIntents{}).SetupWebhookWithManager(s.Mgr, intentsValidator2))
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
	intentsConfig := s.generateIntents(otterizev2alpha1.KafkaOperationProduce)

	intentsConsume := []otterizev2alpha1.Target{intentsConfig}
	_, err := s.AddIntents(intentsObjectName, clientName, "", intentsConsume)
	s.Require().NoError(err)
	namespacedName := types.NamespacedName{
		Namespace: s.TestNamespace,
		Name:      intentsObjectName,
	}

	s.reconcile(namespacedName, true)

	intentsFromReconciler := &otterizev2alpha1.ClientIntents{}
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
	intentsConfigCopy.Kafka.Topics[0].Operations = append(intentsConfigCopy.Kafka.Topics[0].Operations, otterizev2alpha1.KafkaOperationConsume)
	intentsWithConsumeAndProduce := []otterizev2alpha1.Target{intentsConfigCopy}

	// Update the intents object and reconcile
	err = s.UpdateIntents(intentsObjectName, intentsWithConsumeAndProduce)
	s.Require().NoError(err)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), namespacedName, intentsFromReconciler)
		assert.Equal(len(intentsFromReconciler.Spec.Targets[0].Kafka.Topics[0].Operations), 2)
	})

	s.reconcile(namespacedName, true)
}

func (s *KafkaACLReconcilerTestSuite) TestKafkaACLDeletedAfterIntentsRemoved() {
	s.T().Skipf("Flaky test")
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

	list1 := s.mockKafkaAdmin.EXPECT().ListAcls(gomock.Any()).Return([]sarama.ResourceAcls{}, nil).MinTimes(1)
	s.mockKafkaAdmin.EXPECT().CreateACLs(MatchSaramaResource(aclForConsume)).Return(nil).MinTimes(1)
	list2 := s.mockKafkaAdmin.EXPECT().ListAcls(gomock.Any()).Return([]sarama.ResourceAcls{createACL}, nil).MinTimes(1)
	s.mockKafkaAdmin.EXPECT().Close()

	gomock.InOrder(list1, list2)

	// Create intents object with Consume operation
	intentsConfig := s.generateIntents(otterizev2alpha1.KafkaOperationConsume)
	intents := []otterizev2alpha1.Target{intentsConfig}

	clientIntents, err := s.AddIntents(intentsObjectName, clientName, "", intents)
	s.Require().NoError(err)

	namespacedName := types.NamespacedName{
		Namespace: s.TestNamespace,
		Name:      clientIntents.Name,
	}

	s.reconcile(namespacedName, true)

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

	s.reconcile(namespacedName, true)
}

func (s *KafkaACLReconcilerTestSuite) TestKafkaACLCreationDisabled() {
	// Override the default reconciler with ACL creation disabled
	s.initKafkaIntentsAdmin(false, true)

	// Expect only to check the ACL list and close, with not creation
	s.mockKafkaAdmin.EXPECT().ListAcls(gomock.Any()).Return([]sarama.ResourceAcls{}, nil).Times(2)
	s.mockKafkaAdmin.EXPECT().Close().Times(1)

	// Create intents object with Consume operation
	intentsConfig := s.generateIntents(otterizev2alpha1.KafkaOperationConsume)
	intents := []otterizev2alpha1.Target{intentsConfig}

	clientIntents, err := s.AddIntents(intentsObjectName, clientName, "", intents)
	s.Require().NoError(err)

	namespacedName := types.NamespacedName{
		Namespace: s.TestNamespace,
		Name:      clientIntents.Name,
	}

	s.reconcile(namespacedName, true)
}

func (s *KafkaACLReconcilerTestSuite) TestKafkaACLEnforcementGloballyDisabled() {
	// Override the default reconciler with ACL creation disabled
	s.initKafkaIntentsAdmin(true, false)

	// Expect only to check the ACL list and close, with not creation
	s.mockKafkaAdmin.EXPECT().ListAcls(gomock.Any()).Return([]sarama.ResourceAcls{}, nil).Times(2)
	s.mockKafkaAdmin.EXPECT().Close().Times(1)

	// Create intents object with Consume operation
	intentsConfig := s.generateIntents(otterizev2alpha1.KafkaOperationConsume)
	intents := []otterizev2alpha1.Target{intentsConfig}

	clientIntents, err := s.AddIntents(intentsObjectName, clientName, "", intents)
	s.Require().NoError(err)

	namespacedName := types.NamespacedName{
		Namespace: s.TestNamespace,
		Name:      clientIntents.Name,
	}

	s.reconcile(namespacedName, true)
	// the actual test is that there are not unexpected calls to the mockKafkaAdmin
	select {
	case event := <-s.recorder.Events:
		s.Require().Contains(event, consts.ReasonEnforcementDefaultOff)
	default:
		s.Fail("event not raised")
	}
}

func (s *KafkaACLReconcilerTestSuite) reconcile(namespacedName types.NamespacedName, expectLogsOnReQueue bool) {
	res := ctrl.Result{Requeue: true}
	var err error
	firstRun := true

	for res.Requeue {
		if !firstRun && expectLogsOnReQueue {
			logrus.Infof("Retrying reconcile for namespace %s", namespacedName.Namespace)
			// List ACL is called when the reconciler is re-queued for log purposes
			s.mockKafkaAdmin.EXPECT().ListAcls(gomock.Any()).Return([]sarama.ResourceAcls{}, nil).Times(1)
		}
		res, err = s.Reconciler.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: namespacedName,
		})

		firstRun = false
	}

	s.Require().NoError(err)
	s.Require().Empty(res)
}

func (s *KafkaACLReconcilerTestSuite) generateIntents(operation otterizev2alpha1.KafkaOperation) otterizev2alpha1.Target {
	intentsConfig := otterizev2alpha1.Target{
		Kafka: &otterizev2alpha1.KafkaTarget{
			Name: kafkaServiceName,
			Topics: []otterizev2alpha1.KafkaTopic{{
				Name:       kafkaTopicName,
				Operations: []otterizev2alpha1.KafkaOperation{operation},
			},
			},
		},
	}
	return intentsConfig
}

func TestKafkaACLReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(KafkaACLReconcilerTestSuite))
}
