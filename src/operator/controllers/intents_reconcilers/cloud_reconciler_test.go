package intents_reconcilers

import (
	"context"
	"errors"
	"fmt"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/mocks"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"
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
	clientIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: server},
				},
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: server2FullName},
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

	clientIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{
						Name: server,
					},
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

	emptyList := otterizev2alpha1.ClientIntentsList{}
	clientIntentsList := otterizev2alpha1.ClientIntentsList{
		Items: []otterizev2alpha1.ClientIntents{clientIntents},
	}

	s.client.EXPECT().List(gomock.Any(), gomock.Eq(&emptyList), &client.ListOptions{Namespace: testNamespace}).DoAndReturn(
		func(ctx context.Context, list *otterizev2alpha1.ClientIntentsList, opts *client.ListOptions) error {
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

	clientIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kafka: &otterizev2alpha1.KafkaTarget{
						Name: server,
						Topics: []otterizev2alpha1.KafkaTopic{
							{
								Name: "test-topic",
								Operations: []otterizev2alpha1.KafkaOperation{
									otterizev2alpha1.KafkaOperationCreate,
									otterizev2alpha1.KafkaOperationDelete,
								},
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
	clientIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
			Annotations: map[string]string{
				otterizev2alpha1.OtterizeClientServiceAccountAnnotation: serviceAccountName,
				otterizev2alpha1.OtterizeSharedServiceAccountAnnotation: "false",
				otterizev2alpha1.OtterizeMissingSidecarAnnotation:       "false",
			},
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{
						Name: server,
						HTTP: []otterizev2alpha1.HTTPTarget{
							{
								Path: "/login",
								Methods: []otterizev2alpha1.HTTPMethod{
									otterizev2alpha1.HTTPMethodGet,
									otterizev2alpha1.HTTPMethodPost,
								},
							},
							{
								Path: "/logout",
								Methods: []otterizev2alpha1.HTTPMethod{
									otterizev2alpha1.HTTPMethodPost,
								},
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

func (s *CloudReconcilerTestSuite) TestInternetUpload() {
	server := otterizev2alpha1.OtterizeInternetTargetName
	clientIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Internet: &otterizev2alpha1.Internet{
						Ips: []string{"1.1.1.1", "2.2.2.0/24"},
					},
				},
				{
					Internet: &otterizev2alpha1.Internet{
						Ips:   []string{"3.3.3.3"},
						Ports: []int{443},
					},
				},
			},
		},
	}

	expectedIntentA := graphqlclient.IntentInput{
		ClientName:      lo.ToPtr(clientName),
		ServerName:      lo.ToPtr(server),
		Namespace:       lo.ToPtr(testNamespace),
		ServerNamespace: lo.ToPtr(testNamespace),
		Type:            lo.ToPtr(graphqlclient.IntentTypeInternet),
		Internet: lo.ToPtr(graphqlclient.InternetConfigInput{
			Ips: []*string{
				lo.ToPtr("1.1.1.1"),
				lo.ToPtr("2.2.2.0/24"),
			},
			Ports: nil,
		}),
	}

	expectedIntentB := graphqlclient.IntentInput{
		ClientName:      lo.ToPtr(clientName),
		ServerName:      lo.ToPtr(server),
		Namespace:       lo.ToPtr(testNamespace),
		ServerNamespace: lo.ToPtr(testNamespace),
		Type:            lo.ToPtr(graphqlclient.IntentTypeInternet),
		Internet: lo.ToPtr(graphqlclient.InternetConfigInput{
			Ips: []*string{
				lo.ToPtr("3.3.3.3"),
			},
			Ports: []*int{
				lo.ToPtr(443),
			},
		}),
	}

	s.assertReportedIntents(clientIntents, []graphqlclient.IntentInput{expectedIntentA, expectedIntentB})
}

func (s *CloudReconcilerTestSuite) TestInternetUploadWithDNS() {
	server := otterizev2alpha1.OtterizeInternetTargetName
	clientIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Internet: &otterizev2alpha1.Internet{
						Domains: []string{"test-dns.com"},
						Ips:     []string{"1.1.1.1", "2.2.2.0/24"},
					},
				},
			},
		},
	}

	expectedIntentA := graphqlclient.IntentInput{
		ClientName:      lo.ToPtr(clientName),
		ServerName:      lo.ToPtr(server),
		Namespace:       lo.ToPtr(testNamespace),
		ServerNamespace: lo.ToPtr(testNamespace),
		Type:            lo.ToPtr(graphqlclient.IntentTypeInternet),
		Internet: lo.ToPtr(graphqlclient.InternetConfigInput{
			Domains: []*string{lo.ToPtr("test-dns.com")},
			Ips: []*string{
				lo.ToPtr("1.1.1.1"),
				lo.ToPtr("2.2.2.0/24"),
			},
			Ports: nil,
		}),
	}

	s.assertReportedIntents(clientIntents, []graphqlclient.IntentInput{expectedIntentA})
}

func (s *CloudReconcilerTestSuite) TestInternetUploadDomainsOnly() {
	server := otterizev2alpha1.OtterizeInternetTargetName
	clientIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Internet: &otterizev2alpha1.Internet{
						Domains: []string{"test-dns.com"},
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
		Type:            lo.ToPtr(graphqlclient.IntentTypeInternet),
		Internet: lo.ToPtr(graphqlclient.InternetConfigInput{
			Domains: []*string{
				lo.ToPtr("test-dns.com"),
			},
			Ports: nil,
		}),
	}
	s.assertReportedIntents(clientIntents, []graphqlclient.IntentInput{expectedIntent})
}

func (s *CloudReconcilerTestSuite) TestIntentStatusFormattingError_MissingSharedSA() {
	serviceAccountName := "test-service-account"
	server := "test-server"
	clientIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
			Annotations: map[string]string{
				otterizev2alpha1.OtterizeClientServiceAccountAnnotation: serviceAccountName,
				otterizev2alpha1.OtterizeMissingSidecarAnnotation:       "false",
			},
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{
						Name: server,
						HTTP: []otterizev2alpha1.HTTPTarget{
							{
								Path: "/login",
								Methods: []otterizev2alpha1.HTTPMethod{
									otterizev2alpha1.HTTPMethodGet,
									otterizev2alpha1.HTTPMethodPost,
								},
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
	clientIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
			Annotations: map[string]string{
				otterizev2alpha1.OtterizeClientServiceAccountAnnotation: serviceAccountName,
				otterizev2alpha1.OtterizeSharedServiceAccountAnnotation: "false",
			},
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{
						Name: server,
						HTTP: []otterizev2alpha1.HTTPTarget{
							{
								Path: "/login",
								Methods: []otterizev2alpha1.HTTPMethod{
									otterizev2alpha1.HTTPMethodGet,
									otterizev2alpha1.HTTPMethodPost,
								},
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

	clientIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
			Annotations: map[string]string{
				otterizev2alpha1.OtterizeClientServiceAccountAnnotation: serviceAccountName,
				otterizev2alpha1.OtterizeSharedServiceAccountAnnotation: "sharing-is-caring",
				otterizev2alpha1.OtterizeMissingSidecarAnnotation:       "false",
			},
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{
						Name: server,
						HTTP: []otterizev2alpha1.HTTPTarget{
							{
								Path: "/login",
								Methods: []otterizev2alpha1.HTTPMethod{
									otterizev2alpha1.HTTPMethodGet,
									otterizev2alpha1.HTTPMethodPost,
								},
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
	clientIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
			Annotations: map[string]string{
				otterizev2alpha1.OtterizeClientServiceAccountAnnotation: serviceAccountName,
				otterizev2alpha1.OtterizeSharedServiceAccountAnnotation: "false",
				otterizev2alpha1.OtterizeMissingSidecarAnnotation:       "I-don't-see-any-sidecar",
			},
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{
						Name: server,
						HTTP: []otterizev2alpha1.HTTPTarget{
							{
								Path: "/login",
								Methods: []otterizev2alpha1.HTTPMethod{
									otterizev2alpha1.HTTPMethodGet,
									otterizev2alpha1.HTTPMethodPost,
								},
							},
						},
					},
				},
			},
		},
	}

	s.expectReconcilerError(clientIntents)
}

func (s *CloudReconcilerTestSuite) expectReconcilerError(clientIntents otterizev2alpha1.ClientIntents) {
	emptyList := otterizev2alpha1.ClientIntentsList{}
	clientIntentsList := otterizev2alpha1.ClientIntentsList{
		Items: []otterizev2alpha1.ClientIntents{clientIntents},
	}

	s.client.EXPECT().List(gomock.Any(), gomock.Eq(&emptyList), &client.ListOptions{Namespace: testNamespace}).DoAndReturn(
		func(ctx context.Context, list *otterizev2alpha1.ClientIntentsList, opts *client.ListOptions) error {
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

func (s *CloudReconcilerTestSuite) assertReportedIntents(clientIntents otterizev2alpha1.ClientIntents, expectedIntents []graphqlclient.IntentInput) {
	emptyList := otterizev2alpha1.ClientIntentsList{}
	clientIntentsList := otterizev2alpha1.ClientIntentsList{
		Items: []otterizev2alpha1.ClientIntents{clientIntents},
	}

	s.client.EXPECT().List(gomock.Any(), gomock.Eq(&emptyList), &client.ListOptions{Namespace: testNamespace}).DoAndReturn(
		func(ctx context.Context, list *otterizev2alpha1.ClientIntentsList, opts *client.ListOptions) error {
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

func (s *CloudReconcilerTestSuite) TestUploadIntentsDeletion() {
	deletedIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:              intentsObjectName,
			Namespace:         testNamespace,
			DeletionTimestamp: lo.ToPtr(metav1.Date(2021, 6, 13, 0, 0, 0, 0, time.UTC)),
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: "test-server"},
				},
			},
		},
	}

	emptyInput := make([]*graphqlclient.IntentInput, 0)
	emptyList := otterizev2alpha1.ClientIntentsList{}
	clientIntentsList := otterizev2alpha1.ClientIntentsList{
		Items: []otterizev2alpha1.ClientIntents{deletedIntents},
	}

	s.client.EXPECT().List(gomock.Any(), gomock.Eq(&emptyList), &client.ListOptions{Namespace: testNamespace}).DoAndReturn(
		func(ctx context.Context, list *otterizev2alpha1.ClientIntentsList, opts *client.ListOptions) error {
			clientIntentsList.DeepCopyInto(list)
			return nil
		})

	expectedNamespace := lo.ToPtr(testNamespace)

	s.mockCloudClient.EXPECT().ReportAppliedIntents(gomock.Any(), expectedNamespace, emptyInput).Return(nil).Times(1)

	objName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      intentsObjectName,
	}
	req := ctrl.Request{NamespacedName: objName}
	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Equal(ctrl.Result{}, res)
}

func (s *CloudReconcilerTestSuite) TestUploadIntentsOnlyOneDeleted() {
	deletedIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "deleted-intents",
			Namespace:         testNamespace,
			DeletionTimestamp: lo.ToPtr(metav1.Date(2021, 6, 13, 0, 0, 0, 0, time.UTC)),
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: "deleted-client",
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: "test-server"},
				},
			},
		},
	}

	clientIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: "test-server"},
				},
			},
		},
	}

	emptyList := otterizev2alpha1.ClientIntentsList{}
	clientIntentsList := otterizev2alpha1.ClientIntentsList{
		Items: []otterizev2alpha1.ClientIntents{
			deletedIntents,
			clientIntents,
		},
	}

	expectedIntent := graphqlclient.IntentInput{
		ClientName:      lo.ToPtr(clientName),
		ServerName:      lo.ToPtr("test-server"),
		Namespace:       lo.ToPtr(testNamespace),
		ServerNamespace: lo.ToPtr(testNamespace),
	}

	s.client.EXPECT().List(gomock.Any(), gomock.Eq(&emptyList), &client.ListOptions{Namespace: testNamespace}).DoAndReturn(
		func(ctx context.Context, list *otterizev2alpha1.ClientIntentsList, opts *client.ListOptions) error {
			clientIntentsList.DeepCopyInto(list)
			return nil
		})

	expectedNamespace := lo.ToPtr(testNamespace)

	s.mockCloudClient.EXPECT().ReportAppliedIntents(gomock.Any(), expectedNamespace, operator_cloud_client.GetMatcher([]graphqlclient.IntentInput{expectedIntent})).Return(nil).Times(1)

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
	intent := &otterizev2alpha1.Target{Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: serverName}}

	cloudIntent, err := intent.ConvertToCloudFormat(context.Background(), s.client, serviceidentity.ServiceIdentity{Name: clientName, Namespace: testNamespace})
	s.Require().NoError(err)

	s.Require().Equal(lo.FromPtr(cloudIntent.Namespace), testNamespace)
	s.Require().Equal(lo.FromPtr(cloudIntent.ClientName), clientName)
	s.Require().Equal(lo.FromPtr(cloudIntent.ServerName), "server")
	s.Require().Equal(lo.FromPtr(cloudIntent.ServerNamespace), "other-namespace")
}

// TestIntents With Kafka Target
func (s *CloudReconcilerTestSuite) TestKafkaTarget() {
	serverName := "server.other-namespace"
	intent := &otterizev2alpha1.Target{Kafka: &otterizev2alpha1.KafkaTarget{Name: serverName, Topics: []otterizev2alpha1.KafkaTopic{{Name: "test", Operations: []otterizev2alpha1.KafkaOperation{otterizev2alpha1.KafkaOperationConsume}}}}}
	cloudIntent, err := intent.ConvertToCloudFormat(context.Background(), s.client, serviceidentity.ServiceIdentity{Name: clientName, Namespace: testNamespace})
	s.Require().NoError(err)
	s.Require().Equal(lo.FromPtr(cloudIntent.ServerName), "server")
	s.Require().Equal(lo.FromPtr(cloudIntent.ServerNamespace), "other-namespace")
	s.Require().Len(cloudIntent.Topics, 1)
	s.Require().Equal(lo.FromPtr(cloudIntent.Topics[0].Name), "test")
	s.Require().Len(cloudIntent.Topics[0].Operations, 1)
	s.Require().Equal(lo.FromPtr(cloudIntent.Topics[0].Operations[0]), graphqlclient.KafkaOperationConsume)

}

func (s *CloudReconcilerTestSuite) TestTargetNamespaceAsSourceNamespace() {
	serverName := "server"
	intent := &otterizev2alpha1.Target{Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: serverName}}
	cloudIntent, err := intent.ConvertToCloudFormat(context.Background(), s.client, serviceidentity.ServiceIdentity{Name: clientName, Namespace: testNamespace})
	s.Require().NoError(err)
	s.Require().Equal(lo.FromPtr(cloudIntent.ServerNamespace), testNamespace)
}

// TestReportKindAndAlias
func (s *CloudReconcilerTestSuite) TestReportKindAndAlias() {
	serverName := "server"
	intent := &otterizev2alpha1.Target{Service: &otterizev2alpha1.ServiceTarget{Name: serverName}}
	s.client.EXPECT().Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: serverName, Namespace: testNamespace}), gomock.AssignableToTypeOf(&v1.Service{})).DoAndReturn(func(ctx context.Context, _ any, obj *v1.Service, _ ...any) error {
		obj.Name = serverName
		obj.Namespace = testNamespace
		obj.Spec.Selector = map[string]string{"app": "test"}
		return nil
	})
	s.client.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, list *v1.PodList, _ ...any) error {
		list.Items = []v1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: testNamespace, OwnerReferences: []metav1.OwnerReference{{Name: serverName, Kind: "Deployment"}}}}}
		return nil
	})

	s.client.EXPECT().Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: serverName, Namespace: testNamespace}), gomock.AssignableToTypeOf(&unstructured.Unstructured{})).DoAndReturn(func(ctx context.Context, _ any, obj *unstructured.Unstructured, _ ...any) error {
		obj.SetName(serverName)
		obj.SetNamespace(testNamespace)
		obj.SetKind("Deployment")
		return nil
	})

	cloudIntent, err := intent.ConvertToCloudFormat(context.Background(), s.client, serviceidentity.ServiceIdentity{Name: clientName, Namespace: testNamespace, Kind: "StatefulSet"})
	s.Require().NoError(err)
	s.Require().Equal(lo.FromPtr(cloudIntent.ServerWorkloadKind), "Deployment")
	s.Require().Equal(lo.FromPtr(cloudIntent.ServerAlias), graphqlclient.ServerAliasInput{Name: lo.ToPtr(serverName + "." + testNamespace), Kind: lo.ToPtr("Service")})
	s.Require().Equal(lo.FromPtr(cloudIntent.ClientWorkloadKind), "StatefulSet")
}

func (s *CloudReconcilerTestSuite) TestReportTargetKubernetesAPIServiceWithNoSelector() {
	serverName := "kubernetes"
	serverNamespace := "default"
	intent := &otterizev2alpha1.Target{Service: &otterizev2alpha1.ServiceTarget{Name: fmt.Sprint(serverName, ".", serverNamespace)}}
	cloudIntent, err := intent.ConvertToCloudFormat(context.Background(), s.client, serviceidentity.ServiceIdentity{Name: clientName, Namespace: testNamespace})
	s.Require().NoError(err)
	s.Require().Equal(lo.FromPtr(cloudIntent.ServerWorkloadKind), "Service")
	s.Require().Equal(lo.FromPtr(cloudIntent.ServerAlias), graphqlclient.ServerAliasInput{Name: lo.ToPtr(serverName + "." + serverNamespace), Kind: lo.ToPtr("Service")})
	s.Require().Equal(lo.FromPtr(cloudIntent.ServerNamespace), serverNamespace)
	s.Require().Equal(lo.FromPtr(cloudIntent.ServerName), serverName)
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
