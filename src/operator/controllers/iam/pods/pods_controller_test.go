package pods

import (
	"context"
	"errors"
	mock_iamcredentialsagents "github.com/otterize/credentials-operator/src/controllers/iam/iamcredentialsagents/mocks"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	mock_client "github.com/otterize/credentials-operator/src/mocks/controller-runtime/client"
	"github.com/otterize/credentials-operator/src/shared/apiutils"
	"github.com/otterize/credentials-operator/src/shared/testutils"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
)

const (
	testPodName             = "pod"
	testNamespace           = "namespace"
	testServiceAccountName  = "serviceaccount"
	testPodUID              = "pod-uid"
	testRoleARN             = "role-arn"
	testRoleName            = "role-name"
	mockFinalizer           = "credentials-operator.otterize.com/mock-finalizer"
	mockServiceAccountLabel = "credentials-operator.otterize.com/mock-service-account-managed"
)

type TestPodsControllerSuite struct {
	suite.Suite
	controller *gomock.Controller
	client     *mock_client.MockClient
	mockIAM    *mock_iamcredentialsagents.MockIAMCredentialsAgent
	reconciler *PodReconciler
}

func (s *TestPodsControllerSuite) SetupTest() {
	s.controller = gomock.NewController(s.T())
	s.client = mock_client.NewMockClient(s.controller)
	s.mockIAM = mock_iamcredentialsagents.NewMockIAMCredentialsAgent(s.controller)
	s.reconciler = NewPodReconciler(s.client, s.mockIAM)
	s.mockIAM.EXPECT().FinalizerName().Return(mockFinalizer).AnyTimes()
	s.mockIAM.EXPECT().ServiceAccountLabel().Return(mockServiceAccountLabel).AnyTimes()
	s.mockIAM.EXPECT().AppliesOnPod(gomock.Any()).Return(true).AnyTimes()
}

func (s *TestPodsControllerSuite) TestPodWithoutLabelsNotAffected() {
	req := testutils.GetTestPodRequestSchema()
	pod := testutils.GetTestPodSchema()
	serviceAccount := testutils.GetTestServiceSchema()

	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&pod)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.Pod, arg3 ...client.GetOption) error {
			pod.DeepCopyInto(arg2)
			return nil
		},
	)
	s.client.EXPECT().Get(gomock.Any(), types.NamespacedName{
		Namespace: serviceAccount.Namespace,
		Name:      serviceAccount.Name,
	}, gomock.AssignableToTypeOf(&serviceAccount)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.ServiceAccount, arg3 ...client.GetOption) error {
			serviceAccount.DeepCopyInto(arg2)
			return nil
		},
	)

	s.mockIAM.EXPECT().OnPodUpdate(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, false, nil)

	res, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Empty(res)
}

func (s *TestPodsControllerSuite) TestPodNotTerminatingNotAffected() {
	req := testutils.GetTestPodRequestSchema()
	pod := testutils.GetTestPodSchema()
	serviceAccount := testutils.GetTestServiceSchema()

	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&pod)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.Pod, arg3 ...client.GetOption) error {
			pod.DeepCopyInto(arg2)
			return nil
		},
	)

	s.client.EXPECT().Get(gomock.Any(), types.NamespacedName{
		Namespace: serviceAccount.Namespace,
		Name:      serviceAccount.Name,
	}, gomock.AssignableToTypeOf(&serviceAccount)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.ServiceAccount, arg3 ...client.GetOption) error {
			serviceAccount.DeepCopyInto(arg2)
			return nil
		},
	)

	s.mockIAM.EXPECT().OnPodUpdate(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, false, nil)

	res, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Empty(res)
}

func (s *TestPodsControllerSuite) TestPodTerminatingWithNoFinalizerIsNotAffected() {
	req := testutils.GetTestPodRequestSchema()

	pod := testutils.GetTestPodSchema()
	pod.DeletionTimestamp = lo.ToPtr(metav1.Now())

	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&pod)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.Pod, arg3 ...client.GetOption) error {
			pod.DeepCopyInto(arg2)
			return nil
		},
	)

	res, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Empty(res)
}

func (s *TestPodsControllerSuite) TestLastPodTerminatingButDifferentPodUIDDoesNotLabelServiceAccountAndRemovesFinalizer() {
	req := testutils.GetTestPodRequestSchema()

	serviceAccount := testutils.GetTestServiceSchema()
	serviceAccount.Labels = map[string]string{mockServiceAccountLabel: metadata.OtterizeServiceAccountHasPodsValue}

	pod := testutils.GetTestPodSchema()
	pod.DeletionTimestamp = lo.ToPtr(metav1.Now())
	pod.Finalizers = []string{mockFinalizer}

	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&pod)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.Pod, arg3 ...client.GetOption) error {
			pod.DeepCopyInto(arg2)
			return nil
		},
	)

	s.client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&corev1.PodList{}),
		client.MatchingFields{apiutils.PodServiceAccountIndexField: serviceAccount.Name},
		gomock.Any(),
	).DoAndReturn(
		func(arg0 context.Context, arg1 *corev1.PodList, arg2 ...client.ListOption) error {
			podList := corev1.PodList{Items: []corev1.Pod{pod}}
			podList.Items[0].UID += "somestring"

			podList.DeepCopyInto(arg1)
			return nil
		},
	)

	// should not update serviceaccount because UID was different
	updatedPod := pod.DeepCopy()
	s.Require().True(controllerutil.RemoveFinalizer(updatedPod, mockFinalizer))

	s.client.EXPECT().Patch(gomock.Any(), updatedPod, gomock.Any())

	res, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Empty(res)
}

func (s *TestPodsControllerSuite) TestLastPodTerminatingWithFinalizerLabelsServiceAccountAndRemovesFinalizer() {
	req := testutils.GetTestPodRequestSchema()

	serviceAccount := testutils.GetTestServiceSchema()
	serviceAccount.Labels = map[string]string{mockServiceAccountLabel: metadata.OtterizeServiceAccountHasPodsValue}

	pod := testutils.GetTestPodSchema()
	pod.DeletionTimestamp = lo.ToPtr(metav1.Now())
	pod.Finalizers = []string{mockFinalizer}

	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&pod)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.Pod, arg3 ...client.GetOption) error {
			pod.DeepCopyInto(arg2)
			return nil
		},
	)

	s.client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&corev1.PodList{}),
		client.MatchingFields{apiutils.PodServiceAccountIndexField: serviceAccount.Name},
		gomock.Any(),
	).DoAndReturn(
		func(arg0 context.Context, arg1 *corev1.PodList, arg2 ...client.ListOption) error {
			podList := corev1.PodList{Items: []corev1.Pod{pod}}

			podList.DeepCopyInto(arg1)
			return nil
		},
	)

	s.client.EXPECT().Get(gomock.Any(), types.NamespacedName{
		Namespace: serviceAccount.Namespace,
		Name:      serviceAccount.Name,
	}, gomock.AssignableToTypeOf(&serviceAccount)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.ServiceAccount, arg3 ...client.GetOption) error {
			serviceAccount.DeepCopyInto(arg2)
			return nil
		},
	)

	updatedServiceAccount := serviceAccount.DeepCopy()
	updatedServiceAccount.Labels = map[string]string{mockServiceAccountLabel: metadata.OtterizeServiceAccountHasNoPodsValue}

	s.client.EXPECT().Patch(gomock.Any(), updatedServiceAccount, gomock.Any())

	updatedPod := pod.DeepCopy()
	s.Require().True(controllerutil.RemoveFinalizer(updatedPod, mockFinalizer))

	s.client.EXPECT().Patch(gomock.Any(), updatedPod, gomock.Any())

	res, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Empty(res)
}

func (s *TestPodsControllerSuite) TestNonLastPodTerminatingDoesNotLabelServiceAccountAndRemovesFinalizer() {
	req := testutils.GetTestPodRequestSchema()

	serviceAccount := testutils.GetTestServiceSchema()
	serviceAccount.Labels = map[string]string{mockServiceAccountLabel: metadata.OtterizeServiceAccountHasPodsValue}

	pod := testutils.GetTestPodSchema()
	pod.DeletionTimestamp = lo.ToPtr(metav1.Now())
	pod.Finalizers = []string{mockFinalizer}

	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&pod)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.Pod, arg3 ...client.GetOption) error {
			pod.DeepCopyInto(arg2)
			return nil
		},
	)

	s.client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&corev1.PodList{}),
		client.MatchingFields{apiutils.PodServiceAccountIndexField: serviceAccount.Name},
		gomock.Any(),
	).DoAndReturn(
		func(arg0 context.Context, arg1 *corev1.PodList, arg2 ...client.ListOption) error {
			pod2 := testutils.GetTestPodSchema()
			pod2.UID += "2"
			pod2.Name += "2"
			pod2.Finalizers = []string{mockFinalizer}

			podList := corev1.PodList{Items: []corev1.Pod{pod, pod2}}
			podList.DeepCopyInto(arg1)
			return nil
		},
	)

	// should not update serviceaccount because it's not the last pod
	updatedPod := pod.DeepCopy()
	s.Require().True(controllerutil.RemoveFinalizer(updatedPod, mockFinalizer))

	s.client.EXPECT().Patch(gomock.Any(), updatedPod, gomock.Any())

	res, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Empty(res)
}

func (s *TestPodsControllerSuite) TestLastPodTerminatingWithFinalizerServiceAccountGoneAndRemovesFinalizerAnyway() {
	req := testutils.GetTestPodRequestSchema()

	serviceAccount := testutils.GetTestServiceSchema()
	serviceAccount.Labels = map[string]string{mockServiceAccountLabel: metadata.OtterizeServiceAccountHasPodsValue}

	pod := testutils.GetTestPodSchema()
	pod.DeletionTimestamp = lo.ToPtr(metav1.Now())
	pod.Finalizers = []string{mockFinalizer}

	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&pod)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.Pod, arg3 ...client.GetOption) error {
			pod.DeepCopyInto(arg2)
			return nil
		},
	)

	s.client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&corev1.PodList{}),
		client.MatchingFields{apiutils.PodServiceAccountIndexField: serviceAccount.Name},
		gomock.Any(),
	).DoAndReturn(
		func(arg0 context.Context, arg1 *corev1.PodList, arg2 ...client.ListOption) error {
			podList := corev1.PodList{Items: []corev1.Pod{pod}}
			podList.DeepCopyInto(arg1)
			return nil
		},
	)

	s.client.EXPECT().Get(gomock.Any(), types.NamespacedName{
		Namespace: serviceAccount.Namespace,
		Name:      serviceAccount.Name,
	}, gomock.AssignableToTypeOf(&serviceAccount)).Return(k8serrors.NewNotFound(schema.GroupResource{}, serviceAccount.Name))

	updatedPod := pod.DeepCopy()
	s.Require().True(controllerutil.RemoveFinalizer(updatedPod, mockFinalizer))

	s.client.EXPECT().Patch(gomock.Any(), updatedPod, gomock.Any())

	res, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Empty(res)
}

func (s *TestPodsControllerSuite) TestLastPodTerminatingWithFinalizerLabelsServiceAccountButIsConflictSoRequeues() {
	req := testutils.GetTestPodRequestSchema()

	serviceAccount := testutils.GetTestServiceSchema()
	serviceAccount.Labels = map[string]string{mockServiceAccountLabel: metadata.OtterizeServiceAccountHasPodsValue}

	pod := testutils.GetTestPodSchema()
	pod.DeletionTimestamp = lo.ToPtr(metav1.Now())
	pod.Finalizers = []string{mockFinalizer}

	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&pod)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.Pod, arg3 ...client.GetOption) error {
			pod.DeepCopyInto(arg2)
			return nil
		},
	)

	s.client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&corev1.PodList{}),
		client.MatchingFields{apiutils.PodServiceAccountIndexField: serviceAccount.Name},
		gomock.Any(),
	).DoAndReturn(
		func(arg0 context.Context, arg1 *corev1.PodList, arg2 ...client.ListOption) error {
			podList := corev1.PodList{Items: []corev1.Pod{pod}}

			podList.DeepCopyInto(arg1)
			return nil
		},
	)

	s.client.EXPECT().Get(gomock.Any(), types.NamespacedName{
		Namespace: serviceAccount.Namespace,
		Name:      serviceAccount.Name,
	}, gomock.AssignableToTypeOf(&serviceAccount)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.ServiceAccount, arg3 ...client.GetOption) error {
			serviceAccount.DeepCopyInto(arg2)
			return nil
		},
	)

	updatedServiceAccount := serviceAccount.DeepCopy()
	updatedServiceAccount.Labels = map[string]string{mockServiceAccountLabel: metadata.OtterizeServiceAccountHasNoPodsValue}

	s.client.EXPECT().Patch(gomock.Any(), updatedServiceAccount, gomock.Any()).Return(k8serrors.NewConflict(schema.GroupResource{}, serviceAccount.Name, errors.New("conflict")))

	res, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Equal(reconcile.Result{Requeue: true}, res)
}

func TestRunPodsControllerSuite(t *testing.T) {
	suite.Run(t, new(TestPodsControllerSuite))
}
