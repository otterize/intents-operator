package protected_services_reconcilers

import (
	"context"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	otterizecloudmocks "github.com/otterize/intents-operator/src/shared/otterizecloud/mocks"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

const (
	anotherProtectedServicesResourceName = "another-protected-services"
)

type CloudReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	reconciler  *CloudReconciler
	cloudClient *otterizecloudmocks.MockCloudClient
}

func (s *CloudReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()

	s.cloudClient = otterizecloudmocks.NewMockCloudClient(s.Controller)
	scheme := runtime.NewScheme()
	s.reconciler = NewCloudReconciler(s.Client, scheme, s.cloudClient)
	viper.Set(operatorconfig.EnableProtectedServicesKey, true)
}

func (s *CloudReconcilerTestSuite) TearDownTest() {
	viper.Reset()
	s.reconciler = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *CloudReconcilerTestSuite) TestUploadProtectedServices() {
	var protectedServicesResources otterizev1alpha2.ProtectedServicesList
	protectedServicesResources.Items = []otterizev1alpha2.ProtectedServices{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      protectedServicesResourceName,
				Namespace: testNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServicesSpec{
				ProtectedServices: []otterizev1alpha2.ProtectedService{
					{
						Name: protectedService,
					},
					{
						Name: anotherProtectedService,
					},
				},
			},
		},
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServicesList{}), client.InNamespace(testNamespace)).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ProtectedServicesList, opts ...client.ListOption) error {
			protectedServicesResources.DeepCopyInto(list)
			return nil
		})

	services := []graphqlclient.ProtectedServiceInput{
		{
			Name: protectedService,
		},
		{
			Name: anotherProtectedService,
		},
	}
	s.cloudClient.EXPECT().ReportProtectedServices(gomock.Any(), gomock.Eq(testNamespace), MatchProtectedServicesMatcher(services)).Return(nil)

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      protectedServicesResourceName,
		},
	}

	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.NoError(err)
	s.Equal(ctrl.Result{}, res)
}

func (s *CloudReconcilerTestSuite) TestUploadMultipleResources() {
	var protectedServicesResources otterizev1alpha2.ProtectedServicesList
	protectedServicesResources.Items = []otterizev1alpha2.ProtectedServices{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      protectedServicesResourceName,
				Namespace: testNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServicesSpec{
				ProtectedServices: []otterizev1alpha2.ProtectedService{
					{
						Name: protectedService,
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "redundant-protected-services-resource",
				Namespace: testNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServicesSpec{
				ProtectedServices: []otterizev1alpha2.ProtectedService{
					{
						Name: anotherProtectedService,
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      protectedServicesResourceName,
				Namespace: testNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServicesSpec{
				ProtectedServices: []otterizev1alpha2.ProtectedService{
					{
						Name: protectedService,
					},
				},
			},
		},
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServicesList{}), client.InNamespace(testNamespace)).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ProtectedServicesList, opts ...client.ListOption) error {
			protectedServicesResources.DeepCopyInto(list)
			return nil
		})

	services := []graphqlclient.ProtectedServiceInput{
		{
			Name: protectedService,
		},
		{
			Name: anotherProtectedService,
		},
	}
	s.cloudClient.EXPECT().ReportProtectedServices(gomock.Any(), gomock.Eq(testNamespace), MatchProtectedServicesMatcher(services)).Return(nil)

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      protectedServicesResourceName,
		},
	}

	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.NoError(err)
	s.Equal(ctrl.Result{}, res)
}

func TestCloudReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(CloudReconcilerTestSuite))
}
