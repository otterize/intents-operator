package protected_services_reconcilers

import (
	"context"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/spf13/viper"
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
	testNamespace                        = "test-namespace"
)

type DefaultDenyReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	reconciler *DefaultDenyReconciler
}

func (s *DefaultDenyReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()

	scheme := runtime.NewScheme()
	s.reconciler = NewDefaultDenyReconciler(s.Client, scheme)
	viper.Set(operatorconfig.EnableProtectedServicesKey, true)
}

func (s *DefaultDenyReconcilerTestSuite) TearDownTest() {
	viper.Reset()
	s.reconciler = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *DefaultDenyReconcilerTestSuite) TestProtectedServicesCreate() {
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
		otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
	}).Return(nil).Times(1)

	// Create network policy
	formattedServerName := protectedServiceFormattedName
	policy := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-deny-test-service",
			Namespace: testNamespace,
			Labels: map[string]string{
				otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:                   formattedServerName,
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

func (s *DefaultDenyReconcilerTestSuite) TestProtectedServicesCreateFromMultipleLists() {
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
		otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
	}).Return(nil).Times(1)

	// Create two network policies
	formattedServerName := protectedServiceFormattedName
	formattedOtherServerName := anotherProtectedServiceFormattedName
	serverPolicy := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-deny-test-service",
			Namespace: testNamespace,
			Labels: map[string]string{
				otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:                   formattedServerName,
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
				otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:                   formattedOtherServerName,
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
func (s *DefaultDenyReconcilerTestSuite) TestProtectedServiceNotInList() {
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
				otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:                   formattedServerName,
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
		otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
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

func (s *DefaultDenyReconcilerTestSuite) TestProtectedServiceResourceBeingDeleted() {
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
				otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:                   formattedServerName,
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
		otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
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

func (s *DefaultDenyReconcilerTestSuite) TestProtectedServiceResourceAlreadyDeleted() {
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
				otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:                   formattedServerName,
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
		otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
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

func (s *DefaultDenyReconcilerTestSuite) TestProtectedServiceAlreadyExists() {
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
				otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:                   formattedServerName,
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
				otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:                   formattedOtherServerName,
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
		otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
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

func (s *DefaultDenyReconcilerTestSuite) TestProtectedServiceUpdate() {
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
				otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:                   formattedServerName,
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
				otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:                   formattedServerName,
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
		otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
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

func (s *DefaultDenyReconcilerTestSuite) TestDeleteAllWhenFeatureDisabled() {
	viper.Set(operatorconfig.EnableProtectedServicesKey, false)

	formattedServerName := protectedServiceFormattedName
	serverPolicy := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-deny-test-service",
			Namespace: testNamespace,
			Labels: map[string]string{
				otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:                   formattedServerName,
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

	formattedOtherServerName := anotherProtectedServiceFormattedName
	otherServerPolicy := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-deny-other-test-service",
			Namespace: testNamespace,
			Labels: map[string]string{
				otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:                   formattedOtherServerName,
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
		otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
	}).DoAndReturn(
		func(ctx context.Context, list *v1.NetworkPolicyList, opts ...client.ListOption) error {
			list.Items = append(list.Items, serverPolicy, otherServerPolicy)
			return nil
		})
	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      protectedServicesResourceName,
		},
	}

	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(&serverPolicy)).Return(nil)
	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(&otherServerPolicy)).Return(nil)

	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.Require().Empty(res)
	s.Require().NoError(err)

}

func TestDefaultDenyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(DefaultDenyReconcilerTestSuite))
}