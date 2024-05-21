package database

import (
	"context"
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/shared/testbase"
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
	databaseInstance  string = "db-instance"
	tableName         string = "test-table"
	dbName            string = "testdb"
	dbAddress         string = "https://test.this.db:5432"
)

type DatabaseReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	Reconciler     *DatabaseReconciler
	client         *mocks.MockClient
	namespacedName types.NamespacedName
}

func (s *DatabaseReconcilerTestSuite) SetupTest() {
	s.Controller = gomock.NewController(s.T())
	s.client = mocks.NewMockClient(s.Controller)
	s.Reconciler = NewDatabaseReconciler(
		s.client,
		&runtime.Scheme{},
	)
	s.Reconciler.clusterID = lo.ToPtr("abc-def-ghi")
	s.Recorder = record.NewFakeRecorder(100)
	s.Reconciler.Recorder = s.Recorder

	s.namespacedName = types.NamespacedName{
		Namespace: testNamespace,
		Name:      intentsObjectName,
	}
}

func (s *DatabaseReconcilerTestSuite) TestPGServerConfNotMatching() {
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
					Name: databaseInstance,
					Type: otterizev1alpha3.IntentTypeDatabase,
					DatabaseResources: []otterizev1alpha3.DatabaseResource{{
						DatabaseName: dbName,
						Table:        tableName,
						Operations: []otterizev1alpha3.DatabaseOperation{
							otterizev1alpha3.DatabaseOperationSelect,
							otterizev1alpha3.DatabaseOperationInsert,
						},
					}},
				},
			},
		},
	}

	pgServerConf := otterizev1alpha3.PostgreSQLServerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-other", databaseInstance),
			Namespace: testNamespace,
		},
		Spec: otterizev1alpha3.PostgreSQLServerConfigSpec{
			Address: dbAddress,
			Credentials: otterizev1alpha3.DatabaseCredentials{
				Username: "shhhhh",
				Password: "secret",
			},
		},
	}

	_, err := s.reconcileWithExpectedResources(clientIntents, []otterizev1alpha3.PostgreSQLServerConfig{pgServerConf})
	s.Require().Error(err, "Can't reach the server")
	s.Require().Empty(ctrl.Result{})
	s.ExpectEvent(ReasonMissingDBServerConfig)
	s.ExpectEvent(ReasonErrorConnectingToDatabase)
	s.ExpectEvent(ReasonApplyingDatabaseIntentsFailed)
}

func (s *DatabaseReconcilerTestSuite) TestNoPGServerConf() {
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
					Name: databaseInstance,
					Type: otterizev1alpha3.IntentTypeDatabase,
					DatabaseResources: []otterizev1alpha3.DatabaseResource{{
						DatabaseName: dbName,
						Table:        tableName,
						Operations: []otterizev1alpha3.DatabaseOperation{
							otterizev1alpha3.DatabaseOperationSelect,
							otterizev1alpha3.DatabaseOperationInsert,
						},
					}},
				},
			},
		}}

	_, err := s.reconcileWithExpectedResources(clientIntents, []otterizev1alpha3.PostgreSQLServerConfig{})
	s.Require().NoError(err) // Although no PGServerConf, we don't return error - just record an event
	s.Require().Empty(ctrl.Result{})
	s.ExpectEvent(ReasonMissingDBServerConfig)
	s.ExpectEvent(ReasonAppliedDatabaseIntents)
}

func (s *DatabaseReconcilerTestSuite) reconcileWithExpectedResources(clientIntents otterizev1alpha3.ClientIntents, pgServerConfigs []otterizev1alpha3.PostgreSQLServerConfig) (ctrl.Result, error) {
	s.client.EXPECT().Get(gomock.Any(), gomock.Eq(s.namespacedName), gomock.Eq(&otterizev1alpha3.ClientIntents{})).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha3.ClientIntents, options ...client.ListOption) error {
			clientIntents.DeepCopyInto(intents)
			return nil
		})

	s.client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha3.PostgreSQLServerConfigList{}), gomock.Any()).DoAndReturn(
		func(ctx context.Context, pgServerConfList *otterizev1alpha3.PostgreSQLServerConfigList, options ...client.ListOption) error {
			pgServerConfList.Items = pgServerConfigs
			return nil
		})

	s.client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha3.MySQLServerConfigList{}), gomock.Any()).DoAndReturn(
		func(ctx context.Context, mySQLServerConfList *otterizev1alpha3.MySQLServerConfigList, options ...client.ListOption) error {
			mySQLServerConfList.Items = []otterizev1alpha3.MySQLServerConfig{}
			return nil
		})

	req := ctrl.Request{NamespacedName: s.namespacedName}

	return s.Reconciler.Reconcile(context.Background(), req)
}

func TestDatabaseReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(DatabaseReconcilerTestSuite))
}
