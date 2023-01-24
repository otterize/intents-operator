package otterizecloud

import (
	"context"
	"github.com/golang/mock/gomock"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/mocks"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"testing"
)

const (
	clientName        string = "test-client"
	intentsObjectName string = "test-client-intents"
)

type CloudReconcilerTestSuite struct {
	testbase.ControllerManagerTestSuiteBase
	reconciler      *OtterizeCloudReconciler
	mockCloudClient *otterizecloudmocks.MockCloudClient
}

func (s *CloudReconcilerTestSuite) SetupSuite() {
	s.TestEnv = &envtest.Environment{}
	var err error
	s.TestEnv.CRDDirectoryPaths = []string{filepath.Join("..", "..", "..", "config", "crd")}

	s.RestConfig, err = s.TestEnv.Start()
	s.Require().NoError(err)
	s.Require().NotNil(s.RestConfig)

	s.K8sDirectClient, err = kubernetes.NewForConfig(s.RestConfig)
	s.Require().NoError(err)
	s.Require().NotNil(s.K8sDirectClient)

	err = otterizev1alpha2.AddToScheme(s.TestEnv.Scheme)
	s.Require().NoError(err)
}

func (s *CloudReconcilerTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()
}

func (s *CloudReconcilerTestSuite) TearDownSuite() {
	s.ControllerManagerTestSuiteBase.TearDownSuite()
}

func (s *CloudReconcilerTestSuite) BeforeTest(_, testName string) {
	s.ControllerManagerTestSuiteBase.BeforeTest("", testName)
	controller := gomock.NewController(s.T())
	s.mockCloudClient = otterizecloudmocks.NewMockCloudClient(controller)

	s.reconciler = NewOtterizeCloudReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme, s.mockCloudClient)

	recorder := s.Mgr.GetEventRecorderFor("intents-operator")
	s.reconciler.InjectRecorder(recorder)
}

func (s *CloudReconcilerTestSuite) reconcile(namespacedName types.NamespacedName) {
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

func (s *CloudReconcilerTestSuite) TestAppliedIntentsUpload() {
	server := "test-server"
	_, err := s.AddIntents(intentsObjectName, clientName, []otterizev1alpha2.Intent{{
		Name: server,
		Type: otterizev1alpha2.IntentTypeKafka,
		Topics: []otterizev1alpha2.KafkaTopic{{
			Name: "test-topic",
			Operations: []otterizev1alpha2.KafkaOperation{
				otterizev1alpha2.KafkaOperationCreate,
				otterizev1alpha2.KafkaOperationDelete,
			},
		}},
	},
	})
	s.Require().NoError(err)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	expectedIntent := intentInput(clientName, s.TestNamespace, server, s.TestNamespace)
	expectedIntent.Type = lo.ToPtr(graphqlclient.IntentTypeKafka)
	kafkaConfigInput := graphqlclient.KafkaConfigInput{
		Name: lo.ToPtr("test-topic"),
		Operations: []*graphqlclient.KafkaOperation{
			lo.ToPtr(graphqlclient.KafkaOperationDelete),
			lo.ToPtr(graphqlclient.KafkaOperationCreate),
		},
	}
	expectedIntent.Topics = []*graphqlclient.KafkaConfigInput{&kafkaConfigInput}
	expectedIntents := []graphqlclient.IntentInput{expectedIntent}
	expectedNamespace := lo.ToPtr(s.TestNamespace)
	s.mockCloudClient.EXPECT().ReportAppliedIntents(gomock.Any(), expectedNamespace, GetMatcher(expectedIntents)).Return(nil).Times(1)

	s.reconcile(types.NamespacedName{
		Namespace: s.TestNamespace,
		Name:      intentsObjectName,
	})
}

func (s *CloudReconcilerTestSuite) TestNamespaceParseSuccess() {
	serverName := "server.other-namespace"
	intent := &otterizev1alpha2.Intent{Name: serverName}

	cloudIntent := intent.ConvertToCloudFormat(s.TestNamespace, clientName)

	s.Require().Equal(lo.FromPtr(cloudIntent.Namespace), s.TestNamespace)
	s.Require().Equal(lo.FromPtr(cloudIntent.ClientName), clientName)
	s.Require().Equal(lo.FromPtr(cloudIntent.ServerName), "server")
	s.Require().Equal(lo.FromPtr(cloudIntent.ServerNamespace), "other-namespace")
}

func (s *CloudReconcilerTestSuite) TestTargetNamespaceAsSourceNamespace() {
	serverName := "server"
	intent := &otterizev1alpha2.Intent{Name: serverName}
	cloudIntent := intent.ConvertToCloudFormat(s.TestNamespace, clientName)
	s.Require().Equal(lo.FromPtr(cloudIntent.ServerNamespace), s.TestNamespace)
}

func intentInput(clientName string, namespace string, serverName string, serverNamespace string) graphqlclient.IntentInput {
	nilIfEmpty := func(s string) *string {
		if s == "" {
			return nil
		}
		return lo.ToPtr(s)
	}

	return graphqlclient.IntentInput{
		ClientName:      nilIfEmpty(clientName),
		ServerName:      nilIfEmpty(serverName),
		Namespace:       nilIfEmpty(namespace),
		ServerNamespace: nilIfEmpty(serverNamespace),
	}
}

func TestCloudReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(CloudReconcilerTestSuite))
}
