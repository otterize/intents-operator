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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"
)

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
	serviceWithIngressName := "service-with-ingress"
	serviceWithNodePortName := "service-with-node-port"
	deletedServiceWithNodePortName := "deleted-service-with-node-port"
	serviceWithLoadBalancerName := "service-with-load-balancer"
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
		},
	}
	serviceWithNodePort := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceWithNodePortName,
			Namespace: testNamespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
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

	externalServiceInputList := []graphqlclient.ExternallyAccessibleServiceInput{
		{
			Namespace:         testNamespace,
			ServerName:        serviceWithIngressName,
			ReferredByIngress: true,
			ServiceType:       graphqlclient.KubernetesServiceTypeClusterIp,
		},
		{
			Namespace:         testNamespace,
			ServerName:        serviceWithNodePortName,
			ReferredByIngress: false,
			ServiceType:       graphqlclient.KubernetesServiceTypeNodePort,
		},
		{
			Namespace:         testNamespace,
			ServerName:        serviceWithLoadBalancerName,
			ReferredByIngress: false,
			ServiceType:       graphqlclient.KubernetesServiceTypeLoadBalancer,
		},
	}
	s.otterizeClient.EXPECT().ReportExternallyAccessibleServices(gomock.Any(), testNamespace, externalServiceInputList).Return(nil)
	err := s.serviceUploader.UploadNamespaceServices(context.Background(), testNamespace)
	s.Require().NoError(err)
}

func (s *ServiceUploaderTestSuite) TestDoNotUploadNamespaceServicesIfOtterizeClientIsNil() {
	s.serviceUploader = NewServiceUploader(s.Client, nil)
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
