package controllers

import (
	"context"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"
)

const (
	protectedServicesResourceName        = "staging-protected-services"
	protectedService                     = "test-service"
	protectedServiceFormattedName        = "test-service-test-namespace-b0207e"
	anotherProtectedService              = "other-test-service"
	anotherProtectedServiceFormattedName = "other-test-service-test-namespace-398a04"
)

type ProtectedServiceReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	reconciler *ProtectedServicesReconciler
}

func (s *ProtectedServiceReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()

	scheme := runtime.NewScheme()
	s.reconciler = NewProtectedServicesReconciler(s.Client, scheme)
}

func (s *ProtectedServiceReconcilerTestSuite) TestProtectedServicesCreate() {
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
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServicesList{}), client.InNamespace(testNamespace)).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ProtectedServicesList, opts ...client.ListOption) error {
			protectedServicesResources.DeepCopyInto(list)
			return nil
		})

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      protectedServicesResourceName,
		},
	}

	// Get all existing network policies
	// No network policies exist
	var networkPolicies v1.NetworkPolicyList
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&networkPolicies), client.InNamespace(testNamespace), client.MatchingLabels{
		otterizev1alpha2.OtterizeNetworkPolicyDefaultDeny: "true",
	}).Return(nil).Times(1)

	// Create network policy
	formattedServerName := protectedServiceFormattedName
	policy := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-deny-test-service",
			Namespace: testNamespace,
			Labels: map[string]string{
				otterizev1alpha2.OtterizeNetworkPolicyDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:            formattedServerName,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha2.OtterizeServerLabelKey: formattedServerName,
				},
			},
			Ingress: []v1.NetworkPolicyIngressRule{},
		},
	}
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(&policy)).Return(nil).Times(1)

	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.Require().Empty(res)
	s.Require().NoError(err)
}

func (s *ProtectedServiceReconcilerTestSuite) TestProtectedServicesCreateFromMultipleLists() {
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
				Name:      protectedServicesResourceName,
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
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServicesList{}), client.InNamespace(testNamespace)).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ProtectedServicesList, opts ...client.ListOption) error {
			protectedServicesResources.DeepCopyInto(list)
			return nil
		})

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      protectedServicesResourceName,
		},
	}

	// Get all existing network policies
	// No network policies exist
	var networkPolicies v1.NetworkPolicyList
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&networkPolicies), client.InNamespace(testNamespace), client.MatchingLabels{
		otterizev1alpha2.OtterizeNetworkPolicyDefaultDeny: "true",
	}).Return(nil).Times(1)

	// Create two network policies
	formattedServerName := protectedServiceFormattedName
	formattedOtherServerName := anotherProtectedServiceFormattedName
	serverPolicy := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-deny-test-service",
			Namespace: testNamespace,
			Labels: map[string]string{
				otterizev1alpha2.OtterizeNetworkPolicyDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:            formattedServerName,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha2.OtterizeServerLabelKey: formattedServerName,
				},
			},
			Ingress: []v1.NetworkPolicyIngressRule{},
		},
	}
	otherServerPolicy := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-deny-other-test-service",
			Namespace: testNamespace,
			Labels: map[string]string{
				otterizev1alpha2.OtterizeNetworkPolicyDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:            formattedOtherServerName,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha2.OtterizeServerLabelKey: formattedOtherServerName,
				},
			},
			Ingress: []v1.NetworkPolicyIngressRule{},
		},
	}

	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(&serverPolicy)).Return(nil).Times(1)
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(&otherServerPolicy)).Return(nil).Times(1)

	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.Require().Empty(res)
	s.Require().NoError(err)
}

// TestDeleteProtectedServices tests the deletion of a protected service when the service is no longer in the list of protected services
func (s *ProtectedServiceReconcilerTestSuite) TestProtectedServiceNotInList() {
	var protectedServicesResources otterizev1alpha2.ProtectedServicesList
	protectedServicesResources.Items = []otterizev1alpha2.ProtectedServices{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      protectedServicesResourceName,
				Namespace: testNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServicesSpec{
				ProtectedServices: []otterizev1alpha2.ProtectedService{},
			},
		},
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServicesList{}), client.InNamespace(testNamespace)).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ProtectedServicesList, opts ...client.ListOption) error {
			protectedServicesResources.DeepCopyInto(list)
			return nil
		})

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      protectedServicesResourceName,
		},
	}

	// Get all existing network policies
	// An old policy exist

	formattedServerName := protectedServiceFormattedName
	policy := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-deny-test-service",
			Namespace: testNamespace,
			Labels: map[string]string{
				otterizev1alpha2.OtterizeNetworkPolicyDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:            formattedServerName,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha2.OtterizeServerLabelKey: formattedServerName,
				},
			},
			Ingress: []v1.NetworkPolicyIngressRule{},
		},
	}

	var networkPolicies v1.NetworkPolicyList
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&networkPolicies), client.InNamespace(testNamespace), client.MatchingLabels{
		otterizev1alpha2.OtterizeNetworkPolicyDefaultDeny: "true",
	}).DoAndReturn(
		func(ctx context.Context, list *v1.NetworkPolicyList, opts ...client.ListOption) error {
			list.Items = append(list.Items, policy)
			return nil
		})

	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(&policy)).Return(nil).Times(1)

	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.Require().Empty(res)
	s.Require().NoError(err)
}

func (s *ProtectedServiceReconcilerTestSuite) TestProtectedServiceResourceBeingDeleted() {
	var protectedServicesResources otterizev1alpha2.ProtectedServicesList
	protectedServicesResources.Items = []otterizev1alpha2.ProtectedServices{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              protectedServicesResourceName,
				Namespace:         testNamespace,
				DeletionTimestamp: &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)},
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

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      protectedServicesResourceName,
		},
	}

	// Get all existing network policies
	// An old policy exist
	formattedServerName := protectedServiceFormattedName
	policy := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-deny-test-service",
			Namespace: testNamespace,
			Labels: map[string]string{
				otterizev1alpha2.OtterizeNetworkPolicyDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:            formattedServerName,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha2.OtterizeServerLabelKey: formattedServerName,
				},
			},
			Ingress: []v1.NetworkPolicyIngressRule{},
		},
	}

	var networkPolicies v1.NetworkPolicyList
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&networkPolicies), client.InNamespace(testNamespace), client.MatchingLabels{
		otterizev1alpha2.OtterizeNetworkPolicyDefaultDeny: "true",
	}).DoAndReturn(
		func(ctx context.Context, list *v1.NetworkPolicyList, opts ...client.ListOption) error {
			list.Items = append(list.Items, policy)
			return nil
		})

	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(&policy)).Return(nil).Times(1)

	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.Require().Empty(res)
	s.Require().NoError(err)
}

func (s *ProtectedServiceReconcilerTestSuite) TestProtectedServiceResourceAlreadyDeleted() {
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServicesList{}), client.InNamespace(testNamespace)).Return(nil)

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      protectedServicesResourceName,
		},
	}

	// Get all existing network policies
	// An old policy exist
	formattedServerName := protectedServiceFormattedName
	policy := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-deny-test-service",
			Namespace: testNamespace,
			Labels: map[string]string{
				otterizev1alpha2.OtterizeNetworkPolicyDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:            formattedServerName,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha2.OtterizeServerLabelKey: formattedServerName,
				},
			},
			Ingress: []v1.NetworkPolicyIngressRule{},
		},
	}

	var networkPolicies v1.NetworkPolicyList
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&networkPolicies), client.InNamespace(testNamespace), client.MatchingLabels{
		otterizev1alpha2.OtterizeNetworkPolicyDefaultDeny: "true",
	}).DoAndReturn(
		func(ctx context.Context, list *v1.NetworkPolicyList, opts ...client.ListOption) error {
			list.Items = append(list.Items, policy)
			return nil
		})

	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(&policy)).Return(nil).Times(1)

	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.Require().Empty(res)
	s.Require().NoError(err)
}

func (s *ProtectedServiceReconcilerTestSuite) TestProtectedServiceAlreadyExists() {
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

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      protectedServicesResourceName,
		},
	}

	formattedServerName := protectedServiceFormattedName
	policy := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-deny-test-service",
			Namespace: testNamespace,
			Labels: map[string]string{
				otterizev1alpha2.OtterizeNetworkPolicyDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:            formattedServerName,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha2.OtterizeServerLabelKey: formattedServerName,
				},
			},
			Ingress: []v1.NetworkPolicyIngressRule{},
		},
	}

	// We only update the policy if labels or spec changed, since those are the only fields we currently write to
	// this one has its own annotations solely to test that we don't update on other differences
	formattedOtherServerName := anotherProtectedServiceFormattedName
	policyWithAnnotation := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-deny-other-test-service",
			Namespace: testNamespace,
			Labels: map[string]string{
				otterizev1alpha2.OtterizeNetworkPolicyDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:            formattedOtherServerName,
			},
			Annotations: map[string]string{
				"my-annotation": "my-value",
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha2.OtterizeServerLabelKey: formattedOtherServerName,
				},
			},
			Ingress: []v1.NetworkPolicyIngressRule{},
		},
	}

	var networkPolicies v1.NetworkPolicyList
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&networkPolicies), client.InNamespace(testNamespace), client.MatchingLabels{
		otterizev1alpha2.OtterizeNetworkPolicyDefaultDeny: "true",
	}).DoAndReturn(
		func(ctx context.Context, list *v1.NetworkPolicyList, opts ...client.ListOption) error {
			list.Items = append(list.Items, policy, policyWithAnnotation)
			return nil
		})

	// We expect no other calls to the client since the policy already exists and is valid

	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.Require().Empty(res)
	s.Require().NoError(err)
}

func (s *ProtectedServiceReconcilerTestSuite) TestProtectedServiceUpdate() {
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
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServicesList{}), client.InNamespace(testNamespace)).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ProtectedServicesList, opts ...client.ListOption) error {
			protectedServicesResources.DeepCopyInto(list)
			return nil
		})

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      protectedServicesResourceName,
		},
	}

	formattedServerName := protectedServiceFormattedName
	oldPolicy := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-deny-test-service",
			Namespace: testNamespace,
			Labels: map[string]string{
				otterizev1alpha2.OtterizeNetworkPolicyDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:            formattedServerName,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"bad-pod-selector": "just-to-prove-a-point",
				},
			},
			Ingress: []v1.NetworkPolicyIngressRule{},
		},
	}

	fixedPolicy := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-deny-test-service",
			Namespace: testNamespace,
			Labels: map[string]string{
				otterizev1alpha2.OtterizeNetworkPolicyDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:            formattedServerName,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha2.OtterizeServerLabelKey: formattedServerName,
				},
			},
			Ingress: []v1.NetworkPolicyIngressRule{},
		},
	}
	var networkPolicies v1.NetworkPolicyList
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&networkPolicies), client.InNamespace(testNamespace), client.MatchingLabels{
		otterizev1alpha2.OtterizeNetworkPolicyDefaultDeny: "true",
	}).DoAndReturn(
		func(ctx context.Context, list *v1.NetworkPolicyList, opts ...client.ListOption) error {
			list.Items = append(list.Items, oldPolicy)
			return nil
		})

	s.Client.EXPECT().Update(gomock.Any(), gomock.Eq(&fixedPolicy)).Return(nil)

	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.Require().Empty(res)
	s.Require().NoError(err)
}

func (s *ProtectedServiceReconcilerTestSuite) TearDownTest() {
	s.reconciler = nil
	s.MocksSuiteBase.TearDownTest()
}

func TestProtectedServiceReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(ProtectedServiceReconcilerTestSuite))
}
