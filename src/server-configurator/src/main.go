package main

import (
	"context"
	spireclient "github.com/otterize/spifferize/src/spire-client"
	"github.com/sirupsen/logrus"
	entryv1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/entry/v1"
	"github.com/spiffe/spire-api-sdk/proto/spire/api/types"
	"github.com/spiffe/spire/cmd/spire-server/util"
)

const (
	DefaultSocketPath = "/tmp/spire-server/private/api.sock"
)

func createEntry(client util.ServerClient) {
	entry := types.Entry{
		SpiffeId: &types.SPIFFEID{TrustDomain: "example.org",
			Path: "/otterize/test"},
		ParentId: &types.SPIFFEID{TrustDomain: "example.org",
			Path: "/ns/spire/sa/spire-agent"},
		Selectors: []*types.Selector{
			{Type: "k8s", Value: "pod-label:test"},
			{Type: "k8s", Value: "ns:test"},
		},
	}

	entryClient := client.NewEntryClient()
	response, err := entryClient.BatchCreateEntry(context.Background(), &entryv1.BatchCreateEntryRequest{Entries: []*types.Entry{&entry}})
	if err != nil {
		panic(err)
	}

	logrus.Info(response.Results)
}

func listEntries(client util.ServerClient) {
	entryClient := client.NewEntryClient()
	response, err := entryClient.ListEntries(context.Background(), &entryv1.ListEntriesRequest{})
	if err != nil {
		panic(err)
	}

	logrus.Info(response.Entries)
}

// spiffe://example.org/ns/spire/sa/spire-agent
// spiffe://example.com/ns/spire/sa/spire-agent

func main() {
	client, err := spireclient.NewServerClientFromUnixSocket(DefaultSocketPath)
	//client, err := spireclient.NewServerClientFromTCP("127.0.0.1:8081")

	if err != nil {
		panic(err)
	}
	defer client.Release()

	createEntry(client)
}
