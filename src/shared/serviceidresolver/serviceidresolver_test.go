package serviceidresolver

import (
	"context"
	"github.com/golang/mock/gomock"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	serviceidresolvermocks "github.com/otterize/intents-operator/src/shared/serviceidresolver/mocks"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

type MatchingLabelsSelectorMatcher struct {
	expected client.MatchingLabelsSelector
}

func (m *MatchingLabelsSelectorMatcher) Matches(x interface{}) bool {
	if x == nil {
		return false
	}
	matchingLabels, ok := x.(client.MatchingLabelsSelector)
	if !ok {
		return false
	}
	return m.expected.String() == matchingLabels.String()
}

func (m *MatchingLabelsSelectorMatcher) String() string {
	return m.expected.String()
}

type ServiceIdResolverTestSuite struct {
	suite.Suite
	Client   *serviceidresolvermocks.MockClient
	Resolver *Resolver
}

func (s *ServiceIdResolverTestSuite) SetupTest() {
	controller := gomock.NewController(s.T())
	s.Client = serviceidresolvermocks.NewMockClient(controller)
	s.Resolver = NewResolver(s.Client)
}

func (s *ServiceIdResolverTestSuite) TestResolveClientIntentToServiceAccountName_PodExists() {
	serviceName := "coolservice"
	namespace := "coolnamespace"
	SAName := "backendservice"

	intent := v1alpha2.ClientIntents{Spec: &v1alpha2.IntentsSpec{Service: v1alpha2.Service{Name: serviceName}}, ObjectMeta: metav1.ObjectMeta{Namespace: namespace}}
	ls, err := intent.BuildPodLabelSelector()
	s.Require().NoError(err)

	pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace}, Spec: corev1.PodSpec{ServiceAccountName: SAName}}

	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&corev1.PodList{}),
		&MatchingLabelsSelectorMatcher{client.MatchingLabelsSelector{Selector: ls}},
	).Do(func(_ any, podList *corev1.PodList, _ ...any) {
		podList.Items = append(podList.Items, pod)
	})

	resultSAName, err := s.Resolver.ResolveClientIntentToServiceAccountName(context.Background(), intent)
	s.Require().NoError(err)
	s.Require().Equal(SAName, resultSAName)
}

func (s *ServiceIdResolverTestSuite) TestResolveClientIntentToServiceAccountName_PodDoesntExist() {
	serviceName := "coolservice"
	namespace := "coolnamespace"

	intent := v1alpha2.ClientIntents{Spec: &v1alpha2.IntentsSpec{Service: v1alpha2.Service{Name: serviceName}}, ObjectMeta: metav1.ObjectMeta{Namespace: namespace}}
	ls, err := intent.BuildPodLabelSelector()
	s.Require().NoError(err)

	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&corev1.PodList{}),
		&MatchingLabelsSelectorMatcher{client.MatchingLabelsSelector{Selector: ls}},
	).Do(func(_ any, podList *corev1.PodList, _ ...any) {})

	resultSAName, err := s.Resolver.ResolveClientIntentToServiceAccountName(context.Background(), intent)
	s.Require().Equal(err, PodNotFound)
	s.Require().Equal("", resultSAName)
}

func TestServiceIdResolverTestSuite(t *testing.T) {
	suite.Run(t, new(ServiceIdResolverTestSuite))
}
