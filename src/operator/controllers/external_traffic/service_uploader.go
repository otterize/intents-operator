package external_traffic

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServiceUploaderImpl struct {
	client.Client
	otterizeClient operator_cloud_client.CloudClient
}

type ServiceUploader interface {
	UploadNamespaceServices(ctx context.Context, namespace string) error
}

func NewServiceUploader(client client.Client, otterizeClient operator_cloud_client.CloudClient) ServiceUploader {
	return &ServiceUploaderImpl{
		Client:         client,
		otterizeClient: otterizeClient,
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
		externalService, isExternal, err := s.getExternalService(ctx, &service)
		if err != nil {
			return errors.Wrap(err)
		}

		if isExternal {
			externallyAccessibleServices = append(externallyAccessibleServices, externalService)
		}
	}

	err = s.otterizeClient.ReportExternallyAccessibleServices(ctx, namespace, externallyAccessibleServices)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (s *ServiceUploaderImpl) getExternalService(ctx context.Context, svc *corev1.Service) (graphqlclient.ExternallyAccessibleServiceInput, bool, error) {
	if svc.DeletionTimestamp != nil {
		return graphqlclient.ExternallyAccessibleServiceInput{}, false, nil
	}

	ingressList, err := getIngressRefersToService(ctx, s.Client, svc)
	if err != nil {
		return graphqlclient.ExternallyAccessibleServiceInput{}, false, errors.Wrap(err)
	}

	externalService, isExternal, err := convertToCloudExternalService(svc, ingressList)
	if err != nil {
		return graphqlclient.ExternallyAccessibleServiceInput{}, false, errors.Wrap(err)
	}
	return externalService, isExternal, nil
}
