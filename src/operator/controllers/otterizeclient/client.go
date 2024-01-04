package otterizeclient

import (
	"context"
	"fmt"
	"github.com/Khan/genqlient/graphql"
	"github.com/amit7itz/goset"
	"github.com/otterize/credentials-operator/src/controllers/otterizeclient/otterizegraphql"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/sirupsen/logrus"
)

type CloudClient struct {
	graphqlClient graphql.Client
	injectablerecorder.InjectableRecorder
}

func NewCloudClient(ctx context.Context) (*CloudClient, bool, error) {
	client, ok, err := otterizecloudclient.NewClient(ctx)
	if err != nil {
		return nil, false, errors.Wrap(err)
	}

	if !ok {
		return nil, false, nil
	}

	return &CloudClient{graphqlClient: client}, true, errors.Wrap(err)
}
func (c *CloudClient) GetTLSKeyPair(ctx context.Context, serviceId string) (otterizegraphql.TLSKeyPair, error) {
	res, err := otterizegraphql.GetTLSKeyPair(ctx, c.graphqlClient, &serviceId)
	if err != nil {
		return otterizegraphql.TLSKeyPair{}, errors.Wrap(err)
	}
	return res.Service.TlsKeyPair.TLSKeyPair, nil
}

func (c *CloudClient) RegisterK8SPod(ctx context.Context, namespace string, _ string, serviceName string, ttl int32, dnsNames []string) (string, error) {
	certCustomization := otterizegraphql.CertificateCustomization{DnsNames: dnsNames, Ttl: int(ttl)}
	res, err := otterizegraphql.RegisterKubernetesPodOwnerCertificateRequest(ctx, c.graphqlClient, namespace, serviceName, certCustomization)
	if err != nil {
		return "", errors.Wrap(err)
	}
	return res.RegisterKubernetesPodOwnerCertificateRequest.Id, nil
}

func (c *CloudClient) AcquireServiceUserAndPassword(ctx context.Context, serviceName, namespace string) (*otterizegraphql.UserPasswordCredentials, error) {
	userAndPasswordResponse, err := otterizegraphql.RequestUserAndPassword(ctx, c.graphqlClient, serviceName, namespace)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	getUserAndPasswordResponse, err := otterizegraphql.GetUserAndPasswordCredentials(ctx, c.graphqlClient, userAndPasswordResponse.RegisterKubernetesServiceUserAndPasswordRequest.Id)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	return &getUserAndPasswordResponse.Service.UserAndPassword.UserPasswordCredentials, nil
}

func (c *CloudClient) CleanupOrphanK8SPodEntries(ctx context.Context, _ string, existingServicesByNamespace map[string]*goset.Set[string]) error {
	namespacedPodOwners := make([]otterizegraphql.NamespacedPodOwner, 0)
	for namespace, podOwnerNames := range existingServicesByNamespace {
		for _, podOwner := range podOwnerNames.Items() {
			namespacedPodOwners = append(namespacedPodOwners, otterizegraphql.NamespacedPodOwner{Namespace: namespace, Name: podOwner})
		}
	}
	res, err := otterizegraphql.ReportActiveCertificateRequesters(ctx, c.graphqlClient, namespacedPodOwners)
	if err != nil {
		return fmt.Errorf("failed removing orphan entries: %w", err)
	}
	if !res.ReportActiveCertificateRequesters {
		return fmt.Errorf("failed removing orphan entries")
	}

	return nil
}

func (c *CloudClient) ReportComponentStatus(ctx context.Context) {
	_, err := otterizegraphql.ReportComponentStatus(ctx, c.graphqlClient, otterizegraphql.ComponentTypeCredentialsOperator)
	if err != nil {
		logrus.WithError(err).Error("Failed to report component status Otterize cloud")
	}
}
