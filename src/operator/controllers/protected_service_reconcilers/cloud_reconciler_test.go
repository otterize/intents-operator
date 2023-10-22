package protected_service_reconcilers

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/protected_service_reconcilers/consts"
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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"testing"
	"time"
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
}

func (s *CloudReconcilerTestSuite) TearDownTest() {
	viper.Reset()
	s.reconciler = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *CloudReconcilerTestSuite) TestUploadSingleProtectedService() {
	var protectedServicesResources otterizev1alpha3.ProtectedServiceList
	protectedServicesResources.Items = []otterizev1alpha3.ProtectedService{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      protectedServicesResourceName,
				Namespace: testNamespace,
				Finalizers: []string{
					consts.CloudReconcilerFinalizerName,
				},
			},
			Spec: otterizev1alpha3.ProtectedServiceSpec{

				Name: protectedService,
			},
		},
	}

	nameSpacedName := types.NamespacedName{
		Name:      protectedServicesResourceName,
		Namespace: testNamespace,
	}

	s.Client.EXPECT().Get(gomock.Any(), gomock.Eq(nameSpacedName), gomock.Any()).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, protectedService *otterizev1alpha3.ProtectedService, opts ...client.GetOption) error {
			protectedServicesResources.Items[0].DeepCopyInto(protectedService)
			return nil
		})

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha3.ProtectedServiceList{}), client.InNamespace(testNamespace)).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha3.ProtectedServiceList, opts ...client.ListOption) error {
			protectedServicesResources.DeepCopyInto(list)
			return nil
		})

	services := []graphqlclient.ProtectedServiceInput{
		{
			Name: protectedService,
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
	s.Empty(res)
}

func (s *CloudReconcilerTestSuite) TestUploadMultipleProtectedServices() {
	var protectedServicesResources otterizev1alpha3.ProtectedServiceList
	protectedServicesResources.Items = []otterizev1alpha3.ProtectedService{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      protectedServicesResourceName,
				Namespace: testNamespace,
				Finalizers: []string{
					consts.CloudReconcilerFinalizerName,
				},
			},
			Spec: otterizev1alpha3.ProtectedServiceSpec{

				Name: protectedService,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "redundant-protected-services-resource",
				Namespace: testNamespace,
			},
			Spec: otterizev1alpha3.ProtectedServiceSpec{

				Name: anotherProtectedService,
			},
		},
	}

	nameSpacedName := types.NamespacedName{
		Name:      protectedServicesResourceName,
		Namespace: testNamespace,
	}

	s.Client.EXPECT().Get(gomock.Any(), gomock.Eq(nameSpacedName), gomock.Any()).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, protectedService *otterizev1alpha3.ProtectedService, opts ...client.GetOption) error {
			protectedServicesResources.Items[0].DeepCopyInto(protectedService)
			return nil
		})
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha3.ProtectedServiceList{}), client.InNamespace(testNamespace)).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha3.ProtectedServiceList, opts ...client.ListOption) error {
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
	s.Empty(res)
}

func (s *CloudReconcilerTestSuite) TestFinalizerAdd() {
	resourceWithoutFinalizer := otterizev1alpha3.ProtectedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      protectedServicesResourceName,
			Namespace: testNamespace,
		},
		Spec: otterizev1alpha3.ProtectedServiceSpec{

			Name: protectedService,
		},
	}

	resourceWithFinalizer := resourceWithoutFinalizer.DeepCopy()
	resourceWithFinalizer.ObjectMeta.Finalizers = []string{
		consts.CloudReconcilerFinalizerName,
	}

	nameSpacedName := types.NamespacedName{
		Name:      protectedServicesResourceName,
		Namespace: testNamespace,
	}

	s.Client.EXPECT().Get(gomock.Any(), gomock.Eq(nameSpacedName), gomock.Any()).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, protectedService *otterizev1alpha3.ProtectedService, opts ...client.GetOption) error {
			resourceWithoutFinalizer.DeepCopyInto(protectedService)
			return nil
		})

	s.Client.EXPECT().Update(gomock.Any(), gomock.Eq(resourceWithFinalizer)).Return(nil)

	// Ignore the rest of the logic, it's tested in other tests
	s.Client.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	s.cloudClient.EXPECT().ReportProtectedServices(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      protectedServicesResourceName,
		},
	}

	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.NoError(err)
	s.Empty(res)
}

func (s *CloudReconcilerTestSuite) TestFinalizerRemoved() {
	resourceWithFinalizer := otterizev1alpha3.ProtectedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:              protectedServicesResourceName,
			Namespace:         testNamespace,
			DeletionTimestamp: &metav1.Time{Time: time.Date(2023, 9, 13, 18, 15, 0, 0, time.UTC)},
			Finalizers: []string{
				consts.CloudReconcilerFinalizerName,
			},
		},
		Spec: otterizev1alpha3.ProtectedServiceSpec{

			Name: protectedService,
		},
	}

	resourceWithoutFinalizer := resourceWithFinalizer.DeepCopy()
	controllerutil.RemoveFinalizer(resourceWithoutFinalizer, consts.CloudReconcilerFinalizerName)

	nameSpacedName := types.NamespacedName{
		Name:      protectedServicesResourceName,
		Namespace: testNamespace,
	}

	s.Client.EXPECT().Get(gomock.Any(), gomock.Eq(nameSpacedName), gomock.Any()).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, protectedService *otterizev1alpha3.ProtectedService, opts ...client.GetOption) error {
			resourceWithFinalizer.DeepCopyInto(protectedService)
			return nil
		})

	s.Client.EXPECT().Update(gomock.Any(), gomock.Eq(resourceWithoutFinalizer)).Return(nil)

	// Ignore the rest of the logic, it's tested in other tests
	s.Client.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	s.cloudClient.EXPECT().ReportProtectedServices(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      protectedServicesResourceName,
		},
	}

	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.NoError(err)
	s.Empty(res)
}

func TestCloudReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(CloudReconcilerTestSuite))
}
