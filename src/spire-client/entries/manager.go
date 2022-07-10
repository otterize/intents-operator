package entries

import (
	"context"
	"fmt"
	spire_client "github.com/otterize/spifferize/src/spire-client"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	entryv1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/entry/v1"
	"github.com/spiffe/spire-api-sdk/proto/spire/api/types"
	"google.golang.org/grpc/codes"
)

const (
	ServiceNamePodLabel = "otterize/service-name"
)

type Manager struct {
	SpireClient spire_client.ServerClient
}

func NewEntriesManager(spireClient spire_client.ServerClient) *Manager {
	return &Manager{SpireClient: spireClient}
}

func (m *Manager) RegisterK8SPodEntry(ctx context.Context, namespace string, serviceName string) (spiffeid.ID, error) {
	log := logrus.WithFields(logrus.Fields{"namespace": namespace, "service_name": serviceName})

	trustDomain := m.SpireClient.GetSpiffeID().TrustDomain()
	podSpiffeIDPath := fmt.Sprintf("/otterize/namespace/%s/service/%s", namespace, serviceName)
	parentSpiffeIDPath := m.SpireClient.GetSpiffeID().Path()

	entry := types.Entry{
		SpiffeId: &types.SPIFFEID{
			TrustDomain: trustDomain.String(),
			Path:        podSpiffeIDPath,
		},
		ParentId: &types.SPIFFEID{
			TrustDomain: trustDomain.String(),
			Path:        parentSpiffeIDPath,
		},
		Selectors: []*types.Selector{
			{Type: "k8s", Value: fmt.Sprintf("ns:%s", namespace)},
			{Type: "k8s", Value: fmt.Sprintf("pod-label:%s=%s", ServiceNamePodLabel, serviceName)},
		},
	}

	log.Info("Creating SPIRE server entry")
	entryClient := m.SpireClient.NewEntryClient()
	batchCreateEntryRequest := entryv1.BatchCreateEntryRequest{Entries: []*types.Entry{&entry}}

	resp, err := entryClient.BatchCreateEntry(ctx, &batchCreateEntryRequest)
	if err != nil {
		return spiffeid.ID{}, err
	}

	if len(resp.Results) != 1 {
		return spiffeid.ID{}, fmt.Errorf("unexpected number of results returned from SPIRE server, expected exactly 1 and got %d", len(resp.Results))
	}

	result := resp.Results[0]
	switch result.Status.Code {
	case int32(codes.OK):
		log.WithField("entry_id", result.Entry.Id).Info("SPIRE server entry created")
	case int32(codes.AlreadyExists):
		log.WithField("entry_id", result.Entry.Id).Info("SPIRE server entry already exists")
	default:
		return spiffeid.ID{}, fmt.Errorf("entry failed to create with status %s", result.Status)
	}

	id, err := spiffeid.FromPath(trustDomain, entry.SpiffeId.Path)
	if err != nil {
		return spiffeid.ID{}, err
	}

	return id, nil
}
