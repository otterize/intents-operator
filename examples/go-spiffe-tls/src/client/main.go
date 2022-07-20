package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/otterize/spifferize/examples/spiffe-tls/utils"
	"io"
	"log"
	"time"

	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/spiffetls"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
)

const (
	serverAddress = "go-spiffe-server-service:55555"
	bundlePath    = "/etc/spifferize/bundle.pem"
	certFilePath  = "/etc/spifferize/svid.pem"
	keyFilePath   = "/etc/spifferize/key.pem"
	serverID      = "spiffe://example.org/otterize/namespace/go-spiffe-tls/service/server"
)

func dial() {
	// Setup context
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	log.Printf("Client starting to %s", serverAddress)

	// Allowed SPIFFE ID
	serverID := spiffeid.RequireFromString(serverID)

	// Create a TLS connection.
	// The client expects the server to present an SVID with the spiffeID: 'spiffe://example.org/server'
	//
	// An alternative when creating Dial is using `spiffetls.Dial` that uses environment variable `SPIFFE_ENDPOINT_SOCKET`
	svid := utils.NewLocalSVIDSource(certFilePath, keyFilePath)
	bundle := utils.NewLocalBundleSource(bundlePath)

	conn, err := spiffetls.DialWithMode(ctx, "tcp", serverAddress,
		spiffetls.MTLSClientWithRawConfig(
			tlsconfig.AuthorizeID(serverID),
			svid,
			bundle,
		))
	if err != nil {
		log.Fatalf("Unable to create TLS connection: %v", err)
	}
	defer conn.Close()

	// Send a message to the server using the TLS connection
	fmt.Fprintf(conn, "Hello server\n")

	// Read server response
	status, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil && err != io.EOF {
		log.Fatalf("Unable to read server response: %v", err)
	}
	log.Printf("Server says: %q", status)
}

func main() {
	ticker := time.NewTicker(10 * time.Second)
	for {
		select {
		case <-ticker.C:
			dial()
		}
	}
}
