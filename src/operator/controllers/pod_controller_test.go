package controllers

import (
	"context"
	"github.com/golang/mock/gomock"
	mock_client "github.com/otterize/spire-integration-operator/src/mocks/controller-runtime/client"
	mock_secrets "github.com/otterize/spire-integration-operator/src/mocks/secrets"
	mock_spireclient "github.com/otterize/spire-integration-operator/src/mocks/spireclient"
	mock_entries "github.com/otterize/spire-integration-operator/src/mocks/spireclient/entries"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	s.podReconciler = &PodReconciler{
		Client:          s.client,
		Scheme:          nil,
		SpireClient:     s.spireClient,
		EntriesRegistry: s.entriesRegistry,
		SecretsManager:  s.secretsManager,
	}
}

func (s *PodControllerSuite) TestController_Reconcile() {
	namespace := "test_namespace"
	podname := "test_podname"
	servicename := "test_servicename"
	secretname := "test_secretname"
	entryID := "test"

	s.client.EXPECT().Get(
		gomock.Any(),
		types.NamespacedName{Namespace: namespace, Name: podname},
		gomock.AssignableToTypeOf(&corev1.Pod{}),
	).Return(nil).Do(
		func(ctx context.Context, key client.ObjectKey, pod *corev1.Pod) {
			*pod = corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      podname,
					Annotations: map[string]string{
						ServiceNameAnnotation:   servicename,
						TLSSecretNameAnnotation: secretname,
					},
				},
			}
		})

	// expect update pod labels
	var update *corev1.Pod
	s.client.EXPECT().Update(gomock.Any(), gomock.AssignableToTypeOf(&corev1.Pod{})).
		Return(nil).Do(func(ctx context.Context, pod *corev1.Pod, opts ...client.UpdateOption) {
		update = pod
	})

	// expect spire entry registration
	s.entriesRegistry.EXPECT().RegisterK8SPodEntry(gomock.Any(), namespace, ServiceNameSelectorLabel, servicename, int32(0), nil).
		Return(entryID, nil)

	// expect TLS secret creation
	s.secretsManager.EXPECT().EnsureTLSSecret(gomock.Any(), namespace, secretname, servicename, entryID, gomock.Any(), gomock.Any()).Return(nil)

	request := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: podname}}
	result, err := s.podReconciler.Reconcile(context.Background(), request)
	s.Require().NoError(err)
	s.Require().True(result.IsZero())
	s.Require().Equal(update.Labels[ServiceNameSelectorLabel], servicename)
}

func TestRunPodControllerSuite(t *testing.T) {
	suite.Run(t, new(PodControllerSuite))
}
