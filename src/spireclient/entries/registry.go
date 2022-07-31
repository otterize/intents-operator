package entries

import (
	"context"
	"fmt"
	"github.com/otterize/spire-integration-operator/src/spireclient"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	entryv1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/entry/v1"
	"github.com/spiffe/spire-api-sdk/proto/spire/api/types"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"strings"
)

type Registry interface {
	RegisterK8SPodEntry(ctx context.Context, namespace string, serviceNameLabel string, serviceName string, ttl int32, dnsNames []string) (string, error)
}

type registryImpl struct {
	parentSpiffeID spiffeid.ID
	entryClient    entryv1.EntryClient
}

func NewEntriesRegistry(spireClient spireclient.ServerClient) Registry {
	return &registryImpl{
		parentSpiffeID: spireClient.GetSpiffeID(),
		entryClient:    spireClient.NewEntryClient(),
	}
}

func (r *registryImpl) RegisterK8SPodEntry(ctx context.Context, namespace string, serviceNameLabel string, serviceName string, ttl int32, dnsNames []string) (string, error) {
	log := logrus.WithFields(logrus.Fields{"namespace": namespace, "service_name": serviceName})

	trustDomain := r.parentSpiffeID.TrustDomain()
	podSpiffeIDPath := fmt.Sprintf("/otterize/namespace/%s/service/%s", namespace, serviceName)
	parentSpiffeIDPath := r.parentSpiffeID.Path()

	// Kafka will use certificate's CN to enforce ACL Rules
	commonName := []string{strings.Join([]string{serviceName, namespace}, ".")}

	// Spire uses the first DNS name as CN. CN should be a valid dns name.
	dnsNames = append(commonName, dnsNames...)

	log.Infof("dns_names: %s", dnsNames)

	entry := types.Entry{
		Ttl:      ttl,
		DnsNames: dnsNames,
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
			{Type: "k8s", Value: fmt.Sprintf("pod-label:%s=%s", serviceNameLabel, serviceName)},
		},
	}

	log.Info("Creating SPIRE server entry")
	batchCreateEntryRequest := entryv1.BatchCreateEntryRequest{Entries: []*types.Entry{&entry}}

	resp, err := r.entryClient.BatchCreateEntry(ctx, &batchCreateEntryRequest)
	if err != nil {
		return "", err
	}

	if len(resp.Results) != 1 {
		return "", fmt.Errorf("unexpected number of results returned from SPIRE server, expected exactly 1 and got %d", len(resp.Results))
	}

	result := resp.Results[0]
	switch result.Status.Code {
	case int32(codes.OK):
		log.WithField("entry_id", result.Entry.Id).Info("SPIRE server entry created")
	case int32(codes.AlreadyExists):
		if shouldUpdateEntry(result.Entry, &entry) {
			entry.Id = result.Entry.Id
			id, err := r.updateSpireEntry(ctx, &entry)
			if err != nil {
				return "", err
			}
			log.WithField("entry_id", id).Info("updated spire entry")
			return id, nil
		} else {
			log.WithField("entry_id", result.Entry.Id).Info("SPIRE server entry already exists")
		}
	default:
		return "", fmt.Errorf("entry failed to create with status %s", result.Status)
	}

	return result.Entry.Id, nil
}

func (r *registryImpl) updateSpireEntry(ctx context.Context, entry *types.Entry) (string, error) {
	batchUpdateEntryRequest := entryv1.BatchUpdateEntryRequest{Entries: []*types.Entry{entry}}
	updateResp, err := r.entryClient.BatchUpdateEntry(ctx, &batchUpdateEntryRequest)
	if err != nil {
		return "", fmt.Errorf("entry update failed with error %w", err)
	} else if status := updateResp.Results[0].Status; status.Code != int32(codes.OK) {
		return "", fmt.Errorf("entry update failed with status %s", status)
	}
	return updateResp.Results[0].Entry.Id, nil
}

func shouldUpdateEntry(createResultEntry *types.Entry, desiredEntry *types.Entry) bool {
	return createResultEntry.Ttl != desiredEntry.Ttl || !slices.Equal(createResultEntry.DnsNames, desiredEntry.DnsNames)
}
