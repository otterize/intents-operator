package external_traffic

import (
	"context"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/allowexternaltraffic"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

type NetworkPolicyHandlerTestSuite struct {
	testbase.MocksSuiteBase
	handler *NetworkPolicyHandler
}

func (s *NetworkPolicyHandlerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.handler = NewNetworkPolicyHandler(s.Client, &runtime.Scheme{}, allowexternaltraffic.IfBlockedByOtterize, make([]serviceidentity.ServiceIdentity, 0), false)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleBeforeAccessPolicyRemoval_createWhenNoIntentsEnabled_doNothing() {
	s.handler.allowExternalTraffic = allowexternaltraffic.Always

	serviceName := "testservice"
	serviceNamespace := "testnamespace"
	toBeRemovedPolicy := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coolPolicy",
			Namespace: serviceNamespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeNetworkPolicy: serviceName,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev2alpha1.OtterizeServiceLabelKey: serviceName,
				},
			},
		},
	}

	s.Require().NoError(s.handler.HandleBeforeAccessPolicyRemoval(context.Background(), toBeRemovedPolicy))
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleBeforeAccessPolicyRemoval_onlyOneRelatedPolicy_shouldDelete() {
	serviceName := "testservice"
	serviceNamespace := "testnamespace"
	toBeRemovedPolicy := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coolPolicy",
			Namespace: serviceNamespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeNetworkPolicy: serviceName,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev2alpha1.OtterizeServiceLabelKey: serviceName,
				},
			},
		},
	}

	externalPolicy := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "externalPolicy",
			Namespace: serviceNamespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeNetworkPolicyExternalTraffic: serviceName,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev2alpha1.OtterizeServiceLabelKey: serviceName,
				},
			},
		},
	}

	firstList := s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), client.MatchingLabels{otterizev2alpha1.OtterizeNetworkPolicy: serviceName}, &client.ListOptions{Namespace: serviceNamespace},
	).DoAndReturn(
		func(_ any, list *v1.NetworkPolicyList, _ ...any) error {
			list.Items = []v1.NetworkPolicy{*toBeRemovedPolicy}
			return nil
		},
	)
	secondList := s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}),
		client.MatchingLabels{otterizev2alpha1.OtterizeNetworkPolicyExternalTraffic: serviceName},
		&client.ListOptions{Namespace: serviceNamespace},
	).DoAndReturn(
		func(_ any, list *v1.NetworkPolicyList, _ ...any) error {
			list.Items = []v1.NetworkPolicy{*externalPolicy}
			return nil
		},
	)
	gomock.InOrder(firstList, secondList)

	s.Client.EXPECT().Delete(gomock.Any(), externalPolicy, gomock.Any())
	err := s.handler.HandleBeforeAccessPolicyRemoval(context.Background(), toBeRemovedPolicy)
	s.Require().NoError(err)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleBeforeAccessPolicyRemoval_TwoRelatedPolicy_shouldNotDelete() {
	serviceName := "testservice"
	serviceNamespace := "testnamespace"
	toBeRemovedPolicy := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coolPolicy",
			Namespace: serviceNamespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeNetworkPolicy: serviceName,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev2alpha1.OtterizeServiceLabelKey: serviceName,
				},
			},
		},
	}

	otherPolicy := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coolPolicy",
			Namespace: serviceNamespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeNetworkPolicy: serviceName,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev2alpha1.OtterizeServiceLabelKey: serviceName,
				},
			},
		},
	}

	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), client.MatchingLabels{otterizev2alpha1.OtterizeNetworkPolicy: serviceName}, &client.ListOptions{Namespace: serviceNamespace},
	).DoAndReturn(
		func(_ any, list *v1.NetworkPolicyList, _ ...any) error {
			// two policies
			list.Items = []v1.NetworkPolicy{*toBeRemovedPolicy, *otherPolicy}
			return nil
		},
	)

	err := s.handler.HandleBeforeAccessPolicyRemoval(context.Background(), toBeRemovedPolicy)

	s.Require().NoError(err)
}

func TestNetworkPolicyHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(NetworkPolicyHandlerTestSuite))
}
