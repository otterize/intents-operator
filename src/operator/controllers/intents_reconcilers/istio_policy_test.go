package intents_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"istio.io/client-go/pkg/apis/security/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"testing"
	"time"
)

type IstioPolicyReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	Reconciler      *IstioPolicyReconciler
	policyAdmin     *mocks.MockAdmin
	serviceResolver *mocks.MockServiceResolver
	scheme          *runtime.Scheme
}

func (s *IstioPolicyReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.policyAdmin = mocks.NewMockAdmin(s.Controller)
	s.serviceResolver = mocks.NewMockServiceResolver(s.Controller)
	restrictToNamespaces := make([]string, 0)
	s.scheme = runtime.NewScheme()
	s.scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "security.istio.io", Version: "v1", Kind: "authorizationpolicies"}, &v1beta1.AuthorizationPolicy{})

	s.Reconciler = NewIstioPolicyReconciler(
		s.Client,
		s.scheme,
		restrictToNamespaces,
		true,
		true,
	)

	s.Reconciler.Recorder = s.Recorder
	s.Reconciler.serviceIdResolver = s.serviceResolver
	s.Reconciler.policyManager = s.policyAdmin
}

func (s *IstioPolicyReconcilerTestSuite) TearDownTest() {
	s.Reconciler = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *IstioPolicyReconcilerTestSuite) TestCreatePolicy() {
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	serverNamespace := "far-far-away"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev1alpha2.IntentsSpec{
		Service: otterizev1alpha2.Service{Name: serviceName},
		Calls: []otterizev1alpha2.Intent{
			{
				Name: serverName,
			},
		},
	}

	intentsWithoutFinalizer := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientIntentsName,
			Namespace: testNamespace,
		},
		Spec: intentsSpec,
	}

	s.expectValidatingIstioIsInstalled()

	// Initial call to get the ClientIntents object when reconciler starts
	emptyIntents := &otterizev1alpha2.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			intentsWithoutFinalizer.DeepCopyInto(intents)
			return nil
		})

	// Check finalizer is added
	intentsObj := otterizev1alpha2.ClientIntents{}
	intentsWithoutFinalizer.DeepCopyInto(&intentsObj)
	controllerutil.AddFinalizer(&intentsObj, consts.IstioPolicyFinalizerName)
	s.Client.EXPECT().Update(gomock.Any(), gomock.Eq(&intentsObj)).Return(nil)

	clientServiceAccount := "test-server-sa"
	clientPod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-client-fdae32",
			Namespace: serverNamespace,
		},
		Spec: v1.PodSpec{
			ServiceAccountName: clientServiceAccount,
			Containers: []v1.Container{
				{
					Name: "real-application-who-does-something",
				},
				{
					Name: "istio-proxy",
				},
			},
		},
	}

	serverPod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-server-2b5e0d",
			Namespace: serverNamespace,
		},
		Spec: v1.PodSpec{
			ServiceAccountName: "test-server-sa",
			Containers: []v1.Container{
				{
					Name: "server-who-listens",
				},
				{
					Name: "istio-proxy",
				},
			},
		},
	}
	s.serviceResolver.EXPECT().ResolveClientIntentToPod(gomock.Any(), gomock.Eq(intentsObj)).Return(clientPod, nil)
	s.policyAdmin.EXPECT().UpdateIntentsStatus(gomock.Any(), gomock.Eq(&intentsObj), clientServiceAccount, false).Return(nil)
	s.serviceResolver.EXPECT().ResolveIntentServerToPod(gomock.Any(), gomock.Eq(intentsObj.Spec.Calls[0]), serverNamespace).Return(serverPod, nil)
	s.policyAdmin.EXPECT().UpdateServerSidecar(gomock.Any(), gomock.Eq(&intentsObj), "test-server-far-far-away-aa0d79", false).Return(nil)
	s.policyAdmin.EXPECT().Create(gomock.Any(), gomock.Eq(&intentsObj), clientServiceAccount).Return(nil)
	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *IstioPolicyReconcilerTestSuite) expectValidatingIstioIsInstalled() {
	s.Client.EXPECT().Scheme().Return(s.scheme)
	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "authorizationpolicies.security.istio.io"}, gomock.Any()).Return(nil)
}

func (s *IstioPolicyReconcilerTestSuite) TestGlobalEnforcementDisabled() {
	s.Reconciler.enforcementDefaultState = false
	s.assertPolicyCreateCalledEvenIfDisabledEnforcementConfigHappensInPolicyManager()
}

func (s *IstioPolicyReconcilerTestSuite) TestIstioPolicyEnforcementDisabled() {
	s.Reconciler.enableIstioPolicyCreation = false
	s.assertPolicyCreateCalledEvenIfDisabledEnforcementConfigHappensInPolicyManager()
}

func (s *IstioPolicyReconcilerTestSuite) assertPolicyCreateCalledEvenIfDisabledEnforcementConfigHappensInPolicyManager() {
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	serverNamespace := "far-far-away"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev1alpha2.IntentsSpec{
		Service: otterizev1alpha2.Service{Name: serviceName},
		Calls: []otterizev1alpha2.Intent{
			{
				Name: serverName,
			},
		},
	}

	clientIntentsObj := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientIntentsName,
			Namespace: testNamespace,
		},
		Spec: intentsSpec,
	}
	controllerutil.AddFinalizer(&clientIntentsObj, consts.IstioPolicyFinalizerName)

	s.expectValidatingIstioIsInstalled()

	// Initial call to get the ClientIntents object when reconciler starts
	emptyIntents := &otterizev1alpha2.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			clientIntentsObj.DeepCopyInto(intents)
			return nil
		})

	clientServiceAccount := "test-server-sa"
	clientPod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-client-fdae32",
			Namespace: serverNamespace,
		},
		Spec: v1.PodSpec{
			ServiceAccountName: clientServiceAccount,
			Containers: []v1.Container{
				{
					Name: "real-application-who-does-something",
				},
				{
					Name: "istio-proxy",
				},
			},
		},
	}

	serverPod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-server-2b5e0d",
			Namespace: serverNamespace,
		},
		Spec: v1.PodSpec{
			ServiceAccountName: "test-server-sa",
			Containers: []v1.Container{
				{
					Name: "server-who-listens",
				},
				{
					Name: "istio-proxy",
				},
			},
		},
	}

	s.serviceResolver.EXPECT().ResolveClientIntentToPod(gomock.Any(), gomock.Eq(clientIntentsObj)).Return(clientPod, nil)
	s.policyAdmin.EXPECT().UpdateIntentsStatus(gomock.Any(), gomock.Eq(&clientIntentsObj), clientServiceAccount, false).Return(nil)
	s.serviceResolver.EXPECT().ResolveIntentServerToPod(gomock.Any(), gomock.Eq(clientIntentsObj.Spec.Calls[0]), serverNamespace).Return(serverPod, nil)
	s.policyAdmin.EXPECT().UpdateServerSidecar(gomock.Any(), gomock.Eq(&clientIntentsObj), "test-server-far-far-away-aa0d79", false).Return(nil)
	s.policyAdmin.EXPECT().Create(gomock.Any(), gomock.Eq(&clientIntentsObj), clientServiceAccount).Return(nil)

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *IstioPolicyReconcilerTestSuite) TestIstioPolicyFinalizerRemoved() {
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	serverNamespace := "far-far-away"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev1alpha2.IntentsSpec{
		Service: otterizev1alpha2.Service{Name: serviceName},
		Calls: []otterizev1alpha2.Intent{
			{
				Name: serverName,
			},
		},
	}

	date := metav1.Date(1989, 2, 15, 20, 00, 0, 0, time.UTC)
	clientIntentsObj := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:              clientIntentsName,
			Namespace:         testNamespace,
			DeletionTimestamp: &date,
		},
		Spec: intentsSpec,
	}
	controllerutil.AddFinalizer(&clientIntentsObj, consts.IstioPolicyFinalizerName)

	s.expectValidatingIstioIsInstalled()

	// Initial call to get the ClientIntents object when reconciler starts
	emptyIntents := &otterizev1alpha2.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			clientIntentsObj.DeepCopyInto(intents)
			return nil
		})

	s.policyAdmin.EXPECT().DeleteAll(gomock.Any(), gomock.Eq(&clientIntentsObj)).Return(nil)

	intentsWithoutFinalizer := &otterizev1alpha2.ClientIntents{}
	clientIntentsObj.DeepCopyInto(intentsWithoutFinalizer)
	controllerutil.RemoveFinalizer(intentsWithoutFinalizer, consts.IstioPolicyFinalizerName)

	s.Client.EXPECT().Update(gomock.Any(), gomock.Eq(intentsWithoutFinalizer)).Return(nil)

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func TestIstioPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(IstioPolicyReconcilerTestSuite))
}
