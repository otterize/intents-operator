package spire_client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/spiffe/go-spiffe/v2/bundle/x509bundle"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	entryv1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/entry/v1"
	spireServerUtil "github.com/spiffe/spire/cmd/spire-server/util"
	"github.com/spiffe/spire/pkg/agent/client"
	spireCommonUtil "github.com/spiffe/spire/pkg/common/util"
	"google.golang.org/grpc"
)

func NewServerClientFromUnixSocket(socketPath string) (spireServerUtil.ServerClient, error) {
	addr, err := spireCommonUtil.GetUnixAddrWithAbsPath(socketPath)
	if err != nil {
		return nil, err
	}

	c, err := spireServerUtil.NewServerClient(addr)
	if err != nil {
		panic(err)
	}

	return c, nil
}

func serverConn(ctx context.Context, serverAddress string, svid *x509svid.SVID, bundleSet *x509bundle.Set) (*grpc.ClientConn, error) {
	trustDomain := svid.ID.TrustDomain()
	bundle, ok := bundleSet.Get(trustDomain)
	if !ok {
		return nil, fmt.Errorf("bundle is missing for domain %s", trustDomain)
	}

	return client.DialServer(ctx, client.DialServerConfig{
		Address:     serverAddress,
		TrustDomain: svid.ID.TrustDomain(),
		GetBundle: func() []*x509.Certificate {
			return bundle.X509Authorities()
		},
		GetAgentCertificate: func() *tls.Certificate {
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

func NewEntryClientFromTCP(serverAddress string, svid *x509svid.SVID, bundleSet *x509bundle.Set) (entryv1.EntryClient, error) {
	conn, err := serverConn(context.Background(), serverAddress, svid, bundleSet)
	if err != nil {
		return nil, err
	}

	return entryv1.NewEntryClient(conn), nil
}
