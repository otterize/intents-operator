package metrics_collection_traffic

import (
	"context"
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/allowexternaltraffic"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

const TEST_NAMESPACE = "test-namespace"

var EXPECTRED_NETPOL = v1.NetworkPolicy{
	ObjectMeta: metav1.ObjectMeta{
		Namespace:   TEST_NAMESPACE,
		Name:        "metrics-collection-access-to-pod-test-server-5dfcbcbf9",
		Labels:      map[string]string{v2alpha1.OtterizeNetPolMetricsCollectors: "test-server", v2alpha1.OtterizeNetPolMetricsCollectorsLevel: "pod"},
		Annotations: map[string]string{v2alpha1.OtterizeCreatedForServiceAnnotation: "test-server-5dfcbcbf-test-namespace-c6140c"},
	},
	Spec: v1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{
			MatchLabels: map[string]string{v2alpha1.OtterizeOwnerKindLabelKey: "Deployment", v2alpha1.OtterizeServiceLabelKey: "test-server-5dfcbcbf-test-namespace-c6140c"},
		},
		Ingress: []v1.NetworkPolicyIngressRule{
			{
				Ports: []v1.NetworkPolicyPort{{}},
			},
		},
		PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
	},
}

type NetworkPolicyMatcher struct {
	expectedPorts []int32
}

func NewNetworkPolicyMatcher(expectedPorts []int32) *NetworkPolicyMatcher {
	return &NetworkPolicyMatcher{expectedPorts: expectedPorts}
}

func (m *NetworkPolicyMatcher) Matches(other interface{}) bool {
	otherAsNetpol, ok := other.(*v1.NetworkPolicy)
	if !ok {
		return false
	}

	expectedNetpol := EXPECTRED_NETPOL

	expectedNetpol.Spec.Ingress[0].Ports = make([]v1.NetworkPolicyPort, len(m.expectedPorts))
	for i, _ := range m.expectedPorts {
		expectedNetpol.Spec.Ingress[0].Ports[i].Port = lo.ToPtr(intstr.IntOrString{Type: intstr.Int, IntVal: m.expectedPorts[i]})
		expectedNetpol.Spec.Ingress[0].Ports[i].Protocol = lo.ToPtr(corev1.ProtocolTCP)
	}

	return otherAsNetpol.Namespace == TEST_NAMESPACE &&
		otherAsNetpol.Name == expectedNetpol.Name &&
		reflect.DeepEqual(otherAsNetpol.Labels, expectedNetpol.Labels) &&
		reflect.DeepEqual(otherAsNetpol.Annotations, expectedNetpol.Annotations) &&
		reflect.DeepEqual(otherAsNetpol.Spec, expectedNetpol.Spec)
}

func (m *NetworkPolicyMatcher) String() string {
	return "matches network policy"
}

type NetworkPolicyHandlerTestSuite struct {
	testbase.MocksSuiteBase
	handler              *NetworkPolicyHandler
	podMarkedForScraping *corev1.Pod
}

func TestNetworkPolicyHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(NetworkPolicyHandlerTestSuite))
}

func (s *NetworkPolicyHandlerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.handler = NewNetworkPolicyHandler(s.Client, &runtime.Scheme{}, allowexternaltraffic.IfBlockedByOtterize)
	s.handler.InjectRecorder(s.Recorder)

	s.podMarkedForScraping = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      map[string]string{v2alpha1.OtterizeServiceLabelKey: "test-server"},
			Annotations: map[string]string{"prometheus.io/scrape": "true", "prometheus.io/port": "9090"},
			Namespace:   TEST_NAMESPACE,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "test-server-5dfcbcbf9",
				},
			},
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
	}
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleIfBlockedByOtterize_NoOtherOtterizePolicies_ShouldDoNothing() {
	s.mockForReturningScrapePodInListNamespace()
	s.mockForResolvingScrapingPodIdentity()
	s.mockNoOtherOtterizeNetworkPolicies()
	s.mockNoExistingMetricCollectionNetworkPolicies()

	err := s.handler.HandleAllPodsInNamespace(context.Background(), TEST_NAMESPACE)
	s.Require().NoError(err)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleIfBlockedByOtterize_ExistingOtterizePolicies_ShouldAddPolicy() {
	s.mockForReturningScrapePodInListNamespace()
	s.mockForResolvingScrapingPodIdentity()
	s.mockOneExistingOtterizeNetworkPolicies()
	s.mockNoExistingMetricCollectionNetworkPolicies()
	s.mockForRecordingEventExistingPolicy()

	netpolMatcher := NewNetworkPolicyMatcher([]int32{9090})
	s.Client.EXPECT().Create(gomock.Any(), gomock.All(netpolMatcher)).Return(nil)
	err := s.handler.HandleAllPodsInNamespace(context.Background(), TEST_NAMESPACE)
	s.Require().NoError(err)
	s.ExpectEvent(ReasonCreatingMetricsCollectorPolicy)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleIfBlockedByOtterize_PodNoLongerMarkedForScrape_ShouldRemovePolicy() {
	s.podMarkedForScraping.Annotations["prometheus.io/scrape"] = "false"
	s.mockForReturningScrapePodInListNamespace()
	//s.mockForResolvingScrapingPodIdentity() // would not reach here since pod is not mark for scrape
	//s.mockOneExistingOtterizeNetworkPolicies() // would not reach here since pod is not mark for scrape
	s.mockOneExistingMetricCollectionNetworkPolicies()
	s.mockForRecordingEventExistingPolicy()

	s.Client.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(nil)
	err := s.handler.HandleAllPodsInNamespace(context.Background(), TEST_NAMESPACE)
	s.Require().NoError(err)
	s.ExpectEvent(ReasonRemovingMetricsCollectorPolicy)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleIfBlockedByOtterize_ExistingOtterizePolicies_ShouldUpdatePolicy() {
	s.mockForReturningScrapePodInListNamespace()
	s.mockForResolvingScrapingPodIdentity()
	s.mockOneExistingOtterizeNetworkPolicies()
	s.mockOneExistingMetricCollectionNetworkPolicies()
	s.mockForGettingExistingPolicyDuringUpdate()
	s.mockForRecordingEventExistingPolicy()

	netpolMatcher := NewNetworkPolicyMatcher([]int32{9090})
	s.Client.EXPECT().Patch(gomock.Any(), gomock.All(netpolMatcher), gomock.Any()).Return(nil)
	err := s.handler.HandleAllPodsInNamespace(context.Background(), TEST_NAMESPACE)
	s.Require().NoError(err)
	s.ExpectEvent(ReasonUpdatingMetricsCollectorPolicy)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleAlways_ShouldAddPolicy() {
	s.setHandler(allowexternaltraffic.Always)
	s.mockForReturningScrapePodInListNamespace()
	s.mockForResolvingScrapingPodIdentity()
	//s.mockOneExistingOtterizeNetworkPolicies() // would not reach here since configuration is always
	s.mockNoExistingMetricCollectionNetworkPolicies()
	s.mockForRecordingEventExistingPolicy()

	netpolMatcher := NewNetworkPolicyMatcher([]int32{9090})
	s.Client.EXPECT().Create(gomock.Any(), gomock.All(netpolMatcher)).Return(nil)
	err := s.handler.HandleAllPodsInNamespace(context.Background(), TEST_NAMESPACE)
	s.Require().NoError(err)
	s.ExpectEvent(ReasonCreatingMetricsCollectorPolicy)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleOff_ShouldDoNothing() {
	s.setHandler(allowexternaltraffic.Off)
	s.mockForReturningScrapePodInListNamespace()
	s.mockForResolvingScrapingPodIdentity()
	//s.mockOneExistingOtterizeNetworkPolicies() // would not reach here since configuration is never
	s.mockNoExistingMetricCollectionNetworkPolicies()
	//s.mockForRecordingEventExistingPolicy() // would not reach here - not creating netpol

	//netpolMatcher := NewNetworkPolicyMatcher() // would not reach here - not creating netpol
	//s.Client.EXPECT().Create(gomock.Any(), gomock.All(netpolMatcher)).Return(nil) // would not reach here - not creating netpol
	err := s.handler.HandleAllPodsInNamespace(context.Background(), TEST_NAMESPACE)
	s.Require().NoError(err)
	//s.ExpectEvent(ReasonCreatingMetricsCollectorPolicy) // would not reach here - not creating netpol
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleAlways_PortNotDefinedWithAnnotation_ShouldAddPolicy_UseAllResourcesPort() {
	s.setHandler(allowexternaltraffic.Always)
	s.podMarkedForScraping.Annotations["prometheus.io/port"] = ""
	s.podMarkedForScraping.Spec.Containers = []corev1.Container{
		{
			Ports: []corev1.ContainerPort{
				{
					ContainerPort: 4321,
				},
				{
					ContainerPort: 1234,
				},
			},
		},
	}
	s.mockForReturningScrapePodInListNamespace()
	s.mockForResolvingScrapingPodIdentity()
	//s.mockOneExistingOtterizeNetworkPolicies() // would not reach here since configuration is always
	s.mockNoExistingMetricCollectionNetworkPolicies()
	s.mockForRecordingEventExistingPolicy()

	netpolMatcher := NewNetworkPolicyMatcher([]int32{1234, 4321})
	s.Client.EXPECT().Create(gomock.Any(), gomock.All(netpolMatcher)).Return(nil)
	err := s.handler.HandleAllPodsInNamespace(context.Background(), TEST_NAMESPACE)
	s.Require().NoError(err)
	s.ExpectEvent(ReasonCreatingMetricsCollectorPolicy)
}

func (s *NetworkPolicyHandlerTestSuite) setHandler(allowMetricsCollector allowexternaltraffic.Enum) {
	s.handler = NewNetworkPolicyHandler(s.Client, &runtime.Scheme{}, allowMetricsCollector)
	s.handler.InjectRecorder(s.Recorder)
}

func (s *NetworkPolicyHandlerTestSuite) mockForReturningScrapePodInListNamespace() {
	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&corev1.PodList{}), client.InNamespace(TEST_NAMESPACE), gomock.Any(),
	).DoAndReturn(
		func(_ any, list *corev1.PodList, _ ...any) error {
			list.Items = []corev1.Pod{*s.podMarkedForScraping}
			return nil
		},
	)
}

func (s *NetworkPolicyHandlerTestSuite) mockForResolvingScrapingPodIdentity() {
	s.Client.EXPECT().
		Get(gomock.Any(), types.NamespacedName{Name: s.podMarkedForScraping.OwnerReferences[0].Name, Namespace: TEST_NAMESPACE}, gomock.Any()).
		DoAndReturn(func(_ any, _ any, obj metav1.Object, _ ...any) error {
			obj.SetName(s.podMarkedForScraping.OwnerReferences[0].Name)
			return nil
		})
}

func (s *NetworkPolicyHandlerTestSuite) mockNoOtherOtterizeNetworkPolicies() {
	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), gomock.Any(),
	).Return(nil)
	s.Client.EXPECT().List(
		gomock.Any(), gomock.AssignableToTypeOf(&v1.NetworkPolicyList{}), gomock.Any(),
	).Return(nil)

	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&corev1.EndpointsList{}), gomock.Any(), gomock.Any(),
	).Return(nil) // No endpoints -> we will not query netpols again for service
}

func (s *NetworkPolicyHandlerTestSuite) mockOneExistingOtterizeNetworkPolicies() {
	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), gomock.Any(),
	).DoAndReturn(func(_ any, list *v1.NetworkPolicyList, _ ...any) error {
		list.Items = []v1.NetworkPolicy{{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "otterize-netpol",
				Namespace: TEST_NAMESPACE,
			},
			Spec: v1.NetworkPolicySpec{
				PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			},
		}}
		return nil
	})
	s.Client.EXPECT().List(
		gomock.Any(), gomock.AssignableToTypeOf(&v1.NetworkPolicyList{}), gomock.Any(),
	).Return(nil)
	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&corev1.EndpointsList{}), gomock.Any(), gomock.Any(),
	).Return(nil) // No endpoints -> we will not query netpols again for service
}

func (s *NetworkPolicyHandlerTestSuite) mockNoExistingMetricCollectionNetworkPolicies() {
	s.Client.EXPECT().List(gomock.Any(),
		gomock.Eq(&v1.NetworkPolicyList{}),
		client.InNamespace(TEST_NAMESPACE),
		client.MatchingLabels{v2alpha1.OtterizeNetPolMetricsCollectorsLevel: "pod"},
	).Return(nil)
}

func (s *NetworkPolicyHandlerTestSuite) mockOneExistingMetricCollectionNetworkPolicies() {
	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.Eq(&v1.NetworkPolicyList{}), client.InNamespace(TEST_NAMESPACE),
		client.MatchingLabels{v2alpha1.OtterizeNetPolMetricsCollectorsLevel: "pod"},
	).DoAndReturn(func(_ any, list *v1.NetworkPolicyList, _ ...any) error {
		list.Items = []v1.NetworkPolicy{{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "metrics-collection-access-to-pod-test-server-5dfcbcbf9",
				Namespace: TEST_NAMESPACE,
				Labels:    map[string]string{v2alpha1.OtterizeNetPolMetricsCollectorsLevel: "pod"},
			},
		}}
		return nil
	})
}

func (s *NetworkPolicyHandlerTestSuite) mockForGettingExistingPolicyDuringUpdate() {
	s.Client.EXPECT().
		Get(gomock.Any(),
			types.NamespacedName{Name: "metrics-collection-access-to-pod-test-server-5dfcbcbf9", Namespace: TEST_NAMESPACE},
			gomock.Eq(&v1.NetworkPolicy{}),
		).DoAndReturn(func(_ any, _ any, netpol *v1.NetworkPolicy, _ ...any) error {
		netpol.ObjectMeta = *EXPECTRED_NETPOL.ObjectMeta.DeepCopy()
		netpol.Spec = *EXPECTRED_NETPOL.Spec.DeepCopy()
		netpol.Spec.Ingress[0].Ports[0].Port = lo.ToPtr(intstr.IntOrString{Type: intstr.Int, IntVal: 10})
		return nil
	})
}

func (s *NetworkPolicyHandlerTestSuite) mockForRecordingEventExistingPolicy() {
	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&corev1.PodList{}), client.InNamespace(TEST_NAMESPACE), gomock.Any(),
	).DoAndReturn(
		func(_ any, list *corev1.PodList, _ ...any) error {
			list.Items = []corev1.Pod{*s.podMarkedForScraping}
			return nil
		},
	)
}
