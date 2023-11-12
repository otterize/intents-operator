package exp

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	otterizecloudmocks "github.com/otterize/intents-operator/src/shared/otterizecloud/mocks"
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

const (
	testNamespace     string = "test-namespace"
	intentsObjectName string = "test-client-intents"
	clientName        string = "test-client"
	integrationName   string = "test-integration"
	tableName         string = "test-table"
)

type DatabaseReconcilerTestSuite struct {
	suite.Suite
	Reconciler      *DatabaseReconciler
	recorder        *record.FakeRecorder
	client          *mocks.MockClient
	mockCloudClient *otterizecloudmocks.MockCloudClient
	namespacedName  types.NamespacedName
}

func (s *DatabaseReconcilerTestSuite) SetupTest() {
	controller := gomock.NewController(s.T())
	s.client = mocks.NewMockClient(controller)
	s.mockCloudClient = otterizecloudmocks.NewMockCloudClient(controller)

	s.Reconciler = NewDatabaseReconciler(
		s.client,
		&runtime.Scheme{},
		s.mockCloudClient,
	)

	s.recorder = record.NewFakeRecorder(100)
	s.Reconciler.Recorder = s.recorder

	s.namespacedName = types.NamespacedName{
		Namespace: testNamespace,
		Name:      intentsObjectName,
	}
}

func (s *DatabaseReconcilerTestSuite) TearDownTest() {
	s.Reconciler = nil
	s.expectNoEvent()
}

func (s *DatabaseReconcilerTestSuite) expectNoEvent() {
	select {
	case event := <-s.recorder.Events:
		s.Fail("Unexpected event found", event)
	default:
		// Amazing, no events left behind!
	}
}

func (s *DatabaseReconcilerTestSuite) TestSimpleDatabase() {
	clientIntents := otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev1alpha3.IntentsSpec{
			Service: otterizev1alpha3.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha3.Intent{
				{
					Name: integrationName,
					Type: otterizev1alpha3.IntentTypeDatabase,
					DatabaseResources: []otterizev1alpha3.DatabaseResource{{
						Table: tableName,
						Operations: []otterizev1alpha3.DatabaseOperation{
							otterizev1alpha3.DatabaseOperationSelect,
							otterizev1alpha3.DatabaseOperationInsert,
						},
					}},
				},
			},
		},
	}

	expectedIntents := []graphqlclient.IntentInput{{
		ClientName:      lo.ToPtr(clientName),
		ServerName:      lo.ToPtr(integrationName),
		Namespace:       lo.ToPtr(testNamespace),
		ServerNamespace: lo.ToPtr(testNamespace),
		Type:            lo.ToPtr(graphqlclient.IntentTypeDatabase),
		DatabaseResources: []*graphqlclient.DatabaseConfigInput{{
			Table: lo.ToPtr(tableName),
			Operations: []*graphqlclient.DatabaseOperation{
				lo.ToPtr(graphqlclient.DatabaseOperationSelect),
				lo.ToPtr(graphqlclient.DatabaseOperationInsert),
			},
		}},
	}}

	s.assertAppliedDatabaseIntents(clientIntents, expectedIntents)
}

func (s *DatabaseReconcilerTestSuite) TestDontReportIntentsWithoutDatabaseType() {
	clientIntents := otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev1alpha3.IntentsSpec{
			Service: otterizev1alpha3.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha3.Intent{
				{
					Name: "server",
				},
			},
		},
	}

	emptyIntents := otterizev1alpha3.ClientIntents{}

	s.client.EXPECT().Get(gomock.Any(), gomock.Eq(s.namespacedName), gomock.Eq(&emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha3.ClientIntents, options ...client.ListOption) error {
			clientIntents.DeepCopyInto(intents)
			return nil
		})

	req := ctrl.Request{NamespacedName: s.namespacedName}

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Equal(ctrl.Result{}, res)
}

func (s *DatabaseReconcilerTestSuite) TestNoSpecs() {
	clientIntents := otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},
	}

	s.expectHandleReconcilationErrorGracefully(clientIntents)
}

func (s *DatabaseReconcilerTestSuite) expectHandleReconcilationErrorGracefully(clientIntents otterizev1alpha3.ClientIntents) {
	emptyIntents := otterizev1alpha3.ClientIntents{}

	s.client.EXPECT().Get(gomock.Any(), gomock.Eq(s.namespacedName), gomock.Eq(&emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha3.ClientIntents, options ...client.ListOption) error {
			clientIntents.DeepCopyInto(intents)
			return nil
		})

	req := ctrl.Request{NamespacedName: s.namespacedName}
	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Equal(ctrl.Result{}, res)
}

func (s *DatabaseReconcilerTestSuite) assertAppliedDatabaseIntents(clientIntents otterizev1alpha3.ClientIntents, expectedIntents []graphqlclient.IntentInput) {
	emptyIntents := otterizev1alpha3.ClientIntents{}

	s.client.EXPECT().Get(gomock.Any(), gomock.Eq(s.namespacedName), gomock.Eq(&emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha3.ClientIntents, options ...client.ListOption) error {
			clientIntents.DeepCopyInto(intents)
			return nil
		})

	req := ctrl.Request{NamespacedName: s.namespacedName}

	s.mockCloudClient.EXPECT().ApplyDatabaseIntent(gomock.Any(), expectedIntents, graphqlclient.DBPermissionChangeApply).Return(nil).Times(1)

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Equal(ctrl.Result{}, res)
}

func TestDatabaseReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(DatabaseReconcilerTestSuite))
}
