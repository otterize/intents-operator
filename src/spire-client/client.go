package spire_client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/spiffe/go-spiffe/v2/bundle/x509bundle"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	entryv1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/entry/v1"
	"github.com/spiffe/spire/pkg/agent/client"
	"google.golang.org/grpc"
)

type ServerClient interface {
	Close()
	GetTrustDomain() spiffeid.TrustDomain
	GetClientSpiffeID() spiffeid.ID
	NewEntryClient() entryv1.EntryClient
}

type serverClient struct {
	conf ServerClientConfig
	conn *grpc.ClientConn
}

type ServerClientConfig struct {
	ServerAddress string
	ClientSVID    *x509svid.SVID
	ClientBundle  *x509bundle.Set
}

func (c *ServerClientConfig) GetTrustDomain() spiffeid.TrustDomain {
	return c.ClientSVID.ID.TrustDomain()
}

func serverConn(ctx context.Context, conf ServerClientConfig) (*grpc.ClientConn, error) {
	trustDomain := conf.GetTrustDomain()
	bundle, ok := conf.ClientBundle.Get(trustDomain)
	if !ok {
		return nil, fmt.Errorf("bundle is missing for domain %s", trustDomain)
	}

	return client.DialServer(ctx, client.DialServerConfig{
		Address:     conf.ServerAddress,
		TrustDomain: trustDomain,
		GetBundle: func() []*x509.Certificate {
			return bundle.X509Authorities()
		},
		GetAgentCertificate: func() *tls.Certificate {
			agentCert := &tls.Certificate{
				PrivateKey: conf.ClientSVID.PrivateKey,
			}
			for _, cert := range conf.ClientSVID.Certificates {
				agentCert.Certificate = append(agentCert.Certificate, cert.Raw)
			}
			return agentCert
		},
	})
}

func NewServerClient(conf ServerClientConfig) (ServerClient, error) {
	conn, err := serverConn(context.Background(), conf)
	if err != nil {
		return nil, err
	}

	return &serverClient{conf: conf, conn: conn}, nil
}

func (c *serverClient) Close() {
	_ = c.conn.Close()
}

func (c *serverClient) GetTrustDomain() spiffeid.TrustDomain {
	return c.conf.GetTrustDomain()
}

func (c *serverClient) GetClientSpiffeID() spiffeid.ID {
	return c.conf.ClientSVID.ID
}

func (c *serverClient) NewEntryClient() entryv1.EntryClient {
	return entryv1.NewEntryClient(c.conn)
}
