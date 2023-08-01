package intents_reconcilers

import (
	"context"
	"errors"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/mocks"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

type CloudReconcilerTestSuite struct {
	suite.Suite
	Reconciler      *OtterizeCloudReconciler
	client          *mocks.MockClient
	recorder        *record.FakeRecorder
	mockCloudClient *otterizecloudmocks.MockCloudClient
}

func (s *CloudReconcilerTestSuite) SetupTest() {
	controller := gomock.NewController(s.T())
	s.client = mocks.NewMockClient(controller)
	s.mockCloudClient = otterizecloudmocks.NewMockCloudClient(controller)

	s.Reconciler = NewOtterizeCloudReconciler(
		s.client,
		&runtime.Scheme{},
		s.mockCloudClient,
	)

	s.recorder = record.NewFakeRecorder(100)
	s.Reconciler.Recorder = s.recorder
}

func (s *CloudReconcilerTestSuite) TearDownTest() {
	s.Reconciler = nil
	s.expectNoEvent()
}

func (s *CloudReconcilerTestSuite) TestAppliedIntentsUpload() {
	server := "test-server"
	server2 := "other-server"
	server2Namespace := "other-namespace"

	s.assertUploadIntent(server, server2, server2Namespace)
}

func (s *CloudReconcilerTestSuite) assertUploadIntent(server string, server2 string, server2Namespace string) {
	server2FullName := server2 + "." + server2Namespace
	clientIntents := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: server,
					Type: "",
				},
				{
					Name: server2FullName,
				},
			},
		},
	}

	expectedIntentInNamespace := graphqlclient.IntentInput{
		ClientName:      lo.ToPtr(clientName),
		ServerName:      lo.ToPtr(server),
		Namespace:       lo.ToPtr(testNamespace),
		ServerNamespace: lo.ToPtr(testNamespace),
		Type:            nil,
		Topics:          nil,
		Resources:       nil,
	}

	expectedIntentInOtherNamespace := graphqlclient.IntentInput{
		ClientName:      lo.ToPtr(clientName),
		ServerName:      lo.ToPtr(server2),
		Namespace:       lo.ToPtr(testNamespace),
		ServerNamespace: lo.ToPtr(server2Namespace),
		Type:            nil,
		Topics:          nil,
		Resources:       nil,
	}

	expectedIntents := []graphqlclient.IntentInput{
		expectedIntentInNamespace,
		expectedIntentInOtherNamespace,
	}

	s.assertReportedIntents(clientIntents, expectedIntents)
}

func (s *CloudReconcilerTestSuite) TestAppliedIntentsUploadUnderscore() {
	server := "metric-server_3_6_9"
	server2 := "other-server_2_0_0"
	server2Namespace := "other-namespace"

	s.assertUploadIntent(server, server2, server2Namespace)
}

func (s *CloudReconcilerTestSuite) TestAppliedIntentsRetryWhenUploadFailed() {
	server := "test-server"

	clientIntents := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: server,
				},
			},
		},
	}

	expectedIntentInNamespace := graphqlclient.IntentInput{
		ClientName:      lo.ToPtr(clientName),
		ServerName:      lo.ToPtr(server),
		Namespace:       lo.ToPtr(testNamespace),
		ServerNamespace: lo.ToPtr(testNamespace),
		Type:            nil,
		Topics:          nil,
		Resources:       nil,
	}
	expectedIntents := []graphqlclient.IntentInput{
		expectedIntentInNamespace,
	}

	emptyList := otterizev1alpha2.ClientIntentsList{}
	clientIntentsList := otterizev1alpha2.ClientIntentsList{
		Items: []otterizev1alpha2.ClientIntents{clientIntents},
	}

	s.client.EXPECT().List(gomock.Any(), gomock.Eq(&emptyList), &client.ListOptions{Namespace: testNamespace}).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ClientIntentsList, opts *client.ListOptions) error {
			clientIntentsList.DeepCopyInto(list)
			return nil
		})

	expectedNamespace := lo.ToPtr(testNamespace)
	s.mockCloudClient.EXPECT().ReportAppliedIntents(gomock.Any(), expectedNamespace, operator_cloud_client.GetMatcher(expectedIntents)).
		Return(errors.New("upload failed, try again later, ok? cool")).Times(1)

	objName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      intentsObjectName,
	}
	req := ctrl.Request{NamespacedName: objName}
	res, err := s.Reconciler.Reconcile(context.Background(), req)
	// We get an error and the operator will try sending again
	s.Require().Error(err)
	s.Require().Equal(ctrl.Result{}, res)
}

func (s *CloudReconcilerTestSuite) TestUploadKafkaType() {
	server := "test-server"

	clientIntents := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
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
						}},
					},
				},
			},
		},
	}

	expectedIntent := graphqlclient.IntentInput{
		ClientName:      lo.ToPtr(clientName),
		ServerName:      lo.ToPtr(server),
		Namespace:       lo.ToPtr(testNamespace),
		ServerNamespace: lo.ToPtr(testNamespace),
		Type:            lo.ToPtr(graphqlclient.IntentTypeKafka),
		Topics: []*graphqlclient.KafkaConfigInput{{
			Name: lo.ToPtr("test-topic"),
			Operations: []*graphqlclient.KafkaOperation{
				lo.ToPtr(graphqlclient.KafkaOperationDelete),
				lo.ToPtr(graphqlclient.KafkaOperationCreate),
			},
		}},
	}

	s.assertReportedIntents(clientIntents, []graphqlclient.IntentInput{expectedIntent})
}

func (s *CloudReconcilerTestSuite) TestHTTPUpload() {
	serviceAccountName := "test-service-account"
	server := "test-server"
	clientIntents := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
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

	expectedIntent := graphqlclient.IntentInput{
		ClientName:      lo.ToPtr(clientName),
		ServerName:      lo.ToPtr(server),
		Namespace:       lo.ToPtr(testNamespace),
		ServerNamespace: lo.ToPtr(testNamespace),
		Type:            lo.ToPtr(graphqlclient.IntentTypeHttp),
		Resources: []*graphqlclient.HTTPConfigInput{
			{
				Path:    lo.ToPtr("/login"),
				Methods: []*graphqlclient.HTTPMethod{lo.ToPtr(graphqlclient.HTTPMethodGet), lo.ToPtr(graphqlclient.HTTPMethodPost)},
			},
			{
				Path:    lo.ToPtr("/logout"),
				Methods: []*graphqlclient.HTTPMethod{lo.ToPtr(graphqlclient.HTTPMethodPost)},
			},
		},
		Status: &graphqlclient.IntentStatusInput{
			IstioStatus: &graphqlclient.IstioStatusInput{
				ServiceAccountName:     lo.ToPtr(serviceAccountName),
				IsServiceAccountShared: lo.ToPtr(false),
				IsClientMissingSidecar: lo.ToPtr(false),
				IsServerMissingSidecar: lo.ToPtr(false),
			},
		},
	}

	s.assertReportedIntents(clientIntents, []graphqlclient.IntentInput{expectedIntent})
}

func (s *CloudReconcilerTestSuite) TestIntentStatusFormattingError_MissingSharedSA() {
	serviceAccountName := "test-service-account"
	server := "test-server"
	clientIntents := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
			Annotations: map[string]string{
				otterizev1alpha2.OtterizeClientServiceAccountAnnotation: serviceAccountName,
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
					},
				},
			},
		},
	}

	s.expectReconcilerError(clientIntents)
}

func (s *CloudReconcilerTestSuite) TestIntentStatusFormattingError_MissingSidecar() {
	serviceAccountName := "test-service-account"
	server := "test-server"
	clientIntents := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
			Annotations: map[string]string{
				otterizev1alpha2.OtterizeClientServiceAccountAnnotation: serviceAccountName,
				otterizev1alpha2.OtterizeSharedServiceAccountAnnotation: "false",
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
					},
				},
			},
		},
	}

	s.expectReconcilerError(clientIntents)
}

func (s *CloudReconcilerTestSuite) TestIntentStatusFormattingError_BadFormatSharedSA() {
	serviceAccountName := "test-service-account"
	server := "test-server"
	clientIntents := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
			Annotations: map[string]string{
				otterizev1alpha2.OtterizeClientServiceAccountAnnotation: serviceAccountName,
				otterizev1alpha2.OtterizeSharedServiceAccountAnnotation: "sharing-is-caring",
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
					},
				},
			},
		},
	}

	s.expectReconcilerError(clientIntents)
}

func (s *CloudReconcilerTestSuite) TestIntentStatusFormattingError_BadFormatSidecar() {
	serviceAccountName := "test-service-account"
	server := "test-server"
	clientIntents := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
			Annotations: map[string]string{
				otterizev1alpha2.OtterizeClientServiceAccountAnnotation: serviceAccountName,
				otterizev1alpha2.OtterizeSharedServiceAccountAnnotation: "false",
				otterizev1alpha2.OtterizeMissingSidecarAnnotation:       "I-don't-see-any-sidecar",
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
					},
				},
			},
		},
	}

	s.expectReconcilerError(clientIntents)
}

func (s *CloudReconcilerTestSuite) expectReconcilerError(clientIntents otterizev1alpha2.ClientIntents) {
	emptyList := otterizev1alpha2.ClientIntentsList{}
	clientIntentsList := otterizev1alpha2.ClientIntentsList{
		Items: []otterizev1alpha2.ClientIntents{clientIntents},
	}

	s.client.EXPECT().List(gomock.Any(), gomock.Eq(&emptyList), &client.ListOptions{Namespace: testNamespace}).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ClientIntentsList, opts *client.ListOptions) error {
			clientIntentsList.DeepCopyInto(list)
			return nil
		})

	objName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      intentsObjectName,
	}
	req := ctrl.Request{NamespacedName: objName}
	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.Require().Error(err)
	s.Require().Equal(ctrl.Result{}, res)
}

func (s *CloudReconcilerTestSuite) assertReportedIntents(clientIntents otterizev1alpha2.ClientIntents, expectedIntents []graphqlclient.IntentInput) {
	emptyList := otterizev1alpha2.ClientIntentsList{}
	clientIntentsList := otterizev1alpha2.ClientIntentsList{
		Items: []otterizev1alpha2.ClientIntents{clientIntents},
	}

	s.client.EXPECT().List(gomock.Any(), gomock.Eq(&emptyList), &client.ListOptions{Namespace: testNamespace}).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ClientIntentsList, opts *client.ListOptions) error {
			clientIntentsList.DeepCopyInto(list)
			return nil
		})

	expectedNamespace := lo.ToPtr(testNamespace)
	s.mockCloudClient.EXPECT().ReportAppliedIntents(gomock.Any(), expectedNamespace, operator_cloud_client.GetMatcher(expectedIntents)).Return(nil).Times(1)

	objName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      intentsObjectName,
	}
	req := ctrl.Request{NamespacedName: objName}
	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Equal(ctrl.Result{}, res)
}

func (s *CloudReconcilerTestSuite) TestNamespaceParseSuccess() {
	serverName := "server.other-namespace"
	intent := &otterizev1alpha2.Intent{Name: serverName}

	cloudIntent := intent.ConvertToCloudFormat(testNamespace, clientName)

	s.Require().Equal(lo.FromPtr(cloudIntent.Namespace), testNamespace)
	s.Require().Equal(lo.FromPtr(cloudIntent.ClientName), clientName)
	s.Require().Equal(lo.FromPtr(cloudIntent.ServerName), "server")
	s.Require().Equal(lo.FromPtr(cloudIntent.ServerNamespace), "other-namespace")
}

func (s *CloudReconcilerTestSuite) TestTargetNamespaceAsSourceNamespace() {
	serverName := "server"
	intent := &otterizev1alpha2.Intent{Name: serverName}
	cloudIntent := intent.ConvertToCloudFormat(testNamespace, clientName)
	s.Require().Equal(lo.FromPtr(cloudIntent.ServerNamespace), testNamespace)
}

func (s *CloudReconcilerTestSuite) expectNoEvent() {
	select {
	case event := <-s.recorder.Events:
		s.Fail("Unexpected event found", event)
	default:
		// Amazing, no events left behind!
	}
}

func TestCloudReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(CloudReconcilerTestSuite))
}
