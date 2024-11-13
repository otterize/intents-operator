package controllers

import (
	"context"
	otterizev2 "github.com/otterize/intents-operator/src/operator/api/v2"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/enforcement"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
)

type IntentsControllerTestSuite struct {
	testbase.MocksSuiteBase
	intentsReconciler *IntentsReconciler
}

func (s *IntentsControllerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()

	s.intentsReconciler = NewIntentsReconciler(
		s.Client,
		scheme.Scheme,
		nil,
		nil,
		enforcement.Config{},
		"",
		"",
		nil,
		nil,
	)
}

func (s *IntentsControllerTestSuite) TearDownTest() {
	s.MocksSuiteBase.TearDownTest()
}

func (s *IntentsControllerTestSuite) TestMappingProtectedServicesToIntent() {
	protectedService := otterizev2.ProtectedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "protected-service",
			Namespace: "test-namespace",
		},
		Spec: otterizev2.ProtectedServiceSpec{
			Name: "checkoutservice",
		},
	}

	clientIntents := []otterizev2.ClientIntents{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "client-intents",
				Namespace: "test-namespace",
			},
			Spec: &otterizev2.IntentsSpec{
				Workload: otterizev2.Workload{
					Name: "checkoutservice",
				},
				Targets: []otterizev2.Target{
					{
						Kubernetes: &otterizev2.KubernetesTarget{Name: "payments-service"},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-client-intents",
				Namespace: "test-namespace",
			},
			Spec: &otterizev2.IntentsSpec{
				Workload: otterizev2.Workload{
					Name: "another-non-related-client",
				},
				Targets: []otterizev2.Target{
					{
						Kubernetes: &otterizev2.KubernetesTarget{Name: "totally-unrelated-server.another-namespace"},
					},
				},
			},
		},
	}

	fullServerName := "checkoutservice.test-namespace"
	s.Client.EXPECT().List(
		gomock.Any(),
		&otterizev2.ClientIntentsList{},
		&client.MatchingFields{otterizev2.OtterizeTargetServerIndexField: fullServerName},
	).DoAndReturn(
		func(ctx context.Context, list *otterizev2.ClientIntentsList, opts ...client.ListOption) error {
			list.Items = clientIntents
			return nil
		})

	expected := []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Namespace: "test-namespace",
				Name:      "client-intents",
			},
		},
		{
			NamespacedName: types.NamespacedName{
				Namespace: "test-namespace",
				Name:      "other-client-intents",
			},
		},
	}
	res := s.intentsReconciler.mapProtectedServiceToClientIntents(context.Background(), &protectedService)
	s.Require().Equal(expected, res)
}

func (s *IntentsControllerTestSuite) TestMappingProtectedServicesToIntentNoIntents() {
	protectedService := otterizev2.ProtectedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "protected-service",
			Namespace: "test-namespace",
		},
		Spec: otterizev2.ProtectedServiceSpec{
			Name: "checkoutservice",
		},
	}

	fullServerName := "checkoutservice.test-namespace"
	s.Client.EXPECT().List(
		gomock.Any(),
		&otterizev2.ClientIntentsList{},
		&client.MatchingFields{otterizev2.OtterizeTargetServerIndexField: fullServerName},
	).Return(nil)

	expected := make([]reconcile.Request, 0)
	res := s.intentsReconciler.mapProtectedServiceToClientIntents(context.Background(), &protectedService)
	s.Require().Equal(expected, res)
}

func TestIntentsControllerTestSuite(t *testing.T) {
	suite.Run(t, new(IntentsControllerTestSuite))
}
