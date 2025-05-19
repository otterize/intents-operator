package webhook_traffic

import (
	"context"
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/automate_third_party_network_policy"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

const (
	TestServiceName    = "test-service"
	TestServicePort    = 2345
	TestNamespace      = "test-namespace"
	TestWebhookName    = "test-webhook"
	TestServicePodName = "test-service-pod"
	TestControlPlaneIP = "111.222.333.4"
)

type NetworkPolicyHandlerTestSuite struct {
	testbase.MocksSuiteBase
	handler *NetworkPolicyHandler

	validatingWebhook *admissionv1.ValidatingWebhookConfiguration
	webhookService    *corev1.Service
	serviceEndpoints  *corev1.Endpoints
	servicePod        *corev1.Pod
}

func (s *NetworkPolicyHandlerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.handler = NewNetworkPolicyHandler(s.Client, &runtime.Scheme{}, automate_third_party_network_policy.IfBlockedByOtterize)
	s.handler.InjectRecorder(s.Recorder)

	s.validatingWebhook = &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: TestWebhookName,
		},
		Webhooks: []admissionv1.ValidatingWebhook{
			{
				ClientConfig: admissionv1.WebhookClientConfig{
					Service: &admissionv1.ServiceReference{
						Name:      TestServiceName,
						Namespace: TestNamespace,
						Port:      lo.ToPtr(int32(TestServicePort)),
					},
				},
			},
		},
	}

	s.webhookService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestServiceName,
			Namespace: TestNamespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"Taylor": "Swift",
			},
		},
	}

	s.serviceEndpoints = &corev1.Endpoints{
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Name:      TestServicePodName,
							Namespace: TestNamespace,
						},
					},
				},
			},
		},
	}

	s.servicePod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestServicePodName,
			Namespace: TestNamespace,
			Labels: map[string]string{
				v2alpha1.OtterizeServiceLabelKey: TestServiceName,
			},
		},
	}
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleIfBlockedByOtterize_ServiceIsBlockedByOtterize_CreatingWebhookPolicy() {
	s.mockForReturningValidatingWebhook()
	s.mockReturningWebhookService()
	s.mockServiceIsBlockedByOtterize()
	s.mockGetControlPlaneIPs()
	s.mockGetExistingOtterizeWebhooksNetpols([]v1.NetworkPolicy{})

	netpolMatcher := NewNetworkPolicyMatcher()
	s.Client.EXPECT().Create(gomock.Any(), gomock.All(netpolMatcher)).Return(nil)
	err := s.handler.ReconcileAllValidatingWebhooksWebhooks(context.Background())
	s.Require().NoError(err)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpol)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpolSuccess)
}

func (s *NetworkPolicyHandlerTestSuite) mockForReturningValidatingWebhook() {
	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&admissionv1.ValidatingWebhookConfigurationList{}),
	).DoAndReturn(
		func(_ any, list *admissionv1.ValidatingWebhookConfigurationList, _ ...any) error {
			list.Items = []admissionv1.ValidatingWebhookConfiguration{*s.validatingWebhook}
			return nil
		},
	)
}

func (s *NetworkPolicyHandlerTestSuite) mockReturningWebhookService() {
	s.Client.EXPECT().Get(
		gomock.Any(), gomock.Eq(types.NamespacedName{Name: TestServiceName, Namespace: TestNamespace}), gomock.Eq(&corev1.Service{}),
	).DoAndReturn(
		func(_ any, _ any, svc *corev1.Service, _ ...any) error {
			s.webhookService.DeepCopyInto(svc)
			return nil
		},
	)
}

func (s *NetworkPolicyHandlerTestSuite) mockServiceIsBlockedByOtterize() {
	// Get service endpoints
	s.Client.EXPECT().Get(
		gomock.Any(), gomock.Eq(types.NamespacedName{Name: TestServiceName, Namespace: TestNamespace}), gomock.Eq(&corev1.Endpoints{}),
	).DoAndReturn(
		func(_ any, _ any, endpoints *corev1.Endpoints, _ ...any) error {
			s.serviceEndpoints.DeepCopyInto(endpoints)
			return nil
		},
	)

	// Get endpoints pods
	s.Client.EXPECT().Get(
		gomock.Any(), gomock.Eq(types.NamespacedName{Name: TestServicePodName, Namespace: TestNamespace}), gomock.Eq(&corev1.Pod{}),
	).DoAndReturn(
		func(_ any, _ any, pod *corev1.Pod, _ ...any) error {
			s.servicePod.DeepCopyInto(pod)
			return nil
		},
	)

	// Pod has network policy
	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), gomock.Any(),
	).DoAndReturn(
		func(_ any, netpolList *v1.NetworkPolicyList, _ ...any) error {
			netpolList.Items = []v1.NetworkPolicy{
				{
					Spec: v1.NetworkPolicySpec{
						PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
					},
				},
			}
			return nil
		},
	)
	// Other 2 calls for network policies
	s.Client.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)
}

func (s *NetworkPolicyHandlerTestSuite) mockGetControlPlaneIPs() {
	s.Client.EXPECT().Get(
		gomock.Any(), gomock.Eq(types.NamespacedName{Name: "kubernetes", Namespace: "default"}), gomock.Eq(&corev1.Service{}),
	).DoAndReturn(
		func(_ any, _ any, svc *corev1.Service, _ ...any) error {
			svc.Spec.ClusterIP = TestControlPlaneIP
			return nil
		},
	)
}

func (s *NetworkPolicyHandlerTestSuite) mockGetExistingOtterizeWebhooksNetpols(netpols []v1.NetworkPolicy) {
	otterizeWebhookNetpolSelector, err := metav1.LabelSelectorAsSelector(
		&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      v2alpha1.OtterizeNetworkPolicyWebhooks,
				Operator: metav1.LabelSelectorOpExists,
			},
		},
		})
	s.Require().NoError(err)

	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), gomock.Eq(client.MatchingLabelsSelector{Selector: otterizeWebhookNetpolSelector}),
	).DoAndReturn(
		func(_ any, netpolList *v1.NetworkPolicyList, _ ...any) error {
			netpolList.Items = netpols
			return nil
		},
	)
}

func TestNetworkPolicyHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(NetworkPolicyHandlerTestSuite))
}
