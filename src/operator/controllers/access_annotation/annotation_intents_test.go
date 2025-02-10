package access_annotation

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	intentsreconcilersmocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"
)

type AnnotationIntentsTestSuite struct {
	testbase.MocksSuiteBase
	serviceResolver    *intentsreconcilersmocks.MockServiceResolver
	podEventRecorder   *record.FakeRecorder
	injectableRecorder *injectablerecorder.InjectableRecorder
}

func (s *AnnotationIntentsTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.serviceResolver = intentsreconcilersmocks.NewMockServiceResolver(s.Controller)
	s.podEventRecorder = record.NewFakeRecorder(testbase.FakeRecorderBufferSize)
	s.injectableRecorder = &injectablerecorder.InjectableRecorder{}
	s.injectableRecorder.InjectRecorder(s.podEventRecorder)
}

func (s *AnnotationIntentsTestSuite) TearDownTest() {
	s.ExpectNoEvent(s.podEventRecorder)
	s.podEventRecorder = nil
	s.serviceResolver = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *AnnotationIntentsTestSuite) TestGetIntentsForService() {
	podsList := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "server-a-pod-1",
				Namespace: "namespace-a",
				Annotations: map[string]string{
					"intents.otterize.com/called-by": `[{"name": "client-a.namespace-a", "kind": "Deployment"},{"name": "client-b.namespace-b", "kind": "Deployment"}]`,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "server-b-pod-2",
				Namespace: "namespace-a",
				Annotations: map[string]string{
					"intents.otterize.com/called-by": `
[{"name": "client-b.namespace-b", "kind": "Deployment"},
 {"name": "client-c.namespace-b", "kind": "StatefulSet"}]`,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "server-c-pod-1",
				Namespace: "namespace-a",
				Annotations: map[string]string{
					"intents.otterize.com/called-by": `[{"name": "client-d.namespace-a", "kind": "Deployment"}]`,
				},
				DeletionTimestamp: &metav1.Time{Time: time.Date(2024, 4, 25, 19, 30, 0, 0, time.UTC)},
			},
		},
	}

	var emptyPodList corev1.PodList
	s.Client.EXPECT().List(
		gomock.Any(),
		&emptyPodList,
		client.MatchingFields{otterizev1alpha3.OtterizeServerHasAnyCalledByAnnotationIndexField: otterizev1alpha3.OtterizeServerHasAnyCalledByAnnotationValue},
	).DoAndReturn(func(ctx context.Context, podList *corev1.PodList, _ client.ListOption) error {
		podList.Items = podsList
		return nil
	})

	s.serviceResolver.EXPECT().ResolvePodToServiceIdentity(gomock.Any(), &podsList[0]).Return(serviceidentity.ServiceIdentity{Name: "server-a", Namespace: "namespace-a"}, nil)
	s.serviceResolver.EXPECT().ResolvePodToServiceIdentity(gomock.Any(), &podsList[1]).Return(serviceidentity.ServiceIdentity{Name: "server-b", Namespace: "namespace-a"}, nil)
	// We don't expect the third pod to be resolved because it is being deleted

	serverA := serviceidentity.ServiceIdentity{Name: "server-a", Namespace: "namespace-a"}
	serverB := serviceidentity.ServiceIdentity{Name: "server-b", Namespace: "namespace-a"}

	clientA := serviceidentity.ServiceIdentity{Name: "client-a", Namespace: "namespace-a", Kind: "Deployment"}
	clientB := serviceidentity.ServiceIdentity{Name: "client-b", Namespace: "namespace-b", Kind: "Deployment"}
	clientC := serviceidentity.ServiceIdentity{Name: "client-c", Namespace: "namespace-b", Kind: "StatefulSet"}

	annotationIntents, err := GetAllAdditionalAccessFromCluster(context.Background(), s.Client, s.serviceResolver, s.injectableRecorder)
	s.Require().NoError(err)
	s.Require().NotEmpty(annotationIntents)

	keys := lo.Keys(annotationIntents.IntentsByServer)
	s.Require().Len(keys, 2)
	s.Require().Contains(keys, serverA)
	s.Require().Contains(keys, serverB)

	intents, ok := annotationIntents.IntentsByServer[serverA]
	s.Require().True(ok)
	s.Require().Len(intents, 2)
	s.Require().Equal(clientA, intents[0].Client)
	s.Require().Equal(serverA, intents[0].Server)
	s.Require().Equal(clientB, intents[1].Client)
	s.Require().Equal(serverA, intents[1].Server)

	intents, ok = annotationIntents.IntentsByServer[serverB]
	s.Require().True(ok)
	s.Require().Len(intents, 2)
	s.Require().Equal(clientB, intents[0].Client)
	s.Require().Equal(serverB, intents[0].Server)
	s.Require().Equal(clientC, intents[1].Client)
	s.Require().Equal(serverB, intents[1].Server)

	keys = lo.Keys(annotationIntents.IntentsByClient)
	s.Require().Len(keys, 3)
	s.Require().Contains(keys, clientA)
	s.Require().Contains(keys, clientB)
	s.Require().Contains(keys, clientC)

	intents, ok = annotationIntents.IntentsByClient[clientA]
	s.Require().True(ok)
	s.Require().Len(intents, 1)
	s.Require().Equal(clientA, intents[0].Client)
	s.Require().Equal(serverA, intents[0].Server)

	intents, ok = annotationIntents.IntentsByClient[clientB]
	s.Require().True(ok)
	s.Require().Len(intents, 2)
	s.Require().Equal(clientB, intents[0].Client)
	s.Require().Equal(serverA, intents[0].Server)
	s.Require().Equal(clientB, intents[1].Client)
	s.Require().Equal(serverB, intents[1].Server)

	intents, ok = annotationIntents.IntentsByClient[clientC]
	s.Require().True(ok)
	s.Require().Len(intents, 1)
	s.Require().Equal(clientC, intents[0].Client)
	s.Require().Equal(serverB, intents[0].Server)
}

func (s *AnnotationIntentsTestSuite) TestGetIntentsOneHasError() {
	podsList := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "server-a-pod-1",
				Namespace: "namespace-a",
				Annotations: map[string]string{
					"intents.otterize.com/called-by": `[{]`,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "server-b-pod-2",
				Namespace: "namespace-a",
				Annotations: map[string]string{
					"intents.otterize.com/called-by": `[{"name": "client-b.namespace-b", "kind": "Deployment"}]`,
				},
			},
		},
	}

	var emptyPodList corev1.PodList
	s.Client.EXPECT().List(
		gomock.Any(),
		&emptyPodList,
		client.MatchingFields{otterizev1alpha3.OtterizeServerHasAnyCalledByAnnotationIndexField: otterizev1alpha3.OtterizeServerHasAnyCalledByAnnotationValue},
	).DoAndReturn(func(ctx context.Context, podList *corev1.PodList, _ client.ListOption) error {
		podList.Items = podsList
		return nil
	})

	annotationIntents, err := GetAllAdditionalAccessFromCluster(context.Background(), s.Client, s.serviceResolver, s.injectableRecorder)
	s.Require().Error(err)
	s.Require().Empty(annotationIntents)
}

func (s *AnnotationIntentsTestSuite) TestPodEventsRecorder() {
	podsList := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "server-b-pod-2",
				Namespace: "namespace-a",
				Annotations: map[string]string{
					"intents.otterize.com/called-by": `[{"name": "client-b.namespace-b", "kind": "Deployment"}]`,
				},
			},
		},
	}

	var emptyPodList corev1.PodList
	s.Client.EXPECT().List(
		gomock.Any(),
		&emptyPodList,
		client.MatchingFields{otterizev1alpha3.OtterizeServerHasAnyCalledByAnnotationIndexField: otterizev1alpha3.OtterizeServerHasAnyCalledByAnnotationValue},
	).DoAndReturn(func(ctx context.Context, podList *corev1.PodList, _ client.ListOption) error {
		podList.Items = podsList
		return nil
	})

	server := serviceidentity.ServiceIdentity{Name: "server-b", Namespace: "namespace-a"}
	s.serviceResolver.EXPECT().ResolvePodToServiceIdentity(gomock.Any(), &podsList[0]).Return(server, nil)

	annotationIntents, err := GetAllAdditionalAccessFromCluster(context.Background(), s.Client, s.serviceResolver, s.injectableRecorder)
	s.Require().NoError(err)

	intents, ok := annotationIntents.IntentsByServer[server]
	s.Require().True(ok)
	s.Require().Len(intents, 1)
	intent := intents[0]
	intent.EventRecorder.RecordWarningEvent("NetworkPolicyCreationFailed", "Failed to create NetworkPolicy for client")
	s.ExpectEventsForRecorder(s.podEventRecorder, "NetworkPolicyCreationFailed")
}

func (s *AnnotationIntentsTestSuite) TestGetIntentsHasEmptyPods() {
	emptyPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "server-a-pod-1",
			Namespace: "namespace-a",
			Annotations: map[string]string{
				"intents.otterize.com/called-by": `[]`,
			},
		},
	}
	validPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "server-b-pod-2",
			Namespace: "namespace-a",
			Annotations: map[string]string{
				"intents.otterize.com/called-by": `[{"name": "client-a.namespace-a", "kind": "Deployment"}]`,
			},
		},
	}
	deletedPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "server-c-pod-1",
			Namespace:         "namespace-a",
			DeletionTimestamp: &metav1.Time{Time: time.Date(2024, 4, 25, 19, 30, 0, 0, time.UTC)},
			Annotations: map[string]string{
				"intents.otterize.com/called-by": `[{"name": "client-a.namespace-a", "kind": "Deployment"}]`,
			},
		},
	}

	podsList := []corev1.Pod{
		emptyPod,
		validPod,
		deletedPod,
	}

	var emptyPodList corev1.PodList
	s.Client.EXPECT().List(
		gomock.Any(),
		&emptyPodList,
		client.MatchingFields{otterizev1alpha3.OtterizeServerHasAnyCalledByAnnotationIndexField: otterizev1alpha3.OtterizeServerHasAnyCalledByAnnotationValue},
	).DoAndReturn(func(ctx context.Context, podList *corev1.PodList, _ client.ListOption) error {
		podList.Items = podsList
		return nil
	})

	s.serviceResolver.EXPECT().ResolvePodToServiceIdentity(gomock.Any(), &validPod).Return(serviceidentity.ServiceIdentity{Name: "server-a", Namespace: "namespace-a"}, nil)

	annotationIntents, err := GetAllAdditionalAccessFromCluster(context.Background(), s.Client, s.serviceResolver, s.injectableRecorder)
	s.Require().NoError(err)
	s.Require().Len(annotationIntents.IntentsByServer, 1)
	s.Require().Len(annotationIntents.IntentsByClient, 1)
}

func TestAnnotationIntentsTestSuite(t *testing.T) {
	suite.Run(t, new(AnnotationIntentsTestSuite))
}
