package protected_service_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
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
	"testing"
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

type PolicyCleanerReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	reconciler       *PolicyCleanerReconciler
	extNetpolHandler *protectedservicesmock.MockExternalNepolHandler
}

func (s *PolicyCleanerReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()

	s.extNetpolHandler = protectedservicesmock.NewMockExternalNepolHandler(s.Controller)
	s.reconciler = NewPolicyCleanerReconciler(s.Client, s.extNetpolHandler)
}

func (s *PolicyCleanerReconcilerTestSuite) TearDownTest() {
	viper.Reset()
	s.reconciler = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *PolicyCleanerReconcilerTestSuite) TestAllServerAreProtected() {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha2.OtterizeNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
		{
			Key:      otterizev1alpha2.OtterizeNetworkPolicyExternalTraffic,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
		{
			Key:      otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
	}})
	s.Require().NoError(err)

	networkPolicies := []v1.NetworkPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy-name",
				Namespace: testNamespace,
				Labels: map[string]string{
					otterizev1alpha2.OtterizeNetworkPolicy: protectedServiceFormattedName,
				},
			},
			Spec: v1.NetworkPolicySpec{
				PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						otterizev1alpha2.OtterizeServerLabelKey: protectedServiceFormattedName,
					},
				},
				Ingress: []v1.NetworkPolicyIngressRule{
					{
						From: []v1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										fmt.Sprintf(
											otterizev1alpha2.OtterizeAccessLabelKey, protectedServiceFormattedName): "true",
									},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										otterizev1alpha2.OtterizeNamespaceLabelKey: testNamespace,
									},
								},
							},
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "another-policy-name",
				Namespace: testNamespace,
				Labels: map[string]string{
					otterizev1alpha2.OtterizeNetworkPolicy: anotherProtectedServiceFormattedName,
				},
			},
			Spec: v1.NetworkPolicySpec{
				PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						otterizev1alpha2.OtterizeServerLabelKey: anotherProtectedServiceFormattedName,
					},
				},
				Ingress: []v1.NetworkPolicyIngressRule{
					{
						From: []v1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										fmt.Sprintf(
											otterizev1alpha2.OtterizeAccessLabelKey, anotherProtectedServiceFormattedName): "true",
									},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										otterizev1alpha2.OtterizeNamespaceLabelKey: testNamespace,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.Eq(&v1.NetworkPolicyList{}),
		&client.ListOptions{Namespace: testNamespace, LabelSelector: selector},
	).DoAndReturn(
		func(ctx context.Context, list *v1.NetworkPolicyList, opts ...client.ListOption) error {
			list.Items = append(list.Items, networkPolicies...)
			return nil
		})

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
				Name:      anotherProtectedServiceResourceName,
				Namespace: testNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: anotherProtectedService,
			},
		},
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServiceList{}), &client.ListOptions{Namespace: testNamespace}).DoAndReturn(
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

	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.Require().Empty(res)
	s.Require().NoError(err)
}

func (s *PolicyCleanerReconcilerTestSuite) TestUnprotectedServerWithAccessPolicy() {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha2.OtterizeNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
		{
			Key:      otterizev1alpha2.OtterizeNetworkPolicyExternalTraffic,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
		{
			Key:      otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
	}})
	s.Require().NoError(err)

	networkPolicies := []v1.NetworkPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy-name",
				Namespace: testNamespace,
				Labels: map[string]string{
					otterizev1alpha2.OtterizeNetworkPolicy: protectedServiceFormattedName,
				},
			},
			Spec: v1.NetworkPolicySpec{
				PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						otterizev1alpha2.OtterizeServerLabelKey: protectedServiceFormattedName,
					},
				},
				Ingress: []v1.NetworkPolicyIngressRule{
					{
						From: []v1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										fmt.Sprintf(
											otterizev1alpha2.OtterizeAccessLabelKey, protectedServiceFormattedName): "true",
									},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										otterizev1alpha2.OtterizeNamespaceLabelKey: testNamespace,
									},
								},
							},
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "another-policy-name",
				Namespace: testNamespace,
				Labels: map[string]string{
					otterizev1alpha2.OtterizeNetworkPolicy: anotherProtectedServiceFormattedName,
				},
			},
			Spec: v1.NetworkPolicySpec{
				PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						otterizev1alpha2.OtterizeServerLabelKey: anotherProtectedServiceFormattedName,
					},
				},
				Ingress: []v1.NetworkPolicyIngressRule{
					{
						From: []v1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										fmt.Sprintf(
											otterizev1alpha2.OtterizeAccessLabelKey, anotherProtectedServiceFormattedName): "true",
									},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										otterizev1alpha2.OtterizeNamespaceLabelKey: testNamespace,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.Eq(&v1.NetworkPolicyList{}),
		&client.ListOptions{Namespace: testNamespace, LabelSelector: selector},
	).DoAndReturn(
		func(ctx context.Context, list *v1.NetworkPolicyList, opts ...client.ListOption) error {
			list.Items = append(list.Items, networkPolicies...)
			return nil
		})

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

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServiceList{}), &client.ListOptions{Namespace: testNamespace}).DoAndReturn(
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

	s.extNetpolHandler.EXPECT().HandleBeforeAccessPolicyRemoval(gomock.Any(), &networkPolicies[1]).Times(1)
	s.Client.EXPECT().Delete(gomock.Any(), &networkPolicies[1]).Times(1)
	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.Require().Empty(res)
	s.Require().NoError(err)
}

func (s *PolicyCleanerReconcilerTestSuite) TestNoNetworkPolicies() {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha2.OtterizeNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
		{
			Key:      otterizev1alpha2.OtterizeNetworkPolicyExternalTraffic,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
		{
			Key:      otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
	}})
	s.Require().NoError(err)
	// Get all existing network policies
	// No network policies exist
	var networkPolicies v1.NetworkPolicyList
	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.Eq(&networkPolicies),
		&client.ListOptions{Namespace: testNamespace, LabelSelector: selector},
	).Return(nil).Times(1)

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

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      protectedServicesResourceName,
		},
	}

	res, err := s.reconciler.Reconcile(context.Background(), request)
	s.Require().Empty(res)
	s.Require().NoError(err)
}

func TestPolicyCleanerReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(PolicyCleanerReconcilerTestSuite))
}
