package main

import (
	"context"
	spire_client "github.com/otterize/spifferize/src/spire-client"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	entryv1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/entry/v1"
	"github.com/spiffe/spire-api-sdk/proto/spire/api/types"
	"github.com/spiffe/spire/cmd/spire-server/util"
	"os"
)

const (
	DefaultSocketPath = "/tmp/spire-server/private/api.sock"
	AgentSocketPath   = "unix:///tmp/spire-agent/public/api.sock"
)

func createEntry(entryClient entryv1.EntryClient) {
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
	//client, err := spireclient.NewServerClientFromUnixSocket(DefaultSocketPath)
	//client, err := spireclient.NewServerClientFromTCP("127.0.0.1:8081")

	source, err := workloadapi.New(context.Background(), workloadapi.WithAddr(AgentSocketPath))
	if err != nil {
		logrus.Error(err, "unable to start source")
		os.Exit(1)
	}
	defer source.Close()
	logrus.Info("API Initialized")
	svid, err := source.FetchX509SVID(context.Background())
	if err != nil {
		logrus.Error(err, "failed to get svid")
		os.Exit(1)
	}
	bundle, err := source.FetchX509Bundles(context.Background())
	if err != nil {
		logrus.Error(err, "failed to get bundle")
		os.Exit(1)
	}

	logrus.Infof("svid: %s", svid.ID)

	entryClient, err := spire_client.NewEntryClientFromTCP("127.0.0.1:8081", svid, bundle)
	if err != nil {
		logrus.Error(err, "failed to get svid")
		os.Exit(1)
	}
	response, err := entryClient.ListEntries(context.Background(), &entryv1.ListEntriesRequest{})
	if err != nil {
		panic(err)
	}

	logrus.Info(response.Entries)

	//entryClient, err := spireclient.NewEntryClientFromTCP("127.0.0.1:8081", nil)

	//if err != nil {
	//	panic(err)
	//}
	//defer client.Release()

	//createEntry(entryClient)
}
