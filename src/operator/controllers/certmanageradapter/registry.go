package certmanageradapter

import (
	"context"
	"fmt"
	"github.com/amit7itz/goset"
)

type CertificateEntry struct {
	EntryId     string
	Namespace   string
	ServiceName string
	Ttl         int32
	DnsNames    []string
}

type CertManagerWorkloadRegistry struct {
	entries map[string]*CertificateEntry
}

func NewCertManagerWorkloadRegistry() *CertManagerWorkloadRegistry {
	return &CertManagerWorkloadRegistry{
		entries: make(map[string]*CertificateEntry),
	}
}

func (r *CertManagerWorkloadRegistry) getEntryId(namespace, serviceName string) string {
	return fmt.Sprintf("%s.%s", serviceName, namespace)
}

func (r *CertManagerWorkloadRegistry) RegisterK8SPod(_ context.Context, namespace string, _ string, serviceName string, ttl int32, dnsNames []string) (string, error) {
	entryId := r.getEntryId(namespace, serviceName)
	// Would override entry if id already exists
	r.entries[entryId] = &CertificateEntry{
		EntryId:     entryId,
		Namespace:   namespace,
		ServiceName: serviceName,
		Ttl:         ttl,
		DnsNames:    dnsNames,
	}
	return entryId, nil
}

func (r *CertManagerWorkloadRegistry) CleanupOrphanK8SPodEntries(_ context.Context, _ string, existingServicesByNamespace map[string]*goset.Set[string]) error {
	for entryId, entry := range r.entries {
		nsServices, nsExists := existingServicesByNamespace[entry.Namespace]
		if nsExists {
			if nsServices.Contains(entry.ServiceName) {
				continue
			}
		}
		delete(r.entries, entryId)
	}
	return nil
}

func (r *CertManagerWorkloadRegistry) getPodEntryById(entryId string) *CertificateEntry {
	return r.entries[entryId]
}

func (r *CertManagerWorkloadRegistry) getPodEntry(namespace, serviceName string) *CertificateEntry {
	return r.getPodEntryById(r.getEntryId(namespace, serviceName))
}
