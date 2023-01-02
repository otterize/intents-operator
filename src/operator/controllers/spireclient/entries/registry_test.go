package entries

import (
	"context"
	"fmt"
	"github.com/amit7itz/goset"
	"github.com/golang/mock/gomock"
	mock_entryv1 "github.com/otterize/spire-integration-operator/src/mocks/entryv1"
	mock_spireclient "github.com/otterize/spire-integration-operator/src/mocks/spireclient"
	"github.com/samber/lo"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	entryv1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/entry/v1"
	"github.com/spiffe/spire-api-sdk/proto/spire/api/types"
	"github.com/stretchr/testify/suite"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"testing"
)

var (
	trustDomain = spiffeid.RequireTrustDomainFromString("example.org")
)

type RegistrySuite struct {
	suite.Suite
	controller  *gomock.Controller
	spireClient *mock_spireclient.MockServerClient
	entryClient *mock_entryv1.MockEntryClient
	registry    *spireRegistry
}

func (s *RegistrySuite) SetupTest() {
	s.controller = gomock.NewController(s.T())
	s.spireClient = mock_spireclient.NewMockServerClient(s.controller)
	s.entryClient = mock_entryv1.NewMockEntryClient(s.controller)
	clientSpiffeID, err := spiffeid.FromPath(trustDomain, "/client")
	s.Require().NoError(err)
	s.spireClient.EXPECT().GetSpiffeID().Return(clientSpiffeID)
	s.spireClient.EXPECT().NewEntryClient().Return(s.entryClient)
	s.registry = NewSpireRegistry(s.spireClient)
}

func (s *RegistrySuite) TearDownTest() {
	s.spireClient.EXPECT().Close()
	s.spireClient.Close()
}

func (s *RegistrySuite) TestRegistry_RegisterK8SPodEntry() {
	for _, statusCode := range []codes.Code{codes.OK, codes.AlreadyExists} {
		s.Run(fmt.Sprintf("status_code=%d", statusCode), func() {
			namespace := "test-namespace"
			serviceNameLabel := "test/service-name"
			serviceName := "test-service-name"
			spiffeID, err := spiffeid.FromPath(trustDomain, "/otterize/namespace/test-namespace/service/test-service-name")
			s.Require().NoError(err)

			response := entryv1.BatchCreateEntryResponse{
				Results: []*entryv1.BatchCreateEntryResponse_Result{
					{
						Status: &types.Status{Code: int32(statusCode)},
						Entry: &types.Entry{
							Id: "test",
							SpiffeId: &types.SPIFFEID{
								TrustDomain: spiffeID.TrustDomain().String(),
								Path:        spiffeID.Path(),
							},
						},
					},
				},
			}

			s.entryClient.EXPECT().BatchCreateEntry(gomock.Any(), gomock.Any()).Return(&response, nil)

			if statusCode == codes.AlreadyExists {
				updateResponse := entryv1.BatchUpdateEntryResponse{
					Results: []*entryv1.BatchUpdateEntryResponse_Result{
						{
							Status: &types.Status{Code: int32(codes.OK)},
							Entry: &types.Entry{Id: "test",
								SpiffeId: &types.SPIFFEID{TrustDomain: spiffeID.TrustDomain().String(),
									Path: spiffeID.Path(),
								},
							},
						},
					},
				}

				s.entryClient.EXPECT().BatchUpdateEntry(gomock.Any(), gomock.Any()).Return(&updateResponse, nil)
			}

			entryId, err := s.registry.RegisterK8SPod(context.Background(),
				namespace,
				serviceNameLabel,
				serviceName,
				0,
				[]string{})

			s.Require().NoError(err)
			s.Require().Equal(entryId, response.Results[0].Entry.Id)
		})
	}

}

type batchCreateEntryRequestMatcher struct {
	dnsNames *[]string
}

func (m *batchCreateEntryRequestMatcher) Matches(x interface{}) bool {
	request, ok := x.(*entryv1.BatchCreateEntryRequest)
	if !ok {
		return false
	}

	if len(request.Entries) != 1 {
		return false
	}

	if m.dnsNames != nil && !slices.Equal(request.Entries[0].DnsNames, *m.dnsNames) {
		return false
	}

	return true
}

func (m *batchCreateEntryRequestMatcher) String() string {
	return "is a BatchCreateEntryRequest"
}

func (s *RegistrySuite) TestRegistry_RegisterK8SPodEntry_DedupDnsNames() {
	namespace := "test-namespace"
	serviceNameLabel := "test/service-name"
	serviceName := "test-service-name"
	defaultCommonName := fmt.Sprintf("%s.%s", serviceName, namespace)
	extraDnsNames := []string{"hi.com", "test.org"}
	spiffeID, err := spiffeid.FromPath(trustDomain, "/otterize/namespace/test-namespace/service/test-service-name")
	s.Require().NoError(err)

	response := entryv1.BatchCreateEntryResponse{
		Results: []*entryv1.BatchCreateEntryResponse_Result{
			{
				Status: &types.Status{Code: int32(codes.OK)},
				Entry: &types.Entry{
					Id: "test",
					SpiffeId: &types.SPIFFEID{
						TrustDomain: spiffeID.TrustDomain().String(),
						Path:        spiffeID.Path(),
					},
				},
			},
		},
	}

	m := batchCreateEntryRequestMatcher{
		dnsNames: lo.ToPtr(append([]string{defaultCommonName}, extraDnsNames...)),
	}

	s.entryClient.EXPECT().BatchCreateEntry(gomock.Any(), &m).Return(&response, nil)

	// defaultCommonName appears in extraDnsNames input arg and should be deduplicated
	inputDnsNames := append(extraDnsNames, defaultCommonName)
	entryId, err := s.registry.RegisterK8SPod(context.Background(),
		namespace,
		serviceNameLabel,
		serviceName,
		0,
		inputDnsNames)

	s.Require().NoError(err)
	s.Require().Equal(entryId, response.Results[0].Entry.Id)
}

func (s *RegistrySuite) TestShouldUpdateEntry() {
	spiffeID, _ := spiffeid.FromPath(trustDomain, "/otterize/namespace/test-namespace/service/test-service-name")
	entry1 := &types.Entry{
		Id:       "test",
		Ttl:      555,
		DnsNames: []string{"hi.com"},
		SpiffeId: &types.SPIFFEID{
			TrustDomain: spiffeID.TrustDomain().String(),
			Path:        spiffeID.Path(),
		},
	}
	entry2 := &types.Entry{
		Id:       "test",
		Ttl:      555,
		DnsNames: []string{"hi1.com"},
		SpiffeId: &types.SPIFFEID{
			TrustDomain: spiffeID.TrustDomain().String(),
			Path:        spiffeID.Path(),
		},
	}
	entry3 := &types.Entry{
		Id:       "test",
		Ttl:      666,
		DnsNames: []string{"hi.com"},
		SpiffeId: &types.SPIFFEID{
			TrustDomain: spiffeID.TrustDomain().String(),
			Path:        spiffeID.Path(),
		},
	}
	s.Require().True(shouldUpdateEntry(entry1, entry2))
	s.Require().True(shouldUpdateEntry(entry1, entry3))
	s.Require().True(shouldUpdateEntry(entry2, entry3))
}

type batchDeleteEntryRequestMatcher struct {
	entryIDs *[]string
}

func (m *batchDeleteEntryRequestMatcher) Matches(x interface{}) bool {
	request, ok := x.(*entryv1.BatchDeleteEntryRequest)
	if !ok {
		return false
	}

	if len(request.Ids) != 1 {
		return false
	}

	if m.entryIDs != nil && !slices.Equal(request.Ids, *m.entryIDs) {
		return false
	}

	return true
}

func (m *batchDeleteEntryRequestMatcher) String() string {
	return "is a BatchDeleteEntryRequest"
}

func (s *RegistrySuite) TestRegistry_DeleteK8SPodEntry() {
	serviceNameLabel := "test/service-name"
	namespace := "test-namespace"
	serviceName := "test-service-name"
	entryId := "test-entryid"
	spiffeID, err := spiffeid.FromPath(trustDomain, "/otterize/namespace/test-namespace/service/test-service-name")
	s.Require().NoError(err)

	listEntriesResponse := entryv1.ListEntriesResponse{
		Entries: []*types.Entry{
			{
				Id: entryId,
				SpiffeId: &types.SPIFFEID{
					TrustDomain: spiffeID.TrustDomain().String(),
					Path:        spiffeID.Path(),
				},
				Selectors: []*types.Selector{
					{Type: "k8s", Value: fmt.Sprintf("ns:%s", namespace)},
					{Type: "k8s", Value: fmt.Sprintf("pod-label:%s=%s", serviceNameLabel, serviceName)},
				},
			},
		},
	}

	s.entryClient.EXPECT().ListEntries(gomock.Any(), gomock.Any()).Return(&listEntriesResponse, nil)

	deleteEntriesResponse := entryv1.BatchDeleteEntryResponse{
		Results: []*entryv1.BatchDeleteEntryResponse_Result{
			{Id: entryId},
		},
	}

	m := batchDeleteEntryRequestMatcher{
		entryIDs: lo.ToPtr([]string{entryId}),
	}

	s.entryClient.EXPECT().BatchDeleteEntry(gomock.Any(), &m).Return(&deleteEntriesResponse, nil)

	err = s.registry.CleanupOrphanK8SPodEntries(context.Background(),
		serviceNameLabel,
		map[string]*goset.Set[string]{namespace: goset.NewSet[string]()})

	s.Require().NoError(err)
}

func TestRunRegistrySuite(t *testing.T) {
	suite.Run(t, new(RegistrySuite))
}
