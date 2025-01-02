package kafka_server_config_reconcilers

import (
	"context"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"
)

const (
	kscResourceName  = "test-resource-name"
	anotherNamespace = "another-test-namespace"
)

type CountReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	Reconciler *TelemetryReconciler
}

func (s *CountReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()

	s.Reconciler = NewTelemetryReconciler(s.Client)
	s.Reconciler.Recorder = s.Recorder
}

func (s *CountReconcilerTestSuite) TearDownTest() {
	s.Reconciler = nil
}

func (s *CountReconcilerTestSuite) TestAppliedProtectedServices() {
	server := "test-server"
	anotherServer := "another-test-server"

	serverConfig := otterizev2alpha1.KafkaServerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kscResourceName,
			Namespace: testNamespace,
		},
		Spec: otterizev2alpha1.KafkaServerConfigSpec{
			Workload: otterizev2alpha1.Workload{
				Name: server,
			},
		},
	}

	s.applyConfig(serverConfig)
	s.Require().Equal(1, s.Reconciler.kafkaServerCounter.Len())

	s.applyConfig(serverConfig)
	s.Require().Equal(1, s.Reconciler.kafkaServerCounter.Len())

	anotherConfig := otterizev2alpha1.KafkaServerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kscResourceName,
			Namespace: testNamespace,
		},
		Spec: otterizev2alpha1.KafkaServerConfigSpec{
			Workload: otterizev2alpha1.Workload{
				Name: anotherServer,
			},
		},
	}
	s.applyConfig(anotherConfig)
	s.Require().Equal(2, s.Reconciler.kafkaServerCounter.Len())

	serverConfig = otterizev2alpha1.KafkaServerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kscResourceName,
			Namespace: testNamespace,
		},
		Spec: otterizev2alpha1.KafkaServerConfigSpec{
			Workload: otterizev2alpha1.Workload{
				Name: server,
			},
		},
	}
	s.applyConfig(serverConfig)
	s.Require().Equal(2, s.Reconciler.kafkaServerCounter.Len())

	configInAnotherNamespace := otterizev2alpha1.KafkaServerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kscResourceName,
			Namespace: anotherNamespace,
		},
		Spec: otterizev2alpha1.KafkaServerConfigSpec{
			Workload: otterizev2alpha1.Workload{
				Name: server,
			},
		},
	}
	s.applyConfig(configInAnotherNamespace)
	s.Require().Equal(3, s.Reconciler.kafkaServerCounter.Len())

	anotherConfigInAnotherNamespace := otterizev2alpha1.KafkaServerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kscResourceName,
			Namespace: anotherNamespace,
		},
		Spec: otterizev2alpha1.KafkaServerConfigSpec{
			Workload: otterizev2alpha1.Workload{
				Name: anotherServer,
			},
		},
	}

	s.applyConfig(anotherConfigInAnotherNamespace)
	s.Require().Equal(4, s.Reconciler.kafkaServerCounter.Len())

	s.removeConfig(serverConfig)
	s.Require().Equal(3, s.Reconciler.kafkaServerCounter.Len())

	s.removeConfig(anotherConfig)
	s.Require().Equal(2, s.Reconciler.kafkaServerCounter.Len())
}

func (s *CountReconcilerTestSuite) applyConfig(resource otterizev2alpha1.KafkaServerConfig) {
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      kscResourceName,
		},
	}

	emptyConfig := &otterizev2alpha1.KafkaServerConfig{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyConfig)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, serverConfig *otterizev2alpha1.KafkaServerConfig, options ...client.ListOption) error {
			resource.DeepCopyInto(serverConfig)
			return nil
		})

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Equal(ctrl.Result{}, res)
}

func (s *CountReconcilerTestSuite) removeConfig(resource otterizev2alpha1.KafkaServerConfig) {
	resource.DeletionTimestamp = &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)}
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      kscResourceName,
		},
	}

	emptyConfig := &otterizev2alpha1.KafkaServerConfig{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyConfig)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, serverConfig *otterizev2alpha1.KafkaServerConfig, options ...client.ListOption) error {
			resource.DeepCopyInto(serverConfig)
			return nil
		})

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Equal(ctrl.Result{}, res)
}

func TestCountReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(CountReconcilerTestSuite))
}
