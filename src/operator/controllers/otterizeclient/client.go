package otterizeclient

import (
	"context"
	"errors"
	"fmt"
	"github.com/Khan/genqlient/graphql"
	"github.com/amit7itz/goset"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/otterize/spire-integration-operator/src/controllers/otterizeclient/otterizegraphql"
	"github.com/sirupsen/logrus"
)

type CloudClient struct {
	graphqlClient graphql.Client
	injectablerecorder.InjectableRecorder
}

func NewCloudClient(ctx context.Context) (*CloudClient, error) {
	client, ok, err := otterizecloudclient.NewClient(ctx)
	if err != nil {
		return nil, err
	} else if !ok {
		return nil, errors.New("missing cloud client credentials")
	}

	return &CloudClient{graphqlClient: client}, err
}
func (c *CloudClient) GetTLSKeyPair(ctx context.Context, serviceId string) (otterizegraphql.TLSKeyPair, error) {
	res, err := otterizegraphql.GetTLSKeyPair(ctx, c.graphqlClient, &serviceId)
	if err != nil {
		return otterizegraphql.TLSKeyPair{}, err
	}
	return res.Service.TlsKeyPair.TLSKeyPair, nil
}

func (c *CloudClient) RegisterK8SPod(ctx context.Context, namespace string, _ string, serviceName string, ttl int32, dnsNames []string) (string, error) {
	certCustomization := otterizegraphql.CertificateCustomization{DnsNames: dnsNames, Ttl: int(ttl)}
	res, err := otterizegraphql.RegisterKubernetesPodOwnerCertificateRequest(ctx, c.graphqlClient, namespace, serviceName, certCustomization)
	if err != nil {
		return "", err
	}
	return res.RegisterKubernetesPodOwnerCertificateRequest.Id, nil
}

func (c *CloudClient) CleanupOrphanK8SPodEntries(ctx context.Context, _ string, existingServicesByNamespace map[string]*goset.Set[string]) error {
	var namespacedPodOwners []otterizegraphql.NamespacedPodOwner
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
