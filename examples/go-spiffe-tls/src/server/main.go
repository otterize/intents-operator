package main

import (
	"bufio"
	"context"
	"github.com/otterize/credentials-operator/examples/spiffe-tls/utils"
	"log"
	"net"

	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/spiffetls"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
)

const (
	serverAddress = "0.0.0.0:55555"
	bundlePath    = "/etc/spire-integration/bundle.pem"
	certFilePath  = "/etc/spire-integration/svid.pem"
	keyFilePath   = "/etc/spire-integration/key.pem"
	clientID      = "spiffe://example.org/otterize/namespace/go-spiffe-tls/service/client"
)

func main() {
	// Setup context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Printf("Server starting on %s", serverAddress)

	// Allowed SPIFFE ID
	clientID := spiffeid.RequireFromString(clientID)

	svid := utils.NewLocalSVIDSource(certFilePath, keyFilePath)
	bundle := utils.NewLocalBundleSource(bundlePath)

	// Creates a TLS listener
	// The server expects the client to present an Certificate with the spiffeID: 'spiffe://example.org/client'
	//
	// An alternative when creating Listen is using `spiffetls.Listen` that uses environment variable `SPIFFE_ENDPOINT_SOCKET`
	listener, err := spiffetls.ListenWithMode(ctx, "tcp", serverAddress,
		spiffetls.MTLSServerWithRawConfig(
			tlsconfig.AuthorizeID(clientID),
			svid,
			bundle,
		))
	if err != nil {
		log.Fatalf("Unable to create TLS listener: %v", err)
	}
	defer listener.Close()
	log.Print("Server started")

	// Handle connections
	for {
		conn, err := listener.Accept()
		if err != nil {
			go handleError(err)
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	// Read incoming data into buffer
	req, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		log.Printf("Error reading incoming data: %v", err)
		return
	}
	log.Printf("Client says: %q", req)

	// Send a response back to the client
	if _, err = conn.Write([]byte("Hello client\n")); err != nil {
		log.Printf("Unable to send response: %v", err)
		return
	}
}

func handleError(err error) {
	log.Printf("Unable to accept connection: %v", err)
}
