package serviceidresolver

import (
	"context"
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	serviceidresolvermocks "github.com/otterize/intents-operator/src/shared/serviceidresolver/mocks"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

type MatchingLabelsMatcher struct {
	expected client.MatchingLabels
}

func (m *MatchingLabelsMatcher) Matches(x interface{}) bool {
	if x == nil {
		return false
	}
	matchingLabels, ok := x.(client.MatchingLabels)
	if !ok {
		return false
	}
	a, b := lo.Difference(lo.Keys(m.expected), lo.Keys(matchingLabels))
	c, d := lo.Difference(lo.Values(m.expected), lo.Values(matchingLabels))
	return len(a) == 0 && len(b) == 0 && len(c) == 0 && len(d) == 0
}

func (m *MatchingLabelsMatcher) String() string {
	return fmt.Sprintf("%s", m.expected)
}

type ServiceIdResolverTestSuite struct {
	suite.Suite
	Client   *serviceidresolvermocks.Mockk8sClient
	Resolver *Resolver
}

func (s *ServiceIdResolverTestSuite) SetupTest() {
	controller := gomock.NewController(s.T())
	s.Client = serviceidresolvermocks.NewMockk8sClient(controller)
	s.Resolver = NewResolver(s.Client)
}

func (s *ServiceIdResolverTestSuite) TestOtterizeServiceToServiceAccountName_PodExists() {
	otterizeServiceName := "coolservice"
	namespace := "coolnamespace"
	SAName := "backendservice"

	pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: otterizeServiceName, Namespace: namespace}, Spec: corev1.PodSpec{ServiceAccountName: SAName}}

	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&corev1.PodList{}),
		&MatchingLabelsMatcher{map[string]string{v1alpha2.OtterizeServerLabelKey: otterizeServiceName}},
		gomock.Eq(client.InNamespace(namespace))).Do(func(_ any, podList *corev1.PodList, _ ...any) {
		podList.Items = append(podList.Items, pod)
	})

	resultSAName, err := s.Resolver.ResolveOtterizeServiceNameToServiceAccountName(context.Background(), otterizeServiceName, namespace)
	s.Require().NoError(err)
	s.Require().Equal(SAName, resultSAName)

}

func (s *ServiceIdResolverTestSuite) TestOtterizeServiceToServiceAccountName_PodDoesntExist() {
	otterizeServiceName := "coolservice"
	namespace := "coolnamespace"

	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&corev1.PodList{}),
		&MatchingLabelsMatcher{map[string]string{v1alpha2.OtterizeServerLabelKey: otterizeServiceName}},
		gomock.Eq(client.InNamespace(namespace))).Do(func(_ any, podList *corev1.PodList, _ ...any) {})

	resultSAName, err := s.Resolver.ResolveOtterizeServiceNameToServiceAccountName(context.Background(), otterizeServiceName, namespace)
	s.Require().Equal(err, ServiceAccountNotFond)
	s.Require().Equal("", resultSAName)

}

func TestServiceIdResolverTestSuite(t *testing.T) {
	suite.Run(t, new(ServiceIdResolverTestSuite))
}
