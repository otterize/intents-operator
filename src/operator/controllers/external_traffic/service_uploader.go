package external_traffic

import (
	"context"
	"fmt"
	lru "github.com/hashicorp/golang-lru"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServiceUploaderImpl struct {
	client.Client
	otterizeClient        operator_cloud_client.CloudClient
	serviceIdResolver     *serviceidresolver.Resolver
	reportedServicesCache *lru.ARCCache
}

type ServiceUploader interface {
	UploadNamespaceServices(ctx context.Context, namespace string) error
}

func NewServiceUploader(client client.Client, otterizeClient operator_cloud_client.CloudClient) ServiceUploader {
	return &ServiceUploaderImpl{
		Client:                client,
		otterizeClient:        otterizeClient,
		serviceIdResolver:     serviceidresolver.NewResolver(client),
		reportedServicesCache: shared.MustRet(lru.NewARC(1000)),
	}
}

func (s *ServiceUploaderImpl) UploadNamespaceServices(ctx context.Context, namespace string) error {
	if s.otterizeClient == nil {
		return nil
	}

	services := &corev1.ServiceList{}
	err := s.List(ctx, services, client.InNamespace(namespace))
	if err != nil {
		return errors.Wrap(err)
	}

	externallyAccessibleServices := make([]graphqlclient.ExternallyAccessibleServiceInput, 0)
	for _, service := range services.Items {
		externalService, isExternal, err := s.getExternalService(ctx, service)
		if err != nil {
			return errors.Wrap(err)
		}

		if isExternal {
			externallyAccessibleServices = append(externallyAccessibleServices, externalService)
		}
	}

	allServicesReported := lo.EveryBy(externallyAccessibleServices, func(service graphqlclient.ExternallyAccessibleServiceInput) bool {
		return s.reportedServicesCache.Contains(reportedServicesCacheKey(service.Namespace, service.ServerName))
	})

	if allServicesReported && len(externallyAccessibleServices) != 0 {
		return nil
	}

	err = s.otterizeClient.ReportExternallyAccessibleServices(ctx, namespace, externallyAccessibleServices)
	if err != nil {
		return errors.Wrap(err)
	}

	for _, service := range externallyAccessibleServices {
		s.reportedServicesCache.Add(reportedServicesCacheKey(service.Namespace, service.ServerName), true)
	}

	return nil
}

func reportedServicesCacheKey(namespace, serverName string) string {
	return fmt.Sprintf("%s%s", serverName, namespace)
}

func (s *ServiceUploaderImpl) getExternalService(ctx context.Context, svc corev1.Service) (graphqlclient.ExternallyAccessibleServiceInput, bool, error) {
	if svc.DeletionTimestamp != nil {
		return graphqlclient.ExternallyAccessibleServiceInput{}, false, nil
	}

	referringIngressList, err := getIngressRefersToService(ctx, s.Client, svc)
	if err != nil {
		return graphqlclient.ExternallyAccessibleServiceInput{}, false, errors.Wrap(err)
	}

	if !isServiceExternallyAccessible(svc, referringIngressList) {
		return graphqlclient.ExternallyAccessibleServiceInput{}, false, nil
	}

	// get pods that are targeted by the service
	podList := corev1.PodList{}
	err = s.Client.List(ctx, &podList, &client.ListOptions{Namespace: svc.Namespace, LabelSelector: labels.SelectorFromSet(svc.Spec.Selector)})
	if err != nil {
		return graphqlclient.ExternallyAccessibleServiceInput{}, false, errors.Wrap(err)
	}

	if len(podList.Items) == 0 {
		return graphqlclient.ExternallyAccessibleServiceInput{}, false, nil
	}

	identity, err := s.serviceIdResolver.ResolvePodToServiceIdentity(ctx, &podList.Items[0])
	if err != nil {
		return graphqlclient.ExternallyAccessibleServiceInput{}, false, errors.Wrap(err)
	}

	externalService, isExternal, err := convertToCloudExternalService(svc, identity, referringIngressList)
	if err != nil {
		return graphqlclient.ExternallyAccessibleServiceInput{}, false, errors.Wrap(err)
	}
	return externalService, isExternal, nil
}

func isServiceExternallyAccessible(svc corev1.Service, referringIngressList *v1.IngressList) bool {
	return svc.Spec.Type == corev1.ServiceTypeLoadBalancer || svc.Spec.Type == corev1.ServiceTypeNodePort || len(referringIngressList.Items) > 0
}

func convertToCloudExternalService(svc corev1.Service, identity serviceidentity.ServiceIdentity, referringIngressList *v1.IngressList) (graphqlclient.ExternallyAccessibleServiceInput, bool, error) {
	ReferredByIngress := len(referringIngressList.Items) > 0
	var cloudServiceType graphqlclient.KubernetesServiceType
	switch svc.Spec.Type {
	case corev1.ServiceTypeLoadBalancer:
		cloudServiceType = graphqlclient.KubernetesServiceTypeLoadBalancer
	case corev1.ServiceTypeNodePort:
		cloudServiceType = graphqlclient.KubernetesServiceTypeNodePort
	case corev1.ServiceTypeClusterIP:
		cloudServiceType = graphqlclient.KubernetesServiceTypeClusterIp
	case corev1.ServiceTypeExternalName:
		cloudServiceType = graphqlclient.KubernetesServiceTypeExternalName
	default:
		// Should not reach here until k8s adds new service type
		return graphqlclient.ExternallyAccessibleServiceInput{}, false, errors.Errorf("unsupported service type: %s", svc.Spec.Type)
	}

	serviceInput := graphqlclient.ExternallyAccessibleServiceInput{
		Namespace:         identity.Namespace,
		ServerName:        identity.Name,
		ReferredByIngress: ReferredByIngress,
		ServiceType:       cloudServiceType,
	}
	return serviceInput, true, nil
}

func getIngressRefersToService(ctx context.Context, k8sClient client.Client, svc corev1.Service) (*v1.IngressList, error) {
	var ingressList v1.IngressList
	err := k8sClient.List(
		ctx,
		&ingressList,
		&client.MatchingFields{v1alpha3.IngressServiceNamesIndexField: svc.Name},
		&client.ListOptions{Namespace: svc.Namespace})
	if err != nil {
		return nil, errors.Wrap(err)
	}

	return &ingressList, nil
}
