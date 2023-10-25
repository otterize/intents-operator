package controllers

import (
	"context"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
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
		nil,
		nil,
		nil,
		nil,
		EnforcementConfig{},
		nil,
		"",
		"",
	)
}

func (s *IntentsControllerTestSuite) TearDownTest() {
	s.MocksSuiteBase.TearDownTest()
}

func (s *IntentsControllerTestSuite) TestMappingProtectedServicesToIntent() {
	protectedService := otterizev1alpha3.ProtectedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "protected-service",
			Namespace: "test-namespace",
		},
		Spec: otterizev1alpha3.ProtectedServiceSpec{
			Name: "checkoutservice",
		},
	}

	clientIntents := []otterizev1alpha3.ClientIntents{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "client-intents",
				Namespace: "test-namespace",
			},
			Spec: &otterizev1alpha3.IntentsSpec{
				Service: otterizev1alpha3.Service{
					Name: "checkoutservice",
				},
				Calls: []otterizev1alpha3.Intent{
					{
						Name: "payments-service",
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-client-intents",
				Namespace: "test-namespace",
			},
			Spec: &otterizev1alpha3.IntentsSpec{
				Service: otterizev1alpha3.Service{
					Name: "another-non-related-client",
				},
				Calls: []otterizev1alpha3.Intent{
					{
						Name: "totally-unrelated-server.another-namespace",
					},
				},
			},
		},
	}

	fullServerName := "checkoutservice.test-namespace"
	s.Client.EXPECT().List(
		gomock.Any(),
		&otterizev1alpha3.ClientIntentsList{},
		&client.MatchingFields{otterizev1alpha2.OtterizeTargetServerIndexField: fullServerName},
	).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha3.ClientIntentsList, opts ...client.ListOption) error {
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
	res := s.intentsReconciler.mapProtectedServiceToClientIntents(&protectedService)
	s.Require().Equal(expected, res)
}

func (s *IntentsControllerTestSuite) TestMappingProtectedServicesToIntentNoIntents() {
	protectedService := otterizev1alpha3.ProtectedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "protected-service",
			Namespace: "test-namespace",
		},
		Spec: otterizev1alpha3.ProtectedServiceSpec{
			Name: "checkoutservice",
		},
	}

	fullServerName := "checkoutservice.test-namespace"
	s.Client.EXPECT().List(
		gomock.Any(),
		&otterizev1alpha3.ClientIntentsList{},
		&client.MatchingFields{otterizev1alpha2.OtterizeTargetServerIndexField: fullServerName},
	).Return(nil)

	expected := make([]reconcile.Request, 0)
	res := s.intentsReconciler.mapProtectedServiceToClientIntents(&protectedService)
	s.Require().Equal(expected, res)
}

func TestIntentsControllerTestSuite(t *testing.T) {
	suite.Run(t, new(IntentsControllerTestSuite))
}
