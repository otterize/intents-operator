package controllers

import (
	"context"
	"fmt"
	"github.com/amit7itz/goset"
	"github.com/golang/mock/gomock"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/spire-integration-operator/src/controllers/metadata"
	"github.com/otterize/spire-integration-operator/src/controllers/secrets/types"
	"github.com/otterize/spire-integration-operator/src/mocks/controller-runtime/client"
	mock_secrets "github.com/otterize/spire-integration-operator/src/mocks/controllers/secrets"
	mock_record "github.com/otterize/spire-integration-operator/src/mocks/eventrecorder"
	mock_spireclient "github.com/otterize/spire-integration-operator/src/mocks/spireclient"
	mock_entries "github.com/otterize/spire-integration-operator/src/mocks/spireclient/entries"
	"github.com/stretchr/testify/suite"
	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"testing"
)

type PodControllerSuite struct {
	suite.Suite
	controller      *gomock.Controller
	client          *mock_client.MockClient
	spireClient     *mock_spireclient.MockServerClient
	entriesRegistry *mock_entries.MockRegistry
	secretsManager  *mock_secrets.MockManager
	podReconciler   *PodReconciler
}

func (s *PodControllerSuite) SetupTest() {
	s.controller = gomock.NewController(s.T())
	s.client = mock_client.NewMockClient(s.controller)
	s.spireClient = mock_spireclient.NewMockServerClient(s.controller)
	s.entriesRegistry = mock_entries.NewMockRegistry(s.controller)
	s.secretsManager = mock_secrets.NewMockManager(s.controller)
	serviceIdResolver := serviceidresolver.NewResolver(s.client)
	eventRecorder := mock_record.NewMockEventRecorder(s.controller)
	eventRecorder.EXPECT().Event(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	eventRecorder.EXPECT().Eventf(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	s.client.EXPECT().Scheme().Return(scheme).AnyTimes()
	s.podReconciler = NewPodReconciler(s.client, nil, s.entriesRegistry, s.secretsManager,
		serviceIdResolver, eventRecorder)
}

type ObjectNameMatcher struct {
	name      string
	namespace string
}

func (m *ObjectNameMatcher) Matches(x interface{}) bool {
	obj, ok := x.(metav1.Object)
	if !ok {
		return false
	}

	return obj.GetName() == m.name && obj.GetNamespace() == m.namespace
}

func (m *ObjectNameMatcher) String() string {
	return fmt.Sprintf("%T(name=%s, namespace=%s)", m, m.name, m.namespace)
}

func (s *PodControllerSuite) TestController_Reconcile() {
	namespace := "test_namespace"
	podname := "test_podname"
	servicename := "test_servicename"
	secretname := "test_secretname"
	entryID := "test"
	ttl := int32(99999)
	extraDnsNames := []string{"asd.com", "bla.org"}
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      podname,
			Annotations: map[string]string{
				serviceidresolver.ServiceNameAnnotation: servicename,
				metadata.TLSSecretNameAnnotation:        secretname,
				metadata.CertTTLAnnotation:              fmt.Sprintf("%d", ttl),
				metadata.DNSNamesAnnotation:             strings.Join(extraDnsNames, ","),
			},
		},
	}

	s.client.EXPECT().Get(
		gomock.Any(),
		types.NamespacedName{Namespace: namespace, Name: podname},
		gomock.AssignableToTypeOf(&corev1.Pod{}),
	).Return(nil).Do(
		func(ctx context.Context, key client.ObjectKey, returnedPod *corev1.Pod) {
			*returnedPod = pod
		})

	// expect update pod labels
	var update *corev1.Pod
	s.client.EXPECT().Update(gomock.Any(), gomock.AssignableToTypeOf(&corev1.Pod{})).
		Return(nil).Do(func(ctx context.Context, pod *corev1.Pod, opts ...client.UpdateOption) {
		update = pod
	})

	// expect spire entry registration
	s.entriesRegistry.EXPECT().RegisterK8SPodEntry(gomock.Any(), namespace, metadata.RegisteredServiceNameLabel, servicename, ttl, extraDnsNames).
		Return(entryID, nil)

	// expect TLS secret creation
	entryHash, err := getEntryHash(namespace, servicename, ttl, extraDnsNames)
	s.NoError(err)
	certConf, err := certConfigFromPod(&pod)
	s.NoError(err)
	s.secretsManager.EXPECT().EnsureTLSSecret(
		gomock.Any(),
		secretstypes.NewSecretConfig(
			entryID,
			entryHash,
			secretname,
			namespace,
			servicename,
			certConf,
			false,
		),
		&ObjectNameMatcher{name: podname, namespace: namespace}).Return(nil)

	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: podname}}
	result, err := s.podReconciler.Reconcile(context.Background(), request)
	s.Require().NoError(err)
	s.Require().True(result.IsZero())
	s.Require().Equal(update.Labels[metadata.RegisteredServiceNameLabel], servicename)
}

func (s *PodControllerSuite) TestController_cleanupOrphanEntries() {
	existingPods := corev1.PodList{
		Items: []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace1",
					Name:      "pod1-1",
					Labels: map[string]string{
						metadata.RegisteredServiceNameLabel: "service1",
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace1",
					Name:      "pod1-2",
					Labels: map[string]string{
						metadata.RegisteredServiceNameLabel: "service2",
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace2",
					Name:      "pod2-1",
					Labels: map[string]string{
						metadata.RegisteredServiceNameLabel: "service1",
					},
				},
			},
		},
	}

	s.client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&corev1.PodList{}),
		client.HasLabels{metadata.RegisteredServiceNameLabel},
	).Return(nil).Do(
		func(ctx context.Context, returnedPods *corev1.PodList, opts ...client.ListOption) {
			*returnedPods = existingPods
		})

	var receivedExistingServicesByNamespace map[string]*goset.Set[string]

	s.entriesRegistry.EXPECT().CleanupOrphanK8SPodEntries(
		gomock.Any(),
		metadata.RegisteredServiceNameLabel,
		gomock.Any(),
	).Do(func(ctx context.Context, serviceNameLabel string, existingServicesByNamespace map[string]*goset.Set[string]) error {
		receivedExistingServicesByNamespace = existingServicesByNamespace
		return nil
	})

	err := s.podReconciler.cleanupOrphanEntries(context.Background())
	s.Require().NoError(err)
	expectedExistingServicesByNamespace := map[string]*goset.Set[string]{
		"namespace1": goset.NewSet[string]("service1", "service2"),
		"namespace2": goset.NewSet[string]("service1"),
	}
	s.Require().ElementsMatch(maps.Keys(receivedExistingServicesByNamespace), maps.Keys(expectedExistingServicesByNamespace))
	for namespace, services := range expectedExistingServicesByNamespace {
		s.Require().ElementsMatch(services.Items(), receivedExistingServicesByNamespace[namespace].Items())
	}
}

func TestRunPodControllerSuite(t *testing.T) {
	suite.Run(t, new(PodControllerSuite))
}
