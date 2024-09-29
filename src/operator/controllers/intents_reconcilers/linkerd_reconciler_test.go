package intents_reconcilers

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/shared/testbase"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type LinkerdReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	scheme          *runtime.Scheme
	admin           *LinkerdReconciler
	ldm             *mocks.MockLinkerdPolicyManager
	serviceResolver *mocks.MockServiceResolver
}

func (s *LinkerdReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.serviceResolver = mocks.NewMockServiceResolver(s.Controller)
	s.scheme = runtime.NewScheme()
	s.scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "policy.linkerd.io", Version: "v1beta1", Kind: "servers"}, &linkerdserver.Server{})
	s.admin = NewLinkerdReconciler(s.Client, s.scheme, []string{}, true, true)
	s.ldm = mocks.NewMockLinkerdPolicyManager(s.Controller)

	s.admin.Recorder = s.Recorder
	s.admin.serviceIdResolver = s.serviceResolver
	s.admin.linkerdManager = s.ldm
}

func (s *LinkerdReconcilerTestSuite) TearDownTest() {
	s.admin = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *LinkerdReconcilerTestSuite) TestCreatePolicy() {
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
	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Name: serverName,
			},
		},
	}

	intentsWithoutFinalizer := otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientIntentsName,
			Namespace: testNamespace,
		},
		Spec: intentsSpec,
	}

	s.expectValidatingLinkerdInstalled()

	emptyIntents := &otterizev1alpha3.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha3.ClientIntents, options ...client.ListOption) error {
			intentsWithoutFinalizer.DeepCopyInto(intents)
			return nil
		})

	intentsObj := otterizev1alpha3.ClientIntents{}
	intentsWithoutFinalizer.DeepCopyInto(&intentsObj)

	clientServiceAccount := "test-server-sa"
	clientPod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"linkerd.io/inject": "enabled",
			},
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
					Name: "linkerd-proxy",
				},
			},
		},
	}

	s.serviceResolver.EXPECT().ResolveClientIntentToPod(gomock.Any(), gomock.Eq(intentsObj)).Return(clientPod, nil)
	s.ldm.EXPECT().Create(gomock.Any(), gomock.Eq(&intentsObj), clientServiceAccount).Return(nil)
	res, err := s.admin.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *LinkerdReconcilerTestSuite) expectValidatingLinkerdInstalled() {
	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "servers.policy.linkerd.io"}, gomock.Any()).Return(nil)
}

func TestLinkerdReconcilerSuite(t *testing.T) {
	suite.Run(t, new(LinkerdReconcilerTestSuite))
}
