package serviceidresolver

import (
	"context"
	"errors"
	"github.com/golang/mock/gomock"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	serviceidresolvermocks "github.com/otterize/intents-operator/src/shared/serviceidresolver/mocks"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

func (s *ServiceIdResolverTestSuite) TestResolveClientIntentToPod_PodExists() {
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

	resolvedPod, err := s.Resolver.ResolveClientIntentToPod(context.Background(), intent)
	resultSAName := resolvedPod.Spec.ServiceAccountName
	s.Require().NoError(err)
	s.Require().Equal(SAName, resultSAName)
}

func (s *ServiceIdResolverTestSuite) TestResolveClientIntentToPod_PodDoesntExist() {
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

	pod, err := s.Resolver.ResolveClientIntentToPod(context.Background(), intent)
	s.Require().Equal(err, PodNotFound)
	s.Require().Equal(corev1.Pod{}, pod)
}

func (s *ServiceIdResolverTestSuite) TestGetPodAnnotatedName_PodExists() {
	podName := "coolpod"
	podNamespace := "coolnamespace"
	serviceName := "coolservice"

	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: podName, Namespace: podNamespace}, gomock.AssignableToTypeOf(&corev1.Pod{})).Do(
		func(_ context.Context, _ types.NamespacedName, pod *corev1.Pod, _ ...any) {
			pod.Annotations = map[string]string{ServiceNameAnnotation: serviceName}
		}).Return(nil)

	name, found, err := s.Resolver.GetPodAnnotatedName(context.Background(), podName, podNamespace)
	s.Require().NoError(err)
	s.Require().True(found)
	s.Require().Equal(serviceName, name)
}

func (s *ServiceIdResolverTestSuite) TestGetPodAnnotatedName_PodMissingAnnotation() {
	podName := "coolpod"
	podNamespace := "coolnamespace"

	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: podName, Namespace: podNamespace}, gomock.AssignableToTypeOf(&corev1.Pod{})).Return(nil)

	name, found, err := s.Resolver.GetPodAnnotatedName(context.Background(), podName, podNamespace)
	s.Require().NoError(err)
	s.Require().False(found)
	s.Require().Equal("", name)
}

func (s *ServiceIdResolverTestSuite) TestGetPodAnnotatedName_PodMCallFailed() {
	podName := "coolpod"
	podNamespace := "coolnamespace"

	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: podName, Namespace: podNamespace}, gomock.AssignableToTypeOf(&corev1.Pod{})).Return(errors.New("generic error"))

	name, found, err := s.Resolver.GetPodAnnotatedName(context.Background(), podName, podNamespace)
	s.Require().Error(err)
	s.Require().False(found)
	s.Require().Equal("", name)
}

func TestServiceIdResolverTestSuite(t *testing.T) {
	suite.Run(t, new(ServiceIdResolverTestSuite))
}
