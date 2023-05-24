package otterizecloud

import (
	"context"
	"github.com/golang/mock/gomock"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/mocks"
	serviceidresolvermocks "github.com/otterize/intents-operator/src/shared/serviceidresolver/mocks"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	istiosecurityscheme "istio.io/client-go/pkg/apis/security/v1beta1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	mockK8sClient   *serviceidresolvermocks.MockClient
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

	utilruntime.Must(apiextensionsv1.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(istiosecurityscheme.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(otterizev1alpha2.AddToScheme(s.TestEnv.Scheme))

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
	s.mockK8sClient = serviceidresolvermocks.NewMockClient(controller)

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

func (s *CloudReconcilerTestSuite) TestIntentsStatusUpload() {
	s.reconciler = NewOtterizeCloudReconciler(s.mockK8sClient, s.TestEnv.Scheme, s.mockCloudClient)

	serviceAccountName := "test-service-account"
	server := "test-server"
	clientIntents := &otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: s.TestNamespace,
			Annotations: map[string]string{
				otterizev1alpha2.OtterizeClientServiceAccountAnnotation: serviceAccountName,
				otterizev1alpha2.OtterizeSharedServiceAccountAnnotation: "false",
				otterizev1alpha2.OtterizeMissingSidecarAnnotation:       "false",
			},
		},
		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: server,
					Type: otterizev1alpha2.IntentTypeKafka,
					Topics: []otterizev1alpha2.KafkaTopic{{
						Name: "test-topic",
						Operations: []otterizev1alpha2.KafkaOperation{
							otterizev1alpha2.KafkaOperationCreate,
							otterizev1alpha2.KafkaOperationDelete,
						},
					},
					},
				},
			},
		},
	}

	var clientIntentsList otterizev1alpha2.ClientIntentsList
	s.mockK8sClient.EXPECT().List(gomock.Any(), &clientIntentsList, &client.ListOptions{Namespace: s.TestNamespace}).Do(
		func(_ context.Context, list *otterizev1alpha2.ClientIntentsList, _ ...client.ListOption) {
			list.Items = append(list.Items, *clientIntents)
		}).Return(nil).Times(1)

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
	expectedIntent.Status = &graphqlclient.IntentStatusInput{
		IstioStatus: &graphqlclient.IstioStatusInput{},
	}
	expectedIntent.Status.IstioStatus.ServiceAccountName = lo.ToPtr(serviceAccountName)
	expectedIntent.Status.IstioStatus.IsServiceAccountShared = lo.ToPtr(false)
	expectedIntent.Status.IstioStatus.IsClientMissingSidecar = lo.ToPtr(false)
	expectedIntent.Status.IstioStatus.IsServerMissingSidecar = lo.ToPtr(false)
	expectedIntents := []graphqlclient.IntentInput{expectedIntent}
	expectedNamespace := lo.ToPtr(s.TestNamespace)

	s.mockCloudClient.EXPECT().ReportAppliedIntents(gomock.Any(), expectedNamespace, GetMatcher(expectedIntents)).Return(nil).Times(1)

	s.reconcile(types.NamespacedName{
		Namespace: s.TestNamespace,
		Name:      intentsObjectName,
	})
}

func (s *CloudReconcilerTestSuite) TestHTTPUpload() {
	s.reconciler = NewOtterizeCloudReconciler(s.mockK8sClient, s.TestEnv.Scheme, s.mockCloudClient)

	serviceAccountName := "test-service-account"
	server := "test-server"
	clientIntents := &otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: s.TestNamespace,
			Annotations: map[string]string{
				otterizev1alpha2.OtterizeClientServiceAccountAnnotation: serviceAccountName,
				otterizev1alpha2.OtterizeSharedServiceAccountAnnotation: "false",
				otterizev1alpha2.OtterizeMissingSidecarAnnotation:       "false",
			},
		},
		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: server,
					Type: otterizev1alpha2.IntentTypeHTTP,
					HTTPResources: []otterizev1alpha2.HTTPResource{
						{
							Path: "/login",
							Methods: []otterizev1alpha2.HTTPMethod{
								otterizev1alpha2.HTTPMethodGet,
								otterizev1alpha2.HTTPMethodPost,
							},
						},
						{
							Path: "/logout",
							Methods: []otterizev1alpha2.HTTPMethod{
								otterizev1alpha2.HTTPMethodPost,
							},
						},
					},
				},
			},
		},
	}

	var clientIntentsList otterizev1alpha2.ClientIntentsList
	s.mockK8sClient.EXPECT().List(gomock.Any(), &clientIntentsList, &client.ListOptions{Namespace: s.TestNamespace}).Do(
		func(_ context.Context, list *otterizev1alpha2.ClientIntentsList, _ ...client.ListOption) {
			list.Items = append(list.Items, *clientIntents)
		}).Return(nil).Times(1)

	expectedIntent := intentInput(clientName, s.TestNamespace, server, s.TestNamespace)
	expectedIntent.Type = lo.ToPtr(graphqlclient.IntentTypeHttp)
	expectedIntent.Resources = []*graphqlclient.HTTPConfigInput{
		{
			Path:    lo.ToPtr("/login"),
			Methods: []*graphqlclient.HTTPMethod{lo.ToPtr(graphqlclient.HTTPMethodGet), lo.ToPtr(graphqlclient.HTTPMethodPost)},
		},
		{
			Path:    lo.ToPtr("/logout"),
			Methods: []*graphqlclient.HTTPMethod{lo.ToPtr(graphqlclient.HTTPMethodPost)},
		},
	}
	expectedIntent.Status = &graphqlclient.IntentStatusInput{
		IstioStatus: &graphqlclient.IstioStatusInput{},
	}
	expectedIntent.Status.IstioStatus.ServiceAccountName = lo.ToPtr(serviceAccountName)
	expectedIntent.Status.IstioStatus.IsServiceAccountShared = lo.ToPtr(false)
	expectedIntent.Status.IstioStatus.IsClientMissingSidecar = lo.ToPtr(false)
	expectedIntent.Status.IstioStatus.IsServerMissingSidecar = lo.ToPtr(false)

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
