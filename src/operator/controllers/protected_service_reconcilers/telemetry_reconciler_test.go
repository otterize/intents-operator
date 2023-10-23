package protected_service_reconcilers

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
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
	protectedServiceResourceName = "test-resource-name"
	anotherNamespace             = "another-test-namespace"
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

	protectedService := otterizev1alpha3.ProtectedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      protectedServiceResourceName,
			Namespace: testNamespace,
		},
		Spec: otterizev1alpha3.ProtectedServiceSpec{
			Name: server,
		},
	}

	s.applyProtectedService(protectedService)
	s.Require().Equal(1, s.Reconciler.protectedServicesCounter.Len())

	s.applyProtectedService(protectedService)
	s.Require().Equal(1, s.Reconciler.protectedServicesCounter.Len())

	anotherProtectedService := otterizev1alpha3.ProtectedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      protectedServiceResourceName,
			Namespace: testNamespace,
		},
		Spec: otterizev1alpha3.ProtectedServiceSpec{
			Name: anotherServer,
		},
	}
	s.applyProtectedService(anotherProtectedService)
	s.Require().Equal(2, s.Reconciler.protectedServicesCounter.Len())

	protectedService = otterizev1alpha3.ProtectedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      protectedServiceResourceName,
			Namespace: testNamespace,
		},
		Spec: otterizev1alpha3.ProtectedServiceSpec{
			Name: server,
		},
	}
	s.applyProtectedService(protectedService)
	s.Require().Equal(2, s.Reconciler.protectedServicesCounter.Len())

	protectedServiceInAnotherNamespace := otterizev1alpha3.ProtectedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      protectedServiceResourceName,
			Namespace: anotherNamespace,
		},
		Spec: otterizev1alpha3.ProtectedServiceSpec{
			Name: server,
		},
	}
	s.applyProtectedService(protectedServiceInAnotherNamespace)
	s.Require().Equal(3, s.Reconciler.protectedServicesCounter.Len())

	anotherProtectedServiceInAnotherNamespace := otterizev1alpha3.ProtectedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      protectedServiceResourceName,
			Namespace: anotherNamespace,
		},
		Spec: otterizev1alpha3.ProtectedServiceSpec{
			Name: anotherServer,
		},
	}

	s.applyProtectedService(anotherProtectedServiceInAnotherNamespace)
	s.Require().Equal(4, s.Reconciler.protectedServicesCounter.Len())

	s.removeProtectedService(protectedService)
	s.Require().Equal(3, s.Reconciler.protectedServicesCounter.Len())

	s.removeProtectedService(anotherProtectedService)
	s.Require().Equal(2, s.Reconciler.protectedServicesCounter.Len())
}

func (s *CountReconcilerTestSuite) applyProtectedService(resource otterizev1alpha3.ProtectedService) {
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      protectedServiceResourceName,
		},
	}

	emptyProtectedServices := &otterizev1alpha3.ProtectedService{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyProtectedServices)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, protectedService *otterizev1alpha3.ProtectedService, options ...client.ListOption) error {
			resource.DeepCopyInto(protectedService)
			return nil
		})

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Equal(ctrl.Result{}, res)
}

func (s *CountReconcilerTestSuite) removeProtectedService(resource otterizev1alpha3.ProtectedService) {
	resource.DeletionTimestamp = &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)}
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      protectedServiceResourceName,
		},
	}

	emptyProtectedServices := &otterizev1alpha3.ProtectedService{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyProtectedServices)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, protectedService *otterizev1alpha3.ProtectedService, options ...client.ListOption) error {
			resource.DeepCopyInto(protectedService)
			return nil
		})

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Equal(ctrl.Result{}, res)
}

func TestCountReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(CountReconcilerTestSuite))
}
