package entries

import (
	"context"
	"fmt"
	"github.com/amit7itz/goset"
	"github.com/otterize/credentials-operator/src/controllers/spireclient"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	entryv1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/entry/v1"
	"github.com/spiffe/spire-api-sdk/proto/spire/api/types"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"strings"
)

type spireRegistry struct {
	parentSpiffeID spiffeid.ID
	entryClient    entryv1.EntryClient
}

func NewSpireRegistry(spireClient spireclient.ServerClient) *spireRegistry {
	return &spireRegistry{
		parentSpiffeID: spireClient.GetSpiffeID(),
		entryClient:    spireClient.NewEntryClient(),
	}
}

func (r *spireRegistry) RegisterK8SPod(ctx context.Context, namespace string, serviceNameLabel string, serviceName string, ttl int32, extraDnsNames []string) (string, error) {
	log := logrus.WithFields(logrus.Fields{"namespace": namespace, "service_name": serviceName})

	trustDomain := r.parentSpiffeID.TrustDomain()
	podSpiffeIDPath := fmt.Sprintf("/otterize/namespace/%s/service/%s", namespace, serviceName)
	parentSpiffeIDPath := r.parentSpiffeID.Path()

	// Spire uses the first DNS name as CN. CN should be a valid dns name.
	commonName := strings.Join([]string{serviceName, namespace}, ".")
	i := slices.Index(extraDnsNames, commonName)
	if i != -1 {
		// Remove common name from extra DNS names to make sure it doesn't appear as duplicate
		extraDnsNames = slices.Delete(extraDnsNames, i, i+1)
	}

	dnsNames := append([]string{commonName}, extraDnsNames...)

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
		return "", errors.Wrap(err)
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
				return "", errors.Wrap(err)
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

func (r *spireRegistry) updateSpireEntry(ctx context.Context, entry *types.Entry) (string, error) {
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

func (r *spireRegistry) paginatedListEntries(ctx context.Context, pageToken string) ([]*types.Entry, string, error) {
	trustDomain := r.parentSpiffeID.TrustDomain()
	parentSpiffeIDPath := r.parentSpiffeID.Path()

	listEntriesRequest := entryv1.ListEntriesRequest{
		PageToken: pageToken,
		Filter: &entryv1.ListEntriesRequest_Filter{
			ByParentId: &types.SPIFFEID{
				TrustDomain: trustDomain.String(),
				Path:        parentSpiffeIDPath,
			},
		},
	}

	listResp, err := r.entryClient.ListEntries(ctx, &listEntriesRequest)
	if err != nil {
		return nil, "", fmt.Errorf("list entries failed with error %w", err)
	}

	return listResp.Entries, listResp.NextPageToken, nil
}

func extractServiceFromEntry(entry *types.Entry, serviceNameLabel string) (namespace string, serviceName string, ok bool) {
	for _, selector := range entry.Selectors {
		if selector.Type != "k8s" {
			return "", "", false
		}

		valueKey, valueValue, found := strings.Cut(selector.Value, ":")
		if !found {
			return "", "", false
		}
		switch valueKey {
		case "ns":
			namespace = valueValue
		case "pod-label":
			labelKey, labelValue, labelSepFound := strings.Cut(valueValue, "=")
			if !labelSepFound || labelKey != serviceNameLabel {
				return "", "", false
			}
			serviceName = labelValue
		default:
			return "", "", false
		}
	}

	return namespace, serviceName, true
}

func (r *spireRegistry) deleteEntries(ctx context.Context, entryIDs []string) error {
	batchDeleteEntriesRequest := entryv1.BatchDeleteEntryRequest{
		Ids: entryIDs,
	}

	deleteResp, err := r.entryClient.BatchDeleteEntry(ctx, &batchDeleteEntriesRequest)
	if err != nil {
		return fmt.Errorf("entry delete failed with error %w", err)
	}

	errStatuses := lo.Filter(deleteResp.Results, func(res *entryv1.BatchDeleteEntryResponse_Result, _ int) bool {
		if res.Status == nil {
			return false
		}

		switch res.Status.Code {
		case int32(codes.OK), int32(codes.NotFound):
			return false
		default:
			return true
		}
	})

	if len(errStatuses) != 0 {
		return fmt.Errorf("some entries failed to delete failed with status: %v", errStatuses)
	}

	return nil
}

func (r *spireRegistry) CleanupOrphanK8SPodEntries(ctx context.Context, serviceNameLabel string, existingServicesByNamespace map[string]*goset.Set[string]) error {
	log := logrus.StandardLogger()
	pageToken := ""
	pages := 0

	entryIDsToDelete := make([]string, 0)

	for pages == 0 || pageToken != "" {
		log.Debugf("Iterating over paginated list request, page number %d", pages+1)
		entries, nextPageToken, err := r.paginatedListEntries(ctx, pageToken)
		if err != nil {
			return errors.Wrap(err)
		}
		pages++
		pageToken = nextPageToken

		entriesToDelete := lo.Filter(entries, func(entry *types.Entry, _ int) bool {
			namespace, serviceName, ok := extractServiceFromEntry(entry, serviceNameLabel)
			if !ok {
				// entry's selectors are not as expected
				return true
			}

			servicesInNamespace, ok := existingServicesByNamespace[namespace]
			if !ok {
				// entry's namespace is not in existing namespaces
				return true
			}

			if !servicesInNamespace.Contains(serviceName) {
				// entry's service not in services in this namespace
				return true
			}

			return false
		})

		entryIDsToDelete = append(
			entryIDsToDelete,
			lo.Map(entriesToDelete, func(e *types.Entry, _ int) string { return e.Id })...,
		)
	}

	if len(entryIDsToDelete) == 0 {
		log.Info("No orphan entries to delete")
		return nil
	}

	log.Infof("Deleting %d orphan entries", len(entryIDsToDelete))
	if err := r.deleteEntries(ctx, entryIDsToDelete); err != nil {
		return errors.Wrap(err)
	}

	return nil
}
