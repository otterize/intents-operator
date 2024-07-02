package iam

import (
	"context"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	mock_iampolicyagents "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/iam/iampolicyagents/mocks"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
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

const (
	testNamespace = "test-namespace"

	testIntentType = otterizev2alpha1.IntentTypeGCP
)

type IAMIntentsReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	Reconciler      *IAMIntentsReconciler
	serviceResolver *mocks.MockServiceResolver
	iamAgent        *mock_iampolicyagents.MockIAMPolicyAgent
	recorder        *record.FakeRecorder
	scheme          *runtime.Scheme
}

func (s *IAMIntentsReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.scheme = runtime.NewScheme()
	s.serviceResolver = mocks.NewMockServiceResolver(s.Controller)
	s.iamAgent = mock_iampolicyagents.NewMockIAMPolicyAgent(s.Controller)

	s.Reconciler = NewIAMIntentsReconciler(
		s.Client,
		s.scheme,
		s.serviceResolver,
		s.iamAgent,
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

	iamIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientIntentsName,
			Namespace: testNamespace,
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{Name: serviceName},
			Targets: []otterizev2alpha1.Target{
				{
					GCP: &otterizev2alpha1.GCPTarget{
						Resource: "projects/_/buckets/bucket-name",
					},
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

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&iamIntents)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *otterizev2alpha1.ClientIntents, arg3 ...client.GetOption) error {
			iamIntents.DeepCopyInto(arg2)
			return nil
		},
	)

	s.serviceResolver.EXPECT().ResolveClientIntentToPod(gomock.Any(), gomock.Eq(iamIntents)).Return(clientPod, nil)

	s.iamAgent.EXPECT().AppliesOnPod(gomock.AssignableToTypeOf(&clientPod)).Return(false)

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *IAMIntentsReconcilerTestSuite) TestCreateIAMIntentCallingTheiamAgent() {
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	clientServiceAccount := "test-server-sa"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{NamespacedName: namespacedName}

	filteredIntents := []otterizev2alpha1.Target{
		{
			GCP: &otterizev2alpha1.GCPTarget{
				Resource: "projects/_/buckets/bucket-name",
			},
		},
	}

	allIntents := []otterizev2alpha1.Target{
		{
			AWS: &otterizev2alpha1.AWSTarget{
				ARN:     "aws::s3::bucket::bucket-name",
				Actions: []string{"asdf"},
			},
		},
	}
	allIntents = append(allIntents, filteredIntents...)

	iamIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientIntentsName,
			Namespace: testNamespace,
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{Name: serviceName},
			Targets:  allIntents,
		},
	}

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

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&iamIntents)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *otterizev2alpha1.ClientIntents, arg3 ...client.GetOption) error {
			iamIntents.DeepCopyInto(arg2)
			return nil
		},
	)

	s.serviceResolver.EXPECT().ResolveClientIntentToPod(gomock.Any(), gomock.Eq(iamIntents)).Return(clientPod, nil)
	s.iamAgent.EXPECT().AppliesOnPod(gomock.AssignableToTypeOf(&clientPod)).Return(true)
	s.iamAgent.EXPECT().IntentType().Return(testIntentType)
	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&otterizev2alpha1.ClientIntentsList{}),
		&client.ListOptions{Namespace: testNamespace},
	).Return(nil)
	s.iamAgent.EXPECT().AddRolePolicyFromIntents(gomock.Any(), testNamespace, clientServiceAccount, serviceName, filteredIntents, clientPod).Return(nil)

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

func (s *IAMIntentsReconcilerTestSuite) TestCreateIAMIntentPartialDeleteCallingTheiamAgent() {
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	clientServiceAccount := "test-server-sa"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{NamespacedName: namespacedName}

	awsIntents := []otterizev2alpha1.Target{
		{
			AWS: &otterizev2alpha1.AWSTarget{
				ARN: "aws::s3::bucket::bucket-name",
			},
		},
	}

	clientIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientIntentsName,
			Namespace: testNamespace,
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{Name: serviceName},
			Targets:  awsIntents,
		},
	}

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

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&clientIntents)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *otterizev2alpha1.ClientIntents, arg3 ...client.GetOption) error {
			clientIntents.DeepCopyInto(arg2)
			return nil
		},
	)

	s.serviceResolver.EXPECT().ResolveClientIntentToPod(gomock.Any(), gomock.Eq(clientIntents)).Return(clientPod, nil)
	s.iamAgent.EXPECT().AppliesOnPod(gomock.AssignableToTypeOf(&clientPod)).Return(true)
	s.iamAgent.EXPECT().IntentType().Return(testIntentType)
	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&otterizev2alpha1.ClientIntentsList{}),
		&client.ListOptions{Namespace: testNamespace},
	).Return(nil)
	s.iamAgent.EXPECT().AddRolePolicyFromIntents(gomock.Any(), testNamespace, clientServiceAccount, serviceName, []otterizev2alpha1.Target{}, clientPod).Return(nil)

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
