package serviceaccount_old

import (
	"context"
	"errors"
	awstypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	mock_serviceaccount_old "github.com/otterize/credentials-operator/src/controllers/aws_iam/serviceaccount_old/mocks"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	mock_client "github.com/otterize/credentials-operator/src/mocks/controller-runtime/client"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"testing"
)

type TestServiceAccountSuite struct {
	suite.Suite
	controller *gomock.Controller
	client     *mock_client.MockClient
	mockAWS    *mock_serviceaccount_old.MockAWSRolePolicyManager
	reconciler *ServiceAccountReconciler
}

func (s *TestServiceAccountSuite) SetupTest() {
	s.controller = gomock.NewController(s.T())
	s.client = mock_client.NewMockClient(s.controller)
	s.mockAWS = mock_serviceaccount_old.NewMockAWSRolePolicyManager(s.controller)
	s.reconciler = NewServiceAccountReconciler(s.client, s.mockAWS, false)
}

const (
	testPodName            = "pod"
	testNamespace          = "namespace"
	testServiceAccountName = "serviceaccount"
	testPodUID             = "pod-uid"
	testRoleARN            = "role-arn"
	testRoleName           = "role-name"
)

// Tests:
// 1. SA not being deleted and is not modified.
// 2. SA deleted but no finalizer and is not modified.
// 3. SA with finalizer causes role delete.
// 4. SA with finalizer causes deletion to role but role is 404 so sa is terminated successfully.
// 5. SA with finalizer causes update to role but role update returns error so is retried, and terminates successfully on second attempt.

func (s *TestServiceAccountSuite) TestServiceAccountSuite_ServiceAccountNotTerminatingAndHasPodsNotAffected() {
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testServiceAccountName},
	}

	serviceAccount := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testServiceAccountName,
			Namespace:   testNamespace,
			Annotations: map[string]string{metadata.ServiceAccountAWSRoleARNAnnotation: testRoleARN},
			Labels:      map[string]string{metadata.OtterizeServiceAccountLabel: metadata.OtterizeServiceAccountHasPodsValue},
			Finalizers:  []string{metadata.AWSRoleFinalizer},
		},
	}

	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&serviceAccount)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.ServiceAccount, arg3 ...client.GetOption) error {
			serviceAccount.DeepCopyInto(arg2)
			return nil
		},
	)
	// Create happens even if the role was not modified
	s.mockAWS.EXPECT().CreateOtterizeIAMRole(gomock.Any(), testNamespace, testServiceAccountName, false).Return(&awstypes.Role{Arn: lo.ToPtr(testRoleARN)}, nil)

	res, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Empty(res)
}

func (s *TestServiceAccountSuite) TestServiceAccountSuite_ServiceAccountTerminatingWithNoLabelIsNotAffected() {
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testServiceAccountName},
	}

	serviceAccount := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:              testServiceAccountName,
			Namespace:         testNamespace,
			Annotations:       map[string]string{metadata.ServiceAccountAWSRoleARNAnnotation: testRoleARN},
			DeletionTimestamp: lo.ToPtr(metav1.Now()),
			Finalizers:        []string{metadata.AWSRoleFinalizer},
		},
	}

	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&serviceAccount)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.ServiceAccount, arg3 ...client.GetOption) error {
			serviceAccount.DeepCopyInto(arg2)
			return nil
		},
	)

	res, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Empty(res)
}

func (s *TestServiceAccountSuite) TestServiceAccountSuite_ServiceAccountTerminatingWithLabelAndFinalizerRemovesRoleAndFinalizer() {
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testServiceAccountName},
	}

	serviceAccount := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:              testServiceAccountName,
			Namespace:         testNamespace,
			Annotations:       map[string]string{metadata.ServiceAccountAWSRoleARNAnnotation: testRoleARN},
			Labels:            map[string]string{metadata.OtterizeServiceAccountLabel: metadata.OtterizeServiceAccountHasPodsValue},
			DeletionTimestamp: lo.ToPtr(metav1.Now()),
			Finalizers:        []string{metadata.AWSRoleFinalizer},
		},
	}

	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&serviceAccount)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.ServiceAccount, arg3 ...client.GetOption) error {
			serviceAccount.DeepCopyInto(arg2)
			return nil
		},
	)

	s.mockAWS.EXPECT().DeleteOtterizeIAMRole(context.Background(), testNamespace, testServiceAccountName).Return(nil)

	updatedServiceAccount := serviceAccount.DeepCopy()
	s.Require().True(controllerutil.RemoveFinalizer(updatedServiceAccount, metadata.AWSRoleFinalizer))
	s.client.EXPECT().Patch(gomock.Any(), updatedServiceAccount, gomock.Any())

	res, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Empty(res)
}

func (s *TestServiceAccountSuite) TestServiceAccountSuite_ServiceAccountServiceAccountLabeledNoPodsDeletesRoleAndDoesntRemoveFinalizer() {
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testServiceAccountName},
	}

	serviceAccount := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testServiceAccountName,
			Namespace:   testNamespace,
			Annotations: map[string]string{metadata.ServiceAccountAWSRoleARNAnnotation: testRoleARN},
			Labels:      map[string]string{metadata.OtterizeServiceAccountLabel: metadata.OtterizeServiceAccountHasNoPodsValue},
			Finalizers:  []string{metadata.AWSRoleFinalizer},
		},
	}

	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&serviceAccount)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.ServiceAccount, arg3 ...client.GetOption) error {
			serviceAccount.DeepCopyInto(arg2)
			return nil
		},
	)

	s.mockAWS.EXPECT().DeleteOtterizeIAMRole(context.Background(), testNamespace, testServiceAccountName).Return(nil)

	res, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Empty(res)
}

func (s *TestServiceAccountSuite) TestServiceAccountSuite_ServiceAccountServiceAccountTerminatingButRoleDeletionFailsSoDoesntRemoveFinalizer() {
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testServiceAccountName},
	}

	serviceAccount := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:              testServiceAccountName,
			Namespace:         testNamespace,
			Annotations:       map[string]string{metadata.ServiceAccountAWSRoleARNAnnotation: testRoleARN},
			Labels:            map[string]string{metadata.OtterizeServiceAccountLabel: metadata.OtterizeServiceAccountHasPodsValue},
			DeletionTimestamp: lo.ToPtr(metav1.Now()),
			Finalizers:        []string{metadata.AWSRoleFinalizer},
		},
	}

	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&serviceAccount)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.ServiceAccount, arg3 ...client.GetOption) error {
			serviceAccount.DeepCopyInto(arg2)
			return nil
		},
	)

	s.mockAWS.EXPECT().DeleteOtterizeIAMRole(context.Background(), testNamespace, testServiceAccountName).Return(errors.New("role deletion failed"))

	res, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().ErrorContains(err, "role deletion failed")
	s.Require().Empty(res)
}

func (s *TestServiceAccountSuite) TestServiceAccountSuite_CreateIAMRole_SameARN_NoPatch() {
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testServiceAccountName},
	}

	serviceAccount := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testServiceAccountName,
			Namespace:   testNamespace,
			Annotations: map[string]string{metadata.ServiceAccountAWSRoleARNAnnotation: testRoleARN},
			Labels:      map[string]string{metadata.OtterizeServiceAccountLabel: metadata.OtterizeServiceAccountHasPodsValue},
			Finalizers:  []string{metadata.AWSRoleFinalizer},
		},
	}
	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&serviceAccount)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.ServiceAccount, arg3 ...client.GetOption) error {
			serviceAccount.DeepCopyInto(arg2)
			return nil
		},
	)
	s.mockAWS.EXPECT().CreateOtterizeIAMRole(gomock.Any(), testNamespace, testServiceAccountName, false).Return(&awstypes.Role{Arn: lo.ToPtr(testRoleARN)}, nil)

	_, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
}

func (s *TestServiceAccountSuite) TestServiceAccountSuite_CreateIAMRole_NoArn_Patch() {
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testServiceAccountName},
	}

	serviceAccount := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:       testServiceAccountName,
			Namespace:  testNamespace,
			Labels:     map[string]string{metadata.OtterizeServiceAccountLabel: metadata.OtterizeServiceAccountHasPodsValue},
			Finalizers: []string{metadata.AWSRoleFinalizer},
		},
	}
	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&serviceAccount)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.ServiceAccount, arg3 ...client.GetOption) error {
			serviceAccount.DeepCopyInto(arg2)
			return nil
		},
	)
	s.mockAWS.EXPECT().CreateOtterizeIAMRole(gomock.Any(), testNamespace, testServiceAccountName, false).Return(&awstypes.Role{Arn: lo.ToPtr(testRoleARN)}, nil)
	s.client.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any())

	_, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
}

func (s *TestServiceAccountSuite) TestServiceAccountSuite_CreateIAMRoleWithMarkAsUnusedLabel_SameArn() {
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testServiceAccountName},
	}

	serviceAccount := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testServiceAccountName,
			Namespace:   testNamespace,
			Annotations: map[string]string{metadata.ServiceAccountAWSRoleARNAnnotation: testRoleARN},
			Labels:      map[string]string{metadata.OtterizeServiceAccountLabel: metadata.OtterizeServiceAccountHasPodsValue, metadata.OtterizeAWSUseSoftDeleteKey: "true"},
			Finalizers:  []string{metadata.AWSRoleFinalizer},
		},
	}
	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&serviceAccount)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.ServiceAccount, arg3 ...client.GetOption) error {
			serviceAccount.DeepCopyInto(arg2)
			return nil
		},
	)

	s.mockAWS.EXPECT().CreateOtterizeIAMRole(gomock.Any(), testNamespace, testServiceAccountName, true).Return(&awstypes.Role{Arn: lo.ToPtr(testRoleARN)}, nil)

	_, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
}

func TestRunServiceAccountControllerSuite(t *testing.T) {
	suite.Run(t, new(TestServiceAccountSuite))
}
