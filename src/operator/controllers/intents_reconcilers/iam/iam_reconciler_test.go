package iam

import (
	"context"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	mock_iampolicyagents "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/iam/iampolicyagents/mocks"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/shared/agentutils"
	"github.com/otterize/intents-operator/src/shared/errors"
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
	testNamespace            = "test-namespace"
	testServiceName          = "test-client"
	testClientIntentsName    = "client-intents"
	testClientServiceAccount = "test-server-sa"
	testIntentType           = otterizev2alpha1.IntentTypeGCP
)

type IAMIntentsReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	Reconciler      *IAMIntentsReconciler
	serviceResolver *mocks.MockServiceResolver
	iamAgent        *mock_iampolicyagents.MockIAMPolicyAgent
	recorder        *record.FakeRecorder
	scheme          *runtime.Scheme
}

func getTestClientPod(serviceName string, clientServiceAccount string) corev1.Pod {
	return corev1.Pod{
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
}

func getTestIAMIntents(clientIntentsName string, serviceName string, targets []otterizev2alpha1.Target) otterizev2alpha1.ApprovedClientIntents {
	return otterizev2alpha1.ApprovedClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientIntentsName,
			Namespace: testNamespace,
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{Name: serviceName},
			Targets:  targets,
		},
	}
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
	namespacedName := types.NamespacedName{Namespace: testNamespace, Name: testClientIntentsName}
	req := ctrl.Request{NamespacedName: namespacedName}

	clientPod := getTestClientPod(testServiceName, "test-server-sa")
	iamIntents := getTestIAMIntents(testClientIntentsName, testServiceName, []otterizev2alpha1.Target{
		{
			GCP: &otterizev2alpha1.GCPTarget{
				Resource: "projects/_/buckets/bucket-name",
			},
		},
	})

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&iamIntents)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *otterizev2alpha1.ApprovedClientIntents, arg3 ...client.GetOption) error {
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
	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      testClientIntentsName,
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

	clientPod := getTestClientPod(testServiceName, testClientServiceAccount)
	iamIntents := getTestIAMIntents(testClientIntentsName, testServiceName, allIntents)

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&iamIntents)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *otterizev2alpha1.ApprovedClientIntents, arg3 ...client.GetOption) error {
			iamIntents.DeepCopyInto(arg2)
			return nil
		},
	)

	s.serviceResolver.EXPECT().ResolveClientIntentToPod(gomock.Any(), gomock.Eq(iamIntents)).Return(clientPod, nil)
	s.iamAgent.EXPECT().AppliesOnPod(gomock.AssignableToTypeOf(&clientPod)).Return(true)
	s.iamAgent.EXPECT().IntentType().Return(testIntentType)
	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&otterizev2alpha1.ApprovedClientIntentsList{}),
		&client.ListOptions{Namespace: testNamespace},
	).Return(nil)

	clientIntents := otterizev2alpha1.ApprovedClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testClientIntentsName,
			Namespace: testNamespace,
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{Name: testServiceName},
			Targets:  allIntents,
		},
	}

	s.iamAgent.EXPECT().AddRolePolicyFromIntents(gomock.Any(), testNamespace, testClientServiceAccount, testServiceName, clientIntents, filteredIntents, clientPod).Return(nil)

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
	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      testClientIntentsName,
	}
	req := ctrl.Request{NamespacedName: namespacedName}

	awsIntents := []otterizev2alpha1.Target{
		{
			AWS: &otterizev2alpha1.AWSTarget{
				ARN: "aws::s3::bucket::bucket-name",
			},
		},
	}

	clientPod := getTestClientPod(testServiceName, testClientServiceAccount)
	clientIntents := getTestIAMIntents(testClientIntentsName, testServiceName, awsIntents)

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&clientIntents)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *otterizev2alpha1.ApprovedClientIntents, arg3 ...client.GetOption) error {
			clientIntents.DeepCopyInto(arg2)
			return nil
		},
	)

	s.serviceResolver.EXPECT().ResolveClientIntentToPod(gomock.Any(), gomock.Eq(clientIntents)).Return(clientPod, nil)
	s.iamAgent.EXPECT().AppliesOnPod(gomock.AssignableToTypeOf(&clientPod)).Return(true)
	s.iamAgent.EXPECT().IntentType().Return(testIntentType)
	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&otterizev2alpha1.ApprovedClientIntentsList{}),
		&client.ListOptions{Namespace: testNamespace},
	).Return(nil)

	s.iamAgent.EXPECT().AddRolePolicyFromIntents(gomock.Any(), testNamespace, testClientServiceAccount, testServiceName, clientIntents, []otterizev2alpha1.Target{}, clientPod).Return(nil)

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

func (s *IAMIntentsReconcilerTestSuite) TestRoleNotFoundErrorReQueuesEvent() {
	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      testClientIntentsName,
	}
	req := ctrl.Request{NamespacedName: namespacedName}

	clientPod := getTestClientPod(testServiceName, testClientServiceAccount)
	iamIntents := getTestIAMIntents(testClientIntentsName, testServiceName, []otterizev2alpha1.Target{})

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&iamIntents)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *otterizev2alpha1.ApprovedClientIntents, arg3 ...client.GetOption) error {
			iamIntents.DeepCopyInto(arg2)
			return nil
		},
	)

	s.serviceResolver.EXPECT().ResolveClientIntentToPod(gomock.Any(), gomock.Eq(iamIntents)).Return(clientPod, nil)
	s.iamAgent.EXPECT().AppliesOnPod(gomock.AssignableToTypeOf(&clientPod)).Return(true)
	s.iamAgent.EXPECT().IntentType().Return(testIntentType)
	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&otterizev2alpha1.ApprovedClientIntentsList{}),
		&client.ListOptions{Namespace: testNamespace},
	).Return(nil)

	// Throw the sentinel error
	s.iamAgent.EXPECT().AddRolePolicyFromIntents(gomock.Any(), testNamespace, testClientServiceAccount, testServiceName, iamIntents, iamIntents.GetTargetList(), clientPod).Return(
		errors.Errorf("%w: %s", agentutils.ErrCloudIdentityNotFound, "test error"),
	)

	res, err := s.Reconciler.Reconcile(context.Background(), req)

	// Expect no error to be raised in case of role not found and the event to be re-queued
	s.NoError(err)
	s.Equal(res.Requeue, true)
}

func TestIAMIntentsReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(IAMIntentsReconcilerTestSuite))
}
