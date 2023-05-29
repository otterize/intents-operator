package intents_reconcilers

import (
	"context"
	"fmt"
	"github.com/golang/mock/gomock"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/stretchr/testify/suite"
	"istio.io/client-go/pkg/apis/security/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"testing"
	"time"
)

type IstioPolicyReconcilerTestSuite struct {
	suite.Suite
	Reconciler      *IstioPolicyReconciler
	client          *mocks.MockClient
	recorder        *record.FakeRecorder
	policyCreator   *mocks.MockCreatorInterface
	serviceResolver *mocks.MockServiceResolver
	scheme          *runtime.Scheme
}

func (s *IstioPolicyReconcilerTestSuite) SetupTest() {
	controller := gomock.NewController(s.T())
	s.client = mocks.NewMockClient(controller)
	s.policyCreator = mocks.NewMockCreatorInterface(controller)
	s.serviceResolver = mocks.NewMockServiceResolver(controller)
	restrictToNamespaces := make([]string, 0)
	s.scheme = runtime.NewScheme()
	s.scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "security.istio.io", Version: "v1", Kind: "authorizationpolicies"}, &v1beta1.AuthorizationPolicy{})

	s.Reconciler = NewIstioPolicyReconciler(
		s.client,
		s.scheme,
		restrictToNamespaces,
		true,
		true,
	)

	s.recorder = record.NewFakeRecorder(100)
	s.Reconciler.Recorder = s.recorder
	s.Reconciler.serviceIdResolver = s.serviceResolver
	s.Reconciler.policyCreator = s.policyCreator
}

func (s *IstioPolicyReconcilerTestSuite) TearDownTest() {
	s.expectNoEvent()
	s.Reconciler = nil
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
	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			intentsWithoutFinalizer.DeepCopyInto(intents)
			return nil
		})

	// Check finalizer is added
	intentsObj := otterizev1alpha2.ClientIntents{}
	intentsWithoutFinalizer.DeepCopyInto(&intentsObj)
	controllerutil.AddFinalizer(&intentsObj, IstioPolicyFinalizerName)
	s.client.EXPECT().Update(gomock.Any(), gomock.Eq(&intentsObj)).Return(nil)

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
	s.policyCreator.EXPECT().UpdateIntentsStatus(gomock.Any(), gomock.Eq(&intentsObj), clientServiceAccount, false).Return(nil)
	s.serviceResolver.EXPECT().ResolveIntentServerToPod(gomock.Any(), gomock.Eq(intentsObj.Spec.Calls[0]), serverNamespace).Return(serverPod, nil)
	s.policyCreator.EXPECT().UpdateServerSidecar(gomock.Any(), gomock.Eq(&intentsObj), "test-server-far-far-away-aa0d79", false).Return(nil)
	s.policyCreator.EXPECT().Create(gomock.Any(), gomock.Eq(&intentsObj), clientServiceAccount).Return(nil)
	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Equal(ctrl.Result{}, res)
}

func (s *IstioPolicyReconcilerTestSuite) expectValidatingIstioIsInstalled() {
	s.client.EXPECT().Scheme().Return(s.scheme)
	s.client.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "authorizationpolicies.security.istio.io"}, gomock.Any()).Return(nil)
}

func (s *IstioPolicyReconcilerTestSuite) TestGlobalEnforcementDisabled() {
	s.Reconciler.enforcementEnabledGlobally = false
	s.assertPolicyIgnored()
	s.expectEvent(ReasonEnforcementGloballyDisabled)
}

func (s *IstioPolicyReconcilerTestSuite) TestIstioPolicyEnforcementDisabled() {
	s.Reconciler.enableIstioPolicyCreation = false
	s.assertPolicyIgnored()
	s.expectEvent(ReasonIstioPolicyCreationDisabled)
}

func (s *IstioPolicyReconcilerTestSuite) assertPolicyIgnored() {
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
	controllerutil.AddFinalizer(&clientIntentsObj, IstioPolicyFinalizerName)

	s.expectValidatingIstioIsInstalled()

	// Initial call to get the ClientIntents object when reconciler starts
	emptyIntents := &otterizev1alpha2.ClientIntents{}
	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			clientIntentsObj.DeepCopyInto(intents)
			return nil
		})

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Equal(ctrl.Result{}, res)
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
	controllerutil.AddFinalizer(&clientIntentsObj, IstioPolicyFinalizerName)

	s.expectValidatingIstioIsInstalled()

	// Initial call to get the ClientIntents object when reconciler starts
	emptyIntents := &otterizev1alpha2.ClientIntents{}
	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			clientIntentsObj.DeepCopyInto(intents)
			return nil
		})

	s.policyCreator.EXPECT().DeleteAll(gomock.Any(), gomock.Eq(&clientIntentsObj)).Return(nil)

	intentsWithoutFinalizer := &otterizev1alpha2.ClientIntents{}
	clientIntentsObj.DeepCopyInto(intentsWithoutFinalizer)
	controllerutil.RemoveFinalizer(intentsWithoutFinalizer, IstioPolicyFinalizerName)

	s.client.EXPECT().Update(gomock.Any(), gomock.Eq(intentsWithoutFinalizer)).Return(nil)

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Equal(ctrl.Result{}, res)
}

func (s *IstioPolicyReconcilerTestSuite) expectEvent(expectedEvent string) {
	select {
	case event := <-s.recorder.Events:
		s.Require().Contains(event, expectedEvent)
	default:
		s.Fail("Expected event not found")
	}
}

func (s *IstioPolicyReconcilerTestSuite) expectNoEvent() {
	select {
	case event := <-s.recorder.Events:
		s.Fail("Unexpected event found", event)
	default:
		// Amazing, no events left behind!
	}
}

func TestIstioPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(IstioPolicyReconcilerTestSuite))
}
