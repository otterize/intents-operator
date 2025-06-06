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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

const (
	TestServiceName    = "test-service"
	TestServicePort    = 2345
	TestNamespace      = "test-namespace"
	TestWebhookName    = "test-webhook"
	TestServicePodName = "test-service-pod"
	TestControlPlaneIP = "11.22.33.4"
)

var OtterizeIngressNetpols = []v1.NetworkPolicy{
	{
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
		},
	},
}

var ValidatingWebhookConfiguration = admissionv1.ValidatingWebhookConfiguration{
	ObjectMeta: metav1.ObjectMeta{
		Name: TestWebhookName,
	},
	Webhooks: []admissionv1.ValidatingWebhook{
		{
			Name: "First",
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

var MutatingWebhookConfiguration = admissionv1.MutatingWebhookConfiguration{
	ObjectMeta: metav1.ObjectMeta{
		Name: TestWebhookName,
	},
	Webhooks: []admissionv1.MutatingWebhook{
		{
			Name: "First",
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

var CRDWebhookConfiguration = apiextensionsv1.CustomResourceDefinition{
	ObjectMeta: metav1.ObjectMeta{
		Name: TestWebhookName,
	},
	Spec: apiextensionsv1.CustomResourceDefinitionSpec{
		Conversion: &apiextensionsv1.CustomResourceConversion{
			Webhook: &apiextensionsv1.WebhookConversion{
				ClientConfig: &apiextensionsv1.WebhookClientConfig{
					Service: &apiextensionsv1.ServiceReference{
						Name:      TestServiceName,
						Namespace: TestNamespace,
						Port:      lo.ToPtr(int32(TestServicePort)),
					},
				},
			},
		},
	},
}

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
	s.handler = NewNetworkPolicyHandler(s.Client, &runtime.Scheme{}, automate_third_party_network_policy.IfBlockedByOtterize, 32, false)
	s.handler.InjectRecorder(s.Recorder)

	s.validatingWebhook = ValidatingWebhookConfiguration.DeepCopy()

	s.webhookService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestServiceName,
			Namespace: TestNamespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"Taylor": "Swift",
			},
			Ports: []corev1.ServicePort{
				{
					Port:     TestServicePort,
					Protocol: corev1.ProtocolTCP,
				},
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
	s.mockServiceIsBlockedByOtterize(OtterizeIngressNetpols)
	s.mockGetControlPlaneIPs()
	s.mockGetExistingOtterizeWebhooksNetpols([]v1.NetworkPolicy{})

	netpolMatcher := NewNetworkPolicyMatcher([]int32{TestServicePort}, false, nil)
	s.Client.EXPECT().Create(gomock.Any(), gomock.All(netpolMatcher)).Return(nil)
	err := s.handler.HandleAll(context.Background())
	s.Require().NoError(err)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpol)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpolSuccess)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleIfBlockedByOtterize_ServiceIsBlockedByOtterize_TwoWebhooksToSameService_CreatingOneWebhookPolicy() {
	s.validatingWebhook.Webhooks = append(s.validatingWebhook.Webhooks,
		admissionv1.ValidatingWebhook{
			Name: "Second",
			ClientConfig: admissionv1.WebhookClientConfig{
				Service: &admissionv1.ServiceReference{
					Name:      TestServiceName,
					Namespace: TestNamespace,
					Port:      lo.ToPtr(int32(TestServicePort)),
				},
			},
		})

	s.mockForReturningValidatingWebhook()

	// Called once for "First" webhook
	s.mockReturningWebhookService()
	s.mockServiceIsBlockedByOtterize(OtterizeIngressNetpols)
	s.mockGetControlPlaneIPs()

	// Called second time for "Second"" webhook
	s.mockReturningWebhookService()
	s.mockServiceIsBlockedByOtterize(OtterizeIngressNetpols)
	s.mockGetControlPlaneIPs()

	s.mockGetExistingOtterizeWebhooksNetpols([]v1.NetworkPolicy{})

	netpolMatcher := NewNetworkPolicyMatcher([]int32{TestServicePort}, s.handler.allowAllIncomingTraffic, nil)
	s.Client.EXPECT().Create(gomock.Any(), gomock.All(netpolMatcher)).Return(nil)
	err := s.handler.HandleAll(context.Background())
	s.Require().NoError(err)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpol)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpolSuccess)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleIfBlockedByOtterize_ServiceIsBlockedByOtterize_PolicyAlreadyExist_DoNothing() {
	s.mockForReturningValidatingWebhook()
	s.mockReturningWebhookService()
	s.mockServiceIsBlockedByOtterize(OtterizeIngressNetpols)
	s.mockGetControlPlaneIPs()
	s.mockGetExistingOtterizeWebhooksNetpols([]v1.NetworkPolicy{
		NewNetworkPolicyBuilder(ExpectedNetpol).
			WithPorts([]int32{TestServicePort}).
			WithFromIPBlock(s.handler.allowAllIncomingTraffic).
			Build()})
	s.mockGetNetworkPolicyForUpdate(NewNetworkPolicyBuilder(ExpectedNetpol).
		WithPorts([]int32{TestServicePort}).
		WithFromIPBlock(s.handler.allowAllIncomingTraffic).
		Build())

	//netpolMatcher := NewNetworkPolicyMatcher([]int32{TestServicePort})
	//s.Client.EXPECT().Create(gomock.Any(), gomock.All(netpolMatcher)).Return(nil)
	err := s.handler.HandleAll(context.Background())
	s.Require().NoError(err)
	//s.ExpectEvent(ReasonCreatingWebhookTrafficNetpol)
	//s.ExpectEvent(ReasonCreatingWebhookTrafficNetpolSuccess)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleIfBlockedByOtterize_UpdatePolicy() {
	s.mockForReturningValidatingWebhook()
	s.mockReturningWebhookService()
	s.mockServiceIsBlockedByOtterize(OtterizeIngressNetpols)
	s.mockGetControlPlaneIPs()
	s.mockGetExistingOtterizeWebhooksNetpols([]v1.NetworkPolicy{
		NewNetworkPolicyBuilder(ExpectedNetpol).
			WithPorts([]int32{12129}).
			WithFromIPBlock(s.handler.allowAllIncomingTraffic).
			Build()})
	s.mockGetNetworkPolicyForUpdate(
		NewNetworkPolicyBuilder(ExpectedNetpol).
			WithPorts([]int32{12129}).
			WithFromIPBlock(s.handler.allowAllIncomingTraffic).
			Build())

	netpolMatcher := NewNetworkPolicyMatcher([]int32{TestServicePort}, s.handler.allowAllIncomingTraffic, nil)
	s.Client.EXPECT().Patch(gomock.Any(), gomock.All(netpolMatcher), gomock.Any()).Return(nil)
	err := s.handler.HandleAll(context.Background())
	s.Require().NoError(err)
	s.ExpectEvent(ReasonPatchingWebhookTrafficNetpol)
	s.ExpectEvent(ReasonPatchingWebhookTrafficNetpolSuccess)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleIfBlockedByOtterize_ServiceIsNotBlockedByOtterize_DoNothing() {
	s.mockForReturningValidatingWebhook()
	s.mockReturningWebhookService()
	s.mockServiceIsBlockedByOtterize(make([]v1.NetworkPolicy, 0))
	//s.mockGetControlPlaneIPs()
	s.mockGetExistingOtterizeWebhooksNetpols([]v1.NetworkPolicy{})

	//netpolMatcher := NewNetworkPolicyMatcher([]int32{TestServicePort})
	//s.Client.EXPECT().Create(gomock.Any(), gomock.All(netpolMatcher)).Return(nil)
	err := s.handler.HandleAll(context.Background())
	s.Require().NoError(err)
	//s.ExpectEvent(ReasonCreatingWebhookTrafficNetpol)
	//s.ExpectEvent(ReasonCreatingWebhookTrafficNetpolSuccess)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleOff_ServiceIsBlockedByOtterize_DoNothing() {
	s.handler = NewNetworkPolicyHandler(s.Client, &runtime.Scheme{}, automate_third_party_network_policy.Off, 32, false)

	s.mockForReturningValidatingWebhook()
	//s.mockReturningWebhookService()
	//s.mockServiceIsBlockedByOtterize(OtterizeIngressNetpols)
	//s.mockGetControlPlaneIPs()
	s.mockGetExistingOtterizeWebhooksNetpols([]v1.NetworkPolicy{})

	//netpolMatcher := NewNetworkPolicyMatcher([]int32{TestServicePort})
	//s.Client.EXPECT().Create(gomock.Any(), gomock.All(netpolMatcher)).Return(nil)
	err := s.handler.HandleAll(context.Background())
	s.Require().NoError(err)
	//s.ExpectEvent(ReasonCreatingWebhookTrafficNetpol)
	//s.ExpectEvent(ReasonCreatingWebhookTrafficNetpolSuccess)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleOff_ServiceIsBlockedByOtterize_ExistingWebhookPolicy_DeletePolicy() {
	s.handler = NewNetworkPolicyHandler(s.Client, &runtime.Scheme{}, automate_third_party_network_policy.Off, 32, false)

	s.mockForReturningValidatingWebhook()
	//s.mockReturningWebhookService()
	//s.mockServiceIsBlockedByOtterize(OtterizeIngressNetpols)
	//s.mockGetControlPlaneIPs()
	s.mockGetExistingOtterizeWebhooksNetpols([]v1.NetworkPolicy{
		NewNetworkPolicyBuilder(ExpectedNetpol).
			WithPorts([]int32{TestServicePort}).
			WithFromIPBlock(s.handler.allowAllIncomingTraffic).
			Build()})

	netpolMatcher := NewNetworkPolicyMatcher([]int32{TestServicePort}, s.handler.allowAllIncomingTraffic, nil)
	s.Client.EXPECT().Delete(gomock.Any(), gomock.All(netpolMatcher)).Return(nil)
	err := s.handler.HandleAll(context.Background())
	s.Require().NoError(err)
	//s.ExpectEvent(ReasonCreatingWebhookTrafficNetpol)
	//s.ExpectEvent(ReasonCreatingWebhookTrafficNetpolSuccess)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleAlways_ServiceIsNotBlockedByOtterize_CreatePolicy() {
	s.handler = NewNetworkPolicyHandler(s.Client, &runtime.Scheme{}, automate_third_party_network_policy.Always, 32, false)
	s.handler.InjectRecorder(s.Recorder)

	s.mockForReturningValidatingWebhook()
	s.mockReturningWebhookService()
	//s.mockServiceIsBlockedByOtterize(make([]v1.NetworkPolicy, 0))
	s.mockGetControlPlaneIPs()
	s.mockGetExistingOtterizeWebhooksNetpols([]v1.NetworkPolicy{})

	netpolMatcher := NewNetworkPolicyMatcher([]int32{TestServicePort}, s.handler.allowAllIncomingTraffic, nil)
	s.Client.EXPECT().Create(gomock.Any(), gomock.All(netpolMatcher)).Return(nil)
	err := s.handler.HandleAll(context.Background())
	s.Require().NoError(err)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpol)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpolSuccess)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_MutatingWebhooks_HandleIfBlockedByOtterize_ServiceIsBlockedByOtterize_CreatingWebhookPolicy() {
	s.mockForReturningMutatingWebhook()
	s.mockReturningWebhookService()
	s.mockServiceIsBlockedByOtterize(OtterizeIngressNetpols)
	s.mockGetControlPlaneIPs()
	s.mockGetExistingOtterizeWebhooksNetpols([]v1.NetworkPolicy{})

	netpolMatcher := NewNetworkPolicyMatcher([]int32{TestServicePort}, s.handler.allowAllIncomingTraffic, nil)
	s.Client.EXPECT().Create(gomock.Any(), gomock.All(netpolMatcher)).Return(nil)
	err := s.handler.HandleAll(context.Background())
	s.Require().NoError(err)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpol)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpolSuccess)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_CRDsWebhooks_HandleIfBlockedByOtterize_ServiceIsBlockedByOtterize_CreatingWebhookPolicy() {
	s.mockForReturningCRDsWebhook()
	s.mockReturningWebhookService()
	s.mockServiceIsBlockedByOtterize(OtterizeIngressNetpols)
	s.mockGetControlPlaneIPs()
	s.mockGetExistingOtterizeWebhooksNetpols([]v1.NetworkPolicy{})

	netpolMatcher := NewNetworkPolicyMatcher([]int32{TestServicePort}, s.handler.allowAllIncomingTraffic, nil)
	s.Client.EXPECT().Create(gomock.Any(), gomock.All(netpolMatcher)).Return(nil)
	err := s.handler.HandleAll(context.Background())
	s.Require().NoError(err)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpol)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpolSuccess)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleAlways_AllowAllIncomingTraffic_CreatingWebhookPolicy() {
	s.handler = NewNetworkPolicyHandler(s.Client, &runtime.Scheme{}, automate_third_party_network_policy.Always, 32, true)
	s.handler.InjectRecorder(s.Recorder)

	s.mockForReturningValidatingWebhook()
	s.mockReturningWebhookService()
	//s.mockServiceIsBlockedByOtterize(OtterizeIngressNetpols)
	//s.mockGetControlPlaneIPs()
	s.mockGetExistingOtterizeWebhooksNetpols([]v1.NetworkPolicy{})

	netpolMatcher := NewNetworkPolicyMatcher([]int32{TestServicePort}, s.handler.allowAllIncomingTraffic, nil)
	s.Client.EXPECT().Create(gomock.Any(), gomock.All(netpolMatcher)).Return(nil)
	err := s.handler.HandleAll(context.Background())
	s.Require().NoError(err)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpol)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpolSuccess)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleAlways_ServiceHasDifferentTargetPortThanPort_CreatePolicy() {
	s.handler = NewNetworkPolicyHandler(s.Client, &runtime.Scheme{}, automate_third_party_network_policy.Always, 32, false)
	s.handler.InjectRecorder(s.Recorder)
	targetPort := int32(1820)
	s.webhookService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestServiceName,
			Namespace: TestNamespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"Taylor": "Swift",
			},
			Ports: []corev1.ServicePort{
				{
					Port:       TestServicePort,
					TargetPort: intstr.FromInt32(targetPort),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	s.mockForReturningValidatingWebhook()
	s.mockReturningWebhookService()
	//s.mockServiceIsBlockedByOtterize(make([]v1.NetworkPolicy, 0))
	s.mockGetControlPlaneIPs()
	s.mockGetExistingOtterizeWebhooksNetpols([]v1.NetworkPolicy{})

	netpolMatcher := NewNetworkPolicyMatcher([]int32{targetPort, TestServicePort}, s.handler.allowAllIncomingTraffic, nil)
	s.Client.EXPECT().Create(gomock.Any(), gomock.All(netpolMatcher)).Return(nil)
	err := s.handler.HandleAll(context.Background())
	s.Require().NoError(err)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpol)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpolSuccess)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleAlways_ServiceHasNoTargetPortThanPort_CreatePolicy() {
	s.handler = NewNetworkPolicyHandler(s.Client, &runtime.Scheme{}, automate_third_party_network_policy.Always, 32, false)
	s.handler.InjectRecorder(s.Recorder)

	s.webhookService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestServiceName,
			Namespace: TestNamespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"Taylor": "Swift",
			},
			Ports: []corev1.ServicePort{
				{
					Port:     TestServicePort,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}

	s.mockForReturningValidatingWebhook()
	s.mockReturningWebhookService()
	//s.mockServiceIsBlockedByOtterize(make([]v1.NetworkPolicy, 0))
	s.mockGetControlPlaneIPs()
	s.mockGetExistingOtterizeWebhooksNetpols([]v1.NetworkPolicy{})

	netpolMatcher := NewNetworkPolicyMatcher([]int32{TestServicePort}, s.handler.allowAllIncomingTraffic, nil)
	s.Client.EXPECT().Create(gomock.Any(), gomock.All(netpolMatcher)).Return(nil)
	err := s.handler.HandleAll(context.Background())
	s.Require().NoError(err)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpol)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpolSuccess)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleAlways_WebhookDoesNotDefinePort_CreatePolicyWithDefaultPort() {
	s.handler = NewNetworkPolicyHandler(s.Client, &runtime.Scheme{}, automate_third_party_network_policy.Always, 32, false)
	s.handler.InjectRecorder(s.Recorder)

	s.validatingWebhook = ValidatingWebhookConfiguration.DeepCopy()
	s.validatingWebhook.Webhooks[0].ClientConfig.Service.Port = nil

	s.webhookService = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestServiceName,
			Namespace: TestNamespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"Taylor": "Swift",
			},
			Ports: []corev1.ServicePort{
				{
					Port:     443,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}

	s.mockForReturningValidatingWebhook()
	s.mockReturningWebhookService()
	//s.mockServiceIsBlockedByOtterize(make([]v1.NetworkPolicy, 0))
	s.mockGetControlPlaneIPs()
	s.mockGetExistingOtterizeWebhooksNetpols([]v1.NetworkPolicy{})

	netpolMatcher := NewNetworkPolicyMatcher([]int32{443}, s.handler.allowAllIncomingTraffic, nil)
	s.Client.EXPECT().Create(gomock.Any(), gomock.All(netpolMatcher)).Return(nil)
	err := s.handler.HandleAll(context.Background())
	s.Require().NoError(err)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpol)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpolSuccess)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleAlways_WebhookPortNotFoundOnService_ExpectErrors() {
	s.handler = NewNetworkPolicyHandler(s.Client, &runtime.Scheme{}, automate_third_party_network_policy.Always, 32, false)
	s.handler.InjectRecorder(s.Recorder)

	s.validatingWebhook = ValidatingWebhookConfiguration.DeepCopy()
	s.validatingWebhook.Webhooks[0].ClientConfig.Service.Port = nil

	s.mockForReturningValidatingWebhook()
	s.mockReturningWebhookService()
	//s.mockServiceIsBlockedByOtterize(make([]v1.NetworkPolicy, 0))
	s.mockGetControlPlaneIPs()
	//s.mockGetExistingOtterizeWebhooksNetpols([]v1.NetworkPolicy{})

	//netpolMatcher := NewNetworkPolicyMatcher([]int32{443}, s.handler.allowAllIncomingTraffic, nil)
	//s.Client.EXPECT().Create(gomock.Any(), gomock.All(netpolMatcher)).Return(nil)
	err := s.handler.HandleAll(context.Background())
	s.Require().Error(err, "Webhook port was not found on service")
	s.ExpectEvent(ReasonWebhookPortNotFoundOnServiceError)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleAlways_WebhookNameTooLong_CreatePolicy() {
	s.handler = NewNetworkPolicyHandler(s.Client, &runtime.Scheme{}, automate_third_party_network_policy.Always, 32, false)
	s.handler.InjectRecorder(s.Recorder)
	s.validatingWebhook = ValidatingWebhookConfiguration.DeepCopy()
	s.validatingWebhook.Name = "A-lantern-lit-her-journey-through-the-meadow-A-breeze-whispered-shadows-danced-and-the-forest-echoed-with-silent-wonder"

	s.mockForReturningValidatingWebhook()
	s.mockReturningWebhookService()
	//s.mockServiceIsBlockedByOtterize(make([]v1.NetworkPolicy, 0))
	s.mockGetControlPlaneIPs()
	s.mockGetExistingOtterizeWebhooksNetpols([]v1.NetworkPolicy{})

	netpolMatcher := NewNetworkPolicyMatcher([]int32{TestServicePort}, false, lo.ToPtr("A-lantern-lit-her-journey-through-the-meadow-A-breeze-whispered-shadows-danced-and-the-forest-echoed-with-silent-wonder"))
	s.Client.EXPECT().Create(gomock.Any(), gomock.All(netpolMatcher)).Return(nil)
	err := s.handler.HandleAll(context.Background())
	s.Require().NoError(err)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpol)
	s.ExpectEvent(ReasonCreatingWebhookTrafficNetpolSuccess)
}

func (s *NetworkPolicyHandlerTestSuite) TestNetworkPolicyHandler_HandleIfBlockedByOtterize_ServiceIsBlockedByOtterize_TwoWebhooksToSameServiceDifferentPorts_CreatingOneWebhookPolicy() {
	secondPort := int32(1432)
	s.validatingWebhook.Webhooks = append(s.validatingWebhook.Webhooks,
		admissionv1.ValidatingWebhook{
			Name: "Second",
			ClientConfig: admissionv1.WebhookClientConfig{
				Service: &admissionv1.ServiceReference{
					Name:      TestServiceName,
					Namespace: TestNamespace,
					Port:      lo.ToPtr(secondPort),
				},
			},
		})
	s.webhookService.Spec.Ports = append(s.webhookService.Spec.Ports, corev1.ServicePort{
		Port:     secondPort,
		Protocol: corev1.ProtocolTCP,
	})

	s.mockForReturningValidatingWebhook()

	// Called once for "First" webhook
	s.mockReturningWebhookService()
	s.mockServiceIsBlockedByOtterize(OtterizeIngressNetpols)
	s.mockGetControlPlaneIPs()

	// Called second time for "Second"" webhook
	s.mockReturningWebhookService()
	s.mockServiceIsBlockedByOtterize(OtterizeIngressNetpols)
	s.mockGetControlPlaneIPs()

	s.mockGetExistingOtterizeWebhooksNetpols([]v1.NetworkPolicy{})

	netpolMatcher := NewNetworkPolicyMatcher([]int32{secondPort, TestServicePort}, s.handler.allowAllIncomingTraffic, nil)
	s.Client.EXPECT().Create(gomock.Any(), gomock.All(netpolMatcher)).Return(nil)
	err := s.handler.HandleAll(context.Background())
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

	// No mutating webhooks
	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&admissionv1.MutatingWebhookConfigurationList{}),
	).DoAndReturn(
		func(_ any, list *admissionv1.MutatingWebhookConfigurationList, _ ...any) error {
			return nil
		},
	)

	// No CRDs webhooks
	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&apiextensionsv1.CustomResourceDefinitionList{}),
	).DoAndReturn(
		func(_ any, list *apiextensionsv1.CustomResourceDefinitionList, _ ...any) error {
			return nil
		},
	)
}

func (s *NetworkPolicyHandlerTestSuite) mockForReturningMutatingWebhook() {
	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&admissionv1.ValidatingWebhookConfigurationList{}),
	).DoAndReturn(
		func(_ any, list *admissionv1.ValidatingWebhookConfigurationList, _ ...any) error {
			return nil
		},
	)

	// No mutating webhooks
	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&admissionv1.MutatingWebhookConfigurationList{}),
	).DoAndReturn(
		func(_ any, list *admissionv1.MutatingWebhookConfigurationList, _ ...any) error {
			list.Items = []admissionv1.MutatingWebhookConfiguration{MutatingWebhookConfiguration}
			return nil
		},
	)

	// No CRDs webhooks
	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&apiextensionsv1.CustomResourceDefinitionList{}),
	).DoAndReturn(
		func(_ any, list *apiextensionsv1.CustomResourceDefinitionList, _ ...any) error {
			return nil
		},
	)
}

func (s *NetworkPolicyHandlerTestSuite) mockForReturningCRDsWebhook() {
	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&admissionv1.ValidatingWebhookConfigurationList{}),
	).DoAndReturn(
		func(_ any, list *admissionv1.ValidatingWebhookConfigurationList, _ ...any) error {
			return nil
		},
	)

	// No mutating webhooks
	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&admissionv1.MutatingWebhookConfigurationList{}),
	).DoAndReturn(
		func(_ any, list *admissionv1.MutatingWebhookConfigurationList, _ ...any) error {
			return nil
		},
	)

	// No CRDs webhooks
	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&apiextensionsv1.CustomResourceDefinitionList{}),
	).DoAndReturn(
		func(_ any, list *apiextensionsv1.CustomResourceDefinitionList, _ ...any) error {
			list.Items = []apiextensionsv1.CustomResourceDefinition{CRDWebhookConfiguration}
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

func (s *NetworkPolicyHandlerTestSuite) mockServiceIsBlockedByOtterize(netpols []v1.NetworkPolicy) {
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
			netpolList.Items = netpols
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
			svc.Spec.ClusterIPs = []string{TestControlPlaneIP}
			svc.Name = "kubernetes"
			svc.Namespace = "default"
			return nil
		},
	)

	s.Client.EXPECT().Get(
		gomock.Any(), gomock.Eq(types.NamespacedName{Name: "kubernetes", Namespace: "default"}), gomock.Eq(&corev1.Endpoints{}),
	).DoAndReturn(
		func(_ any, _ any, endpoints *corev1.Endpoints, _ ...any) error {
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

func (s *NetworkPolicyHandlerTestSuite) mockGetNetworkPolicyForUpdate(netpol v1.NetworkPolicy) {
	s.Client.EXPECT().Get(
		gomock.Any(), gomock.Eq(types.NamespacedName{Name: netpol.Name, Namespace: netpol.Namespace}), gomock.Eq(&v1.NetworkPolicy{}),
	).DoAndReturn(
		func(_ any, _ any, output *v1.NetworkPolicy, _ ...any) error {
			netpol.DeepCopyInto(output)
			return nil
		},
	)
}

func TestNetworkPolicyHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(NetworkPolicyHandlerTestSuite))
}
