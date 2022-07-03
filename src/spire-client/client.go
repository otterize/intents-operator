package spire_client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/spiffe/go-spiffe/v2/bundle/x509bundle"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
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

	ClientConf() ServerClientConfig

	NewAgentClient() agentv1.AgentClient
	NewBundleClient() bundlev1.BundleClient
	NewEntryClient() entryv1.EntryClient
	NewSVIDClient() svidv1.SVIDClient
	NewTrustDomainClient() trustdomainv1.TrustDomainClient
	NewHealthClient() grpc_health_v1.HealthClient
}

type ServerClientConfig struct {
	SVID   *x509svid.SVID
	Bundle *x509bundle.Set
}

func (conf ServerClientConfig) TrustDomain() spiffeid.TrustDomain {
	return conf.SVID.ID.TrustDomain()
}

func (conf ServerClientConfig) ClientSpiffeID() spiffeid.ID {
	return conf.SVID.ID
}

type serverClient struct {
	conn       *grpc.ClientConn
	clientConf ServerClientConfig
}

func serverConn(ctx context.Context, serverAddress string, clientConf ServerClientConfig) (*grpc.ClientConn, error) {
	trustDomain := clientConf.SVID.ID.TrustDomain()
	bundle, ok := clientConf.Bundle.Get(trustDomain)
	if !ok {
		return nil, fmt.Errorf("bundle is missing for domain %s", trustDomain)
	}

	return client.DialServer(ctx, client.DialServerConfig{
		Address:     serverAddress,
		TrustDomain: trustDomain,
		GetBundle: func() []*x509.Certificate {
			return bundle.X509Authorities()
		},
		GetAgentCertificate: func() *tls.Certificate {
			agentCert := &tls.Certificate{
				PrivateKey: clientConf.SVID.PrivateKey,
			}
			for _, cert := range clientConf.SVID.Certificates {
				agentCert.Certificate = append(agentCert.Certificate, cert.Raw)
			}
			return agentCert
		},
	})
}

func NewServerClient(ctx context.Context, serverAddress string, clientConf ServerClientConfig) (ServerClient, error) {
	conn, err := serverConn(ctx, serverAddress, clientConf)
	if err != nil {
		return nil, err
	}

	return &serverClient{clientConf: clientConf, conn: conn}, nil
}

func (c *serverClient) Close() {
	_ = c.conn.Close()
}

func (c *serverClient) ClientConf() ServerClientConfig {
	return c.clientConf
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
