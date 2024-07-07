package external_traffic

import (
	"context"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	otterizecloudmocks "github.com/otterize/intents-operator/src/shared/otterizecloud/mocks"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"
)

const testNamespace = "test-namespace"

type ServiceUploaderTestSuite struct {
	testbase.MocksSuiteBase
	serviceUploader ServiceUploader
	otterizeClient  *otterizecloudmocks.MockCloudClient
}

func (s *ServiceUploaderTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	controller := gomock.NewController(s.T())
	s.otterizeClient = otterizecloudmocks.NewMockCloudClient(controller)
	s.serviceUploader = NewServiceUploader(s.Client, s.otterizeClient)
}

func (s *ServiceUploaderTestSuite) TearDownTest() {
	s.serviceUploader = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *ServiceUploaderTestSuite) TestUploadNamespaceServices() {
	localServiceName := "local-service"
	const podForServiceWithIngressName = "pod-for-service-with-ingress"
	const podForServiceWithNodePortName = "pod-for-service-with-nodeport"
	const podForServiceWithLoadBalancerName = "pod-for-service-with-loadbalancer"
	const serviceWithIngressName = "service-with-ingress"
	const serviceWithNodePortName = "service-with-node-port"
	const deletedServiceWithNodePortName = "deleted-service-with-node-port"
	const serviceWithLoadBalancerName = "service-with-load-balancer"
	localService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      localServiceName,
			Namespace: testNamespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
		},
	}
	serviceWithIngress := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceWithIngressName,
			Namespace: testNamespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app": "ingress",
			},
		},
	}
	serviceWithNodePort := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceWithNodePortName,
			Namespace: testNamespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Selector: map[string]string{
				"app": "nodeport",
			},
		},
	}
	deletedServiceWithNodePort := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:              deletedServiceWithNodePortName,
			Namespace:         testNamespace,
			DeletionTimestamp: &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
		},
	}
	serviceWithLoadBalancer := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceWithLoadBalancerName,
			Namespace: testNamespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Selector: map[string]string{
				"app": "loadbalancer",
			},
		},
	}

	services := &corev1.ServiceList{
		Items: []corev1.Service{
			localService,
			serviceWithIngress,
			serviceWithNodePort,
			deletedServiceWithNodePort,
			serviceWithLoadBalancer,
		},
	}

	ingressList := &v1.IngressList{
		Items: []v1.Ingress{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress-1",
					Namespace: testNamespace,
				},
				Spec: v1.IngressSpec{
					Rules: []v1.IngressRule{
						{
							Host: "test-host",
							IngressRuleValue: v1.IngressRuleValue{
								HTTP: &v1.HTTPIngressRuleValue{
									Paths: []v1.HTTPIngressPath{
										{
											Backend: v1.IngressBackend{
												Service: &v1.IngressServiceBackend{
													Name: serviceWithIngressName,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	emptyServicesList := corev1.ServiceList{}
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&emptyServicesList), gomock.Eq(client.InNamespace(testNamespace))).DoAndReturn(
		func(ctx context.Context, list *corev1.ServiceList, opts ...client.ListOption) error {
			services.DeepCopyInto(list)
			return nil
		})

	emptyIngressesList := v1.IngressList{}
	for _, svc := range services.Items {
		if svc.Name == deletedServiceWithNodePortName {
			continue
		}
		result := &v1.IngressList{}
		if svc.Name == serviceWithIngressName {
			ingressList.DeepCopyInto(result)
		}
		s.Client.EXPECT().List(
			gomock.Any(),
			gomock.Eq(&emptyIngressesList),
			gomock.Eq(&client.MatchingFields{v1alpha3.IngressServiceNamesIndexField: svc.Name}),
			gomock.Eq(&client.ListOptions{Namespace: svc.Namespace})).DoAndReturn(
			func(ctx context.Context, list *v1.IngressList, opts ...client.ListOption) error {
				result.DeepCopyInto(list)
				return nil
			})
	}

	s.Client.EXPECT().List(gomock.Any(),
		gomock.Eq(&corev1.PodList{}),
		&client.ListOptions{Namespace: testNamespace, LabelSelector: labels.SelectorFromSet(serviceWithIngress.Spec.Selector)}).DoAndReturn(
		func(ctx context.Context, podList *corev1.PodList, listOptions ...client.ListOption) error {
			podList.Items = []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      podForServiceWithIngressName,
						Namespace: serviceWithIngress.Namespace,
						Labels: map[string]string{
							"app": "ingress",
						},
					},
				},
			}
			return nil
		})

	s.Client.EXPECT().List(gomock.Any(),
		gomock.Eq(&corev1.PodList{}),
		&client.ListOptions{Namespace: testNamespace, LabelSelector: labels.SelectorFromSet(serviceWithNodePort.Spec.Selector)}).DoAndReturn(
		func(ctx context.Context, podList *corev1.PodList, listOptions ...client.ListOption) error {
			podList.Items = []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      podForServiceWithNodePortName,
						Namespace: serviceWithNodePort.Namespace,
						Labels: map[string]string{
							"app": "nodeport",
						},
					},
				},
			}
			return nil
		})

	s.Client.EXPECT().List(gomock.Any(),
		gomock.Eq(&corev1.PodList{}),
		&client.ListOptions{Namespace: testNamespace, LabelSelector: labels.SelectorFromSet(serviceWithLoadBalancer.Spec.Selector)}).DoAndReturn(
		func(ctx context.Context, podList *corev1.PodList, listOptions ...client.ListOption) error {
			podList.Items = []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      podForServiceWithLoadBalancerName,
						Namespace: serviceWithLoadBalancer.Namespace,
						Labels: map[string]string{
							"app": "loadbalancer",
						},
					},
				},
			}
			return nil
		})

	externalServiceInputList := []graphqlclient.ExternallyAccessibleServiceInput{
		{
			Namespace:         testNamespace,
			ServerName:        podForServiceWithIngressName,
			ReferredByIngress: true,
			ServiceType:       graphqlclient.KubernetesServiceTypeClusterIp,
		},
		{
			Namespace:         testNamespace,
			ServerName:        podForServiceWithNodePortName,
			ReferredByIngress: false,
			ServiceType:       graphqlclient.KubernetesServiceTypeNodePort,
		},
		{
			Namespace:         testNamespace,
			ServerName:        podForServiceWithLoadBalancerName,
			ReferredByIngress: false,
			ServiceType:       graphqlclient.KubernetesServiceTypeLoadBalancer,
		},
	}
	s.otterizeClient.EXPECT().ReportExternallyAccessibleServices(gomock.Any(), testNamespace, gomock.InAnyOrder(externalServiceInputList)).Return(nil)
	err := s.serviceUploader.UploadNamespaceServices(context.Background(), testNamespace)
	s.Require().NoError(err)
}

func (s *ServiceUploaderTestSuite) TestReportEmptyNamespace() {
	emptyServicesList := corev1.ServiceList{}
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&emptyServicesList), gomock.Eq(client.InNamespace(testNamespace))).Return(nil)

	emptyInput := make([]graphqlclient.ExternallyAccessibleServiceInput, 0)
	s.otterizeClient.EXPECT().ReportExternallyAccessibleServices(gomock.Any(), testNamespace, emptyInput).Return(nil)
	err := s.serviceUploader.UploadNamespaceServices(context.Background(), testNamespace)
	s.Require().NoError(err)
}

func (s *ServiceUploaderTestSuite) TestReportEmptyNamespaceWithDeletedService() {
	deletedServiceWithNodePortName := "deleted-service-with-node-port"
	deletedServiceWithNodePort := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:              deletedServiceWithNodePortName,
			Namespace:         testNamespace,
			DeletionTimestamp: &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
		},
	}
	services := &corev1.ServiceList{
		Items: []corev1.Service{
			deletedServiceWithNodePort,
		},
	}

	emptyServicesList := corev1.ServiceList{}
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&emptyServicesList), gomock.Eq(client.InNamespace(testNamespace))).DoAndReturn(
		func(ctx context.Context, list *corev1.ServiceList, opts ...client.ListOption) error {
			services.DeepCopyInto(list)
			return nil
		})

	emptyInput := make([]graphqlclient.ExternallyAccessibleServiceInput, 0)
	s.otterizeClient.EXPECT().ReportExternallyAccessibleServices(gomock.Any(), testNamespace, emptyInput).Return(nil)
	err := s.serviceUploader.UploadNamespaceServices(context.Background(), testNamespace)
	s.Require().NoError(err)
}

func TestServiceUploaderTestSuite(t *testing.T) {
	suite.Run(t, new(ServiceUploaderTestSuite))
}
