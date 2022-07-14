package spireclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	agentv1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/agent/v1"
	bundlev1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/bundle/v1"
	entryv1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/entry/v1"
	svidv1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/svid/v1"
	trustdomainv1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/trustdomain/v1"
	"github.com/spiffe/spire/pkg/agent/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type ServerClient interface {
	Close()

	GetSpiffeID() spiffeid.ID

	NewAgentClient() agentv1.AgentClient
	NewBundleClient() bundlev1.BundleClient
	NewEntryClient() entryv1.EntryClient
	NewSVIDClient() svidv1.SVIDClient
	NewTrustDomainClient() trustdomainv1.TrustDomainClient
	NewHealthClient() grpc_health_v1.HealthClient
}

type serverClient struct {
	conn     *grpc.ClientConn
	source   *workloadapi.X509Source
	spiffeID spiffeid.ID
}

func serverConn(ctx context.Context, serverAddress string, trustDomain spiffeid.TrustDomain, source *workloadapi.X509Source) (*grpc.ClientConn, error) {
	return client.DialServer(ctx, client.DialServerConfig{
		Address:     serverAddress,
		TrustDomain: trustDomain,
		GetBundle: func() []*x509.Certificate {
			bundle, err := source.GetX509BundleForTrustDomain(trustDomain)
			if err != nil {
				logrus.WithError(err).Error("failed to get bundle source")
				return nil
			}

			return bundle.X509Authorities()
		},
		GetAgentCertificate: func() *tls.Certificate {
			svid, err := source.GetX509SVID()
			if err != nil {
				logrus.WithError(err).Error("failed to get svid source")
				return nil
			}

			agentCert := &tls.Certificate{
				PrivateKey: svid.PrivateKey,
			}
			for _, cert := range svid.Certificates {
				agentCert.Certificate = append(agentCert.Certificate, cert.Raw)
			}
			return agentCert
		},
	})
}

func NewServerClient(ctx context.Context, serverAddress string, source *workloadapi.X509Source) (ServerClient, error) {
	svid, err := source.GetX509SVID()
	if err != nil {
		return nil, err
	}
	spiffeID := svid.ID
	trustDomain := spiffeID.TrustDomain()

	conn, err := serverConn(ctx, serverAddress, trustDomain, source)
	if err != nil {
		return nil, err
	}

	return &serverClient{conn: conn, source: source, spiffeID: spiffeID}, nil
}

func (c *serverClient) Close() {
	_ = c.source.Close()
	_ = c.conn.Close()
}

func (c *serverClient) GetSpiffeID() spiffeid.ID {
	return c.spiffeID
}

func (c *serverClient) NewAgentClient() agentv1.AgentClient {
	return agentv1.NewAgentClient(c.conn)
}

func (c *serverClient) NewBundleClient() bundlev1.BundleClient {
	return bundlev1.NewBundleClient(c.conn)
}

func (c *serverClient) NewEntryClient() entryv1.EntryClient {
	return entryv1.NewEntryClient(c.conn)
}

func (c *serverClient) NewSVIDClient() svidv1.SVIDClient {
	return svidv1.NewSVIDClient(c.conn)
}

func (c *serverClient) NewTrustDomainClient() trustdomainv1.TrustDomainClient {
	return trustdomainv1.NewTrustDomainClient(c.conn)
}

func (c *serverClient) NewHealthClient() grpc_health_v1.HealthClient {
	return grpc_health_v1.NewHealthClient(c.conn)
}
