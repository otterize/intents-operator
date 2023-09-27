package protected_service_reconcilers

import (
	"context"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/operator/controllers/protected_service_reconcilers/consts"
	protectedservicesmock "github.com/otterize/intents-operator/src/operator/controllers/protected_service_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"testing"
	"time"
)

const (
	protectedServicesResourceName        = "staging-protected-services"
	protectedService                     = "test-service"
	protectedServiceFormattedName        = "test-service-test-namespace-b0207e"
	anotherProtectedServiceResourceName  = "protect-other-services"
	anotherProtectedService              = "other-test-service"
	anotherProtectedServiceFormattedName = "other-test-service-test-namespace-398a04"
	testNamespace                        = "test-namespace"
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

type DefaultDenyReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	reconciler       *DefaultDenyReconciler
	extNetpolHandler *protectedservicesmock.MockExternalNepolHandler
}

func (s *DefaultDenyReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()

	s.extNetpolHandler = protectedservicesmock.NewMockExternalNepolHandler(s.Controller)
	s.reconciler = NewDefaultDenyReconciler(s.Client, s.extNetpolHandler, true)
}

func (s *DefaultDenyReconcilerTestSuite) TearDownTest() {
	viper.Reset()
	s.reconciler = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *DefaultDenyReconcilerTestSuite) TestProtectedServicesCreateGlobalNetpolDisabled() {
	s.ignoreFinalizerHandling()
	s.reconciler.netpolEnforcementEnabled = false

	var protectedServicesResources otterizev1alpha2.ProtectedServiceList
	protectedServicesResources.Items = []otterizev1alpha2.ProtectedService{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      protectedServicesResourceName,
				Namespace: testNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: protectedService,
			},
		},
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServiceList{}), client.InNamespace(testNamespace)).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ProtectedServiceList, opts ...client.ListOption) error {
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

	s.extNetpolHandler.EXPECT().HandleAllPods(gomock.Any())
	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.Require().Empty(res)
	s.Require().NoError(err)
}

func (s *DefaultDenyReconcilerTestSuite) TestProtectedServicesCreate() {
	s.ignoreFinalizerHandling()

	var protectedServicesResources otterizev1alpha2.ProtectedServiceList
	protectedServicesResources.Items = []otterizev1alpha2.ProtectedService{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      protectedServicesResourceName,
				Namespace: testNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: protectedService,
			},
		},
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServiceList{}), client.InNamespace(testNamespace)).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ProtectedServiceList, opts ...client.ListOption) error {
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

	s.extNetpolHandler.EXPECT().HandleAllPods(gomock.Any())
	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.Require().Empty(res)
	s.Require().NoError(err)
}

func (s *DefaultDenyReconcilerTestSuite) TestProtectedServicesCreateFromMultipleLists() {
	s.ignoreFinalizerHandling()

	var protectedServicesResources otterizev1alpha2.ProtectedServiceList
	protectedServicesResources.Items = []otterizev1alpha2.ProtectedService{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      protectedServicesResourceName,
				Namespace: testNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: protectedService,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      protectedServicesResourceName,
				Namespace: testNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: anotherProtectedService,
			},
		},
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServiceList{}), client.InNamespace(testNamespace)).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ProtectedServiceList, opts ...client.ListOption) error {
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

	s.extNetpolHandler.EXPECT().HandleAllPods(gomock.Any())
	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.Require().Empty(res)
	s.Require().NoError(err)
}

// TestDeleteProtectedServices tests the deletion of a protected service when the service is no longer in the list of protected services
func (s *DefaultDenyReconcilerTestSuite) TestProtectedServiceNotInList() {
	s.ignoreFinalizerHandling()

	var protectedServicesResources otterizev1alpha2.ProtectedServiceList
	protectedServicesResources.Items = []otterizev1alpha2.ProtectedService{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      protectedServicesResourceName,
				Namespace: testNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: anotherProtectedService,
			},
		},
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServiceList{}), client.InNamespace(testNamespace)).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ProtectedServiceList, opts ...client.ListOption) error {
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

	otherProtectedServicePolicy := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-deny-other-test-service",
			Namespace: testNamespace,
			Labels: map[string]string{
				otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny: "true",
				otterizev1alpha2.OtterizeNetworkPolicy:                   anotherProtectedServiceFormattedName,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha2.OtterizeServerLabelKey: anotherProtectedServiceFormattedName,
				},
			},
			Ingress: []v1.NetworkPolicyIngressRule{},
		},
	}
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(&otherProtectedServicePolicy)).Return(nil).Times(1)
	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(&policy)).Return(nil).Times(1)
	s.extNetpolHandler.EXPECT().HandleAllPods(gomock.Any())

	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.Require().Empty(res)
	s.Require().NoError(err)
}

func (s *DefaultDenyReconcilerTestSuite) TestProtectedServiceResourceBeingDeleted() {
	s.ignoreFinalizerHandling()

	var protectedServicesResources otterizev1alpha2.ProtectedServiceList
	protectedServicesResources.Items = []otterizev1alpha2.ProtectedService{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              protectedServicesResourceName,
				Namespace:         testNamespace,
				DeletionTimestamp: &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)},
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: protectedService,
			},
		},
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServiceList{}), client.InNamespace(testNamespace)).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ProtectedServiceList, opts ...client.ListOption) error {
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

	s.extNetpolHandler.EXPECT().HandleAllPods(gomock.Any())
	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.Require().Empty(res)
	s.Require().NoError(err)
}

func (s *DefaultDenyReconcilerTestSuite) TestProtectedServiceResourceAlreadyDeleted() {
	s.ignoreFinalizerHandling()

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServiceList{}), client.InNamespace(testNamespace)).Return(nil)

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

	s.extNetpolHandler.EXPECT().HandleAllPods(gomock.Any())
	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.Require().Empty(res)
	s.Require().NoError(err)
}

func (s *DefaultDenyReconcilerTestSuite) TestProtectedServiceAlreadyExists() {
	s.ignoreFinalizerHandling()

	var protectedServicesResources otterizev1alpha2.ProtectedServiceList
	protectedServicesResources.Items = []otterizev1alpha2.ProtectedService{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      protectedServicesResourceName,
				Namespace: testNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: protectedService,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      protectedServicesResourceName,
				Namespace: testNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: anotherProtectedService,
			},
		},
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServiceList{}), client.InNamespace(testNamespace)).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ProtectedServiceList, opts ...client.ListOption) error {
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

	s.extNetpolHandler.EXPECT().HandleAllPods(gomock.Any())

	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.Require().Empty(res)
	s.Require().NoError(err)
}

func (s *DefaultDenyReconcilerTestSuite) TestProtectedServiceUpdate() {
	s.ignoreFinalizerHandling()
	var protectedServicesResources otterizev1alpha2.ProtectedServiceList
	protectedServicesResources.Items = []otterizev1alpha2.ProtectedService{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      protectedServicesResourceName,
				Namespace: testNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: protectedService,
			},
		},
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServiceList{}), client.InNamespace(testNamespace)).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ProtectedServiceList, opts ...client.ListOption) error {
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

	s.extNetpolHandler.EXPECT().HandleAllPods(gomock.Any())
	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.Require().Empty(res)
	s.Require().NoError(err)
}

func (s *DefaultDenyReconcilerTestSuite) TestFinalizerAdd() {
	resourceWithoutFinalizer := otterizev1alpha2.ProtectedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      protectedServicesResourceName,
			Namespace: testNamespace,
		},
		Spec: otterizev1alpha2.ProtectedServiceSpec{

			Name: protectedService,
		},
	}

	resourceWithFinalizer := resourceWithoutFinalizer.DeepCopy()
	resourceWithFinalizer.ObjectMeta.Finalizers = []string{
		consts.DefaultDenyReconcilerFinalizerName,
	}

	nameSpacedName := types.NamespacedName{
		Name:      protectedServicesResourceName,
		Namespace: testNamespace,
	}

	s.Client.EXPECT().Get(gomock.Any(), gomock.Eq(nameSpacedName), gomock.Any()).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, protectedService *otterizev1alpha2.ProtectedService, opts ...client.GetOption) error {
			resourceWithoutFinalizer.DeepCopyInto(protectedService)
			return nil
		})

	s.Client.EXPECT().Update(gomock.Any(), gomock.Eq(resourceWithFinalizer)).Return(nil)

	// Ignore the rest of the logic, it's tested in other tests
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServiceList{}), client.InNamespace(testNamespace)).Return(nil)
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), client.InNamespace(testNamespace)).Return(nil)
	s.extNetpolHandler.EXPECT().HandleAllPods(gomock.Any())

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      protectedServicesResourceName,
		},
	}

	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.NoError(err)
	s.Empty(res)
}

func (s *DefaultDenyReconcilerTestSuite) TestFinalizerRemoved() {
	resourceWithFinalizer := otterizev1alpha2.ProtectedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:              protectedServicesResourceName,
			Namespace:         testNamespace,
			DeletionTimestamp: &metav1.Time{Time: time.Date(2023, 9, 13, 18, 15, 0, 0, time.UTC)},
			Finalizers: []string{
				consts.DefaultDenyReconcilerFinalizerName,
			},
		},
		Spec: otterizev1alpha2.ProtectedServiceSpec{

			Name: protectedService,
		},
	}

	resourceWithoutFinalizer := resourceWithFinalizer.DeepCopy()
	controllerutil.RemoveFinalizer(resourceWithoutFinalizer, consts.DefaultDenyReconcilerFinalizerName)

	nameSpacedName := types.NamespacedName{
		Name:      protectedServicesResourceName,
		Namespace: testNamespace,
	}

	s.Client.EXPECT().Get(gomock.Any(), gomock.Eq(nameSpacedName), gomock.Any()).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, protectedService *otterizev1alpha2.ProtectedService, opts ...client.GetOption) error {
			resourceWithFinalizer.DeepCopyInto(protectedService)
			return nil
		})

	s.Client.EXPECT().Update(gomock.Any(), gomock.Eq(resourceWithoutFinalizer)).Return(nil)

	// Ignore the rest of the logic, it's tested in other tests
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServiceList{}), client.InNamespace(testNamespace)).Return(nil)
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), client.InNamespace(testNamespace)).Return(nil)
	s.extNetpolHandler.EXPECT().HandleAllPods(gomock.Any())

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      protectedServicesResourceName,
		},
	}

	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.NoError(err)
	s.Empty(res)
}

func (s *DefaultDenyReconcilerTestSuite) ignoreFinalizerHandling() {
	s.Client.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	s.Client.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)
}

func TestDefaultDenyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(DefaultDenyReconcilerTestSuite))
}
