package intents_reconcilers

import (
	"context"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/shared/gcpagent"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"

	"github.com/otterize/intents-operator/src/shared/testbase"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

type IAMIntentsReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	Reconciler      *IAMIntentsReconciler
	serviceResolver *mocks.MockServiceResolver
	gcpAgent        *mocks.MockIAMPolicyAgent
	recorder        *record.FakeRecorder
	scheme          *runtime.Scheme
}

func (s *IAMIntentsReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.scheme = runtime.NewScheme()
	s.serviceResolver = mocks.NewMockServiceResolver(s.Controller)
	s.gcpAgent = mocks.NewMockIAMPolicyAgent(s.Controller)

	var iamAgents []IAMPolicyAgent
	iamAgents = append(iamAgents, s.gcpAgent)

	s.Reconciler = NewIAMIntentsReconciler(
		s.Client,
		s.scheme,
		s.serviceResolver,
		iamAgents,
	)

	s.recorder = record.NewFakeRecorder(100)
	s.Reconciler.InjectRecorder(s.recorder)
}

func (s *IAMIntentsReconcilerTestSuite) TestCreateIAMIntentNoPodLabelHasNoEffect() {
	clientIntentsName := "client-intents"
	serviceName := "test-client"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{NamespacedName: namespacedName}

	gcpIntents := otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientIntentsName,
			Namespace: testNamespace,
		},
		Spec: &otterizev1alpha3.IntentsSpec{
			Service: otterizev1alpha3.Service{Name: serviceName},
			Calls: []otterizev1alpha3.Intent{
				{
					Name: "projects/_/buckets/bucket-name",
					Type: v1alpha3.IntentTypeGCP,
				},
			},
		},
	}

	clientServiceAccount := "test-server-sa"
	clientPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: testNamespace,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: clientServiceAccount,
			Containers: []corev1.Container{
				{
					Name: "real-application-who-does-something",
				},
			},
		},
	}

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&gcpIntents)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *otterizev1alpha3.ClientIntents, arg3 ...client.GetOption) error {
			gcpIntents.DeepCopyInto(arg2)
			return nil
		},
	)

	s.serviceResolver.EXPECT().ResolveClientIntentToPod(gomock.Any(), gomock.Eq(gcpIntents)).Return(clientPod, nil)

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *IAMIntentsReconcilerTestSuite) TestCreateIAMIntentCallingTheGCPAgent() {
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	clientServiceAccount := "test-server-sa"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{NamespacedName: namespacedName}

	filteredIntents := []otterizev1alpha3.Intent{
		{
			Name: "projects/_/buckets/bucket-name",
			Type: v1alpha3.IntentTypeGCP,
		},
	}

	allIntents := []otterizev1alpha3.Intent{
		{
			Name: "aws::s3::bucket::bucket-name",
			Type: v1alpha3.IntentTypeAWS,
		},
	}
	allIntents = append(allIntents, filteredIntents...)

	gcpIntents := otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientIntentsName,
			Namespace: testNamespace,
		},
		Spec: &otterizev1alpha3.IntentsSpec{
			Service: otterizev1alpha3.Service{Name: serviceName},
			Calls:   allIntents,
		},
	}

	clientPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: testNamespace,
			Labels:    map[string]string{gcpagent.GCPApplyOnPodLabel: "true"},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: clientServiceAccount,
			Containers: []corev1.Container{
				{
					Name: "real-application-who-does-something",
				},
			},
		},
	}

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&gcpIntents)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *otterizev1alpha3.ClientIntents, arg3 ...client.GetOption) error {
			gcpIntents.DeepCopyInto(arg2)
			return nil
		},
	)

	s.serviceResolver.EXPECT().ResolveClientIntentToPod(gomock.Any(), gomock.Eq(gcpIntents)).Return(clientPod, nil)
	s.gcpAgent.EXPECT().AppliesOnPod(gomock.AssignableToTypeOf(&clientPod)).Return(true)
	s.gcpAgent.EXPECT().IntentType().Return(otterizev1alpha3.IntentTypeGCP)
	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&otterizev1alpha3.ClientIntentsList{}),
		&client.ListOptions{Namespace: testNamespace},
	).Return(nil)
	s.gcpAgent.EXPECT().AddRolePolicyFromIntents(gomock.Any(), testNamespace, clientServiceAccount, serviceName, filteredIntents).Return(nil)

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)

	select {
	case event := <-s.recorder.Events:
		s.Require().Contains(event, consts.ReasonReconciledIAMPolicies)
	default:
		s.Fail("event not raised")
	}
}

func (s *IAMIntentsReconcilerTestSuite) TestCreateIAMIntentPartialDeleteCallingTheGCPAgent() {
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	clientServiceAccount := "test-server-sa"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{NamespacedName: namespacedName}

	awsIntents := []otterizev1alpha3.Intent{
		{
			Name: "aws::s3::bucket::bucket-name",
			Type: v1alpha3.IntentTypeAWS,
		},
	}

	clientIntents := otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientIntentsName,
			Namespace: testNamespace,
		},
		Spec: &otterizev1alpha3.IntentsSpec{
			Service: otterizev1alpha3.Service{Name: serviceName},
			Calls:   awsIntents,
		},
	}

	clientPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: testNamespace,
			Labels:    map[string]string{gcpagent.GCPApplyOnPodLabel: "true"},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: clientServiceAccount,
			Containers: []corev1.Container{
				{
					Name: "real-application-who-does-something",
				},
			},
		},
	}

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&clientIntents)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *otterizev1alpha3.ClientIntents, arg3 ...client.GetOption) error {
			clientIntents.DeepCopyInto(arg2)
			return nil
		},
	)

	s.serviceResolver.EXPECT().ResolveClientIntentToPod(gomock.Any(), gomock.Eq(clientIntents)).Return(clientPod, nil)
	s.gcpAgent.EXPECT().AppliesOnPod(gomock.AssignableToTypeOf(&clientPod)).Return(true)
	s.gcpAgent.EXPECT().IntentType().Return(otterizev1alpha3.IntentTypeGCP)
	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&otterizev1alpha3.ClientIntentsList{}),
		&client.ListOptions{Namespace: testNamespace},
	).Return(nil)
	s.gcpAgent.EXPECT().AddRolePolicyFromIntents(gomock.Any(), testNamespace, clientServiceAccount, serviceName, []otterizev1alpha3.Intent{}).Return(nil)

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)

	select {
	case event := <-s.recorder.Events:
		s.Require().Contains(event, consts.ReasonReconciledIAMPolicies)
	default:
		s.Fail("event not raised")
	}
}

func TestIAMIntentsReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(IAMIntentsReconcilerTestSuite))
}
