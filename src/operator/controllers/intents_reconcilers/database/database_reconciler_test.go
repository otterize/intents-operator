package database

import (
	"context"
	"fmt"
	otterizev2 "github.com/otterize/intents-operator/src/operator/api/v2"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/shared/clusterutils"
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
	clientIntents := otterizev2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2.IntentsSpec{
			Workload: otterizev2.Workload{
				Name: clientName,
			},
			Targets: []otterizev2.Target{
				{
					SQL: &otterizev2.SQLTarget{
						Name: databaseInstance,
						Privileges: []otterizev2.SQLPrivileges{{
							DatabaseName: dbName,
							Table:        tableName,
							Operations: []otterizev2.DatabaseOperation{
								otterizev2.DatabaseOperationSelect,
								otterizev2.DatabaseOperationInsert,
							},
						}},
					},
				},
			},
		},
	}

	pgServerConf := otterizev2.PostgreSQLServerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-other", databaseInstance),
			Namespace: testNamespace,
		},
		Spec: otterizev2.PostgreSQLServerConfigSpec{
			Address: dbAddress,
			Credentials: otterizev2.DatabaseCredentials{
				Username: "shhhhh",
				Password: "secret",
			},
		},
	}

	_, err := s.reconcileWithExpectedResources(clientIntents, []otterizev2.PostgreSQLServerConfig{pgServerConf})
	s.Require().Error(err, "Can't reach the server")
	s.Require().Empty(ctrl.Result{})
	s.ExpectEvent(ReasonMissingDBServerConfig)
	s.ExpectEvent(ReasonErrorConnectingToDatabase)
	s.ExpectEvent(ReasonApplyingDatabaseIntentsFailed)
}

func (s *DatabaseReconcilerTestSuite) TestNoPGServerConf() {
	clientIntents := otterizev2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2.IntentsSpec{
			Workload: otterizev2.Workload{
				Name: clientName,
			},
			Targets: []otterizev2.Target{
				{
					SQL: &otterizev2.SQLTarget{
						Name: databaseInstance,
						Privileges: []otterizev2.SQLPrivileges{{
							DatabaseName: dbName,
							Table:        tableName,
							Operations: []otterizev2.DatabaseOperation{
								otterizev2.DatabaseOperationSelect,
								otterizev2.DatabaseOperationInsert,
							},
						}},
					},
				},
			},
		}}

	_, err := s.reconcileWithExpectedResources(clientIntents, []otterizev2.PostgreSQLServerConfig{})
	s.Require().NoError(err) // Although no PGServerConf, we don't return error - just record an event
	s.Require().Empty(ctrl.Result{})
	s.ExpectEvent(ReasonMissingDBServerConfig)
	s.ExpectEvent(ReasonAppliedDatabaseIntents)
}

func (s *DatabaseReconcilerTestSuite) reconcileWithExpectedResources(clientIntents otterizev2.ClientIntents, pgServerConfigs []otterizev2.PostgreSQLServerConfig) (ctrl.Result, error) {
	s.client.EXPECT().Get(gomock.Any(), gomock.Eq(s.namespacedName), gomock.Eq(&otterizev2.ClientIntents{})).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev2.ClientIntents, options ...client.ListOption) error {
			clientIntents.DeepCopyInto(intents)
			return nil
		})

	s.client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev2.PostgreSQLServerConfigList{}), gomock.Any()).DoAndReturn(
		func(ctx context.Context, pgServerConfList *otterizev2.PostgreSQLServerConfigList, options ...client.ListOption) error {
			pgServerConfList.Items = pgServerConfigs
			return nil
		})

	s.client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev2.MySQLServerConfigList{}), gomock.Any()).DoAndReturn(
		func(ctx context.Context, mySQLServerConfList *otterizev2.MySQLServerConfigList, options ...client.ListOption) error {
			mySQLServerConfList.Items = []otterizev2.MySQLServerConfig{}
			return nil
		})

	req := ctrl.Request{NamespacedName: s.namespacedName}

	return s.Reconciler.Reconcile(context.Background(), req)
}

func (s *DatabaseReconcilerTestSuite) TestHashedUsernameLength() {
	longWorkloadName := "my.super.long-workload-name"
	longNamespace := "my.super.long-namespace-name"
	clusterID := "abc-def-ghi-jkl-mno-189023123"
	hashedUsername := clusterutils.BuildHashedUsername(longWorkloadName, longNamespace, clusterID)
	s.Require().True(len(hashedUsername) <= 32)

	pgUsername := clusterutils.KubernetesToPostgresName(hashedUsername)
	s.Require().True(len(pgUsername) <= 32)
}

func TestDatabaseReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(DatabaseReconcilerTestSuite))
}
