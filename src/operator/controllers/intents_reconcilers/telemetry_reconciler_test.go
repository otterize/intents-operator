package intents_reconcilers

import (
	"context"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"
)

type CountReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	Reconciler *TelemetryReconciler
}

func (s *CountReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()

	s.Reconciler = NewTelemetryReconciler(
		s.Client,
		&runtime.Scheme{},
	)
	s.Reconciler.Recorder = s.Recorder
}

func (s *CountReconcilerTestSuite) TearDownTest() {
	s.Reconciler = nil
}

func (s *CountReconcilerTestSuite) TestAppliedIntentsTelemetries() {
	server := "test-server"
	anotherServer := "another-test-server"

	clientIntents1 := otterizev2alpha1.ApprovedClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{
						Name: server,
					},
				},
			},
		},
	}

	s.applyIntent(clientIntents1)
	s.Require().Equal(1, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	s.applyIntent(clientIntents1)
	s.Require().Equal(1, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	clientIntents1 = otterizev2alpha1.ApprovedClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{
						Name: anotherServer,
					},
				},
			},
		},
	}

	s.applyIntent(clientIntents1)
	s.Require().Equal(1, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	clientIntents1 = otterizev2alpha1.ApprovedClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: server},
				},
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: anotherServer},
				},
			},
		},
	}

	s.applyIntent(clientIntents1)
	s.Require().Equal(2, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	clientIntents2 := otterizev2alpha1.ApprovedClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "another-client-name",
			Namespace: testNamespace,
		},

		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: server},
				},
			},
		},
	}

	s.applyIntent(clientIntents2)
	s.Require().Equal(3, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	clientIntents3 := otterizev2alpha1.ApprovedClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "another-client-name",
			Namespace: "another-test-namespace",
		},

		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: server},
				},
			},
		},
	}

	s.applyIntent(clientIntents3)
	s.Require().Equal(4, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	s.removeIntent(clientIntents1)
	s.Require().Equal(2, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))
}

func (s *CountReconcilerTestSuite) TestKafkaIntentsTelemetries() {
	server := "test-server"
	anotherServer := "another-test-server"

	clientIntents1 := otterizev2alpha1.ApprovedClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: server},
				},
			},
		},
	}

	s.applyIntent(clientIntents1)
	s.Require().Equal(1, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	s.applyIntent(clientIntents1)
	s.Require().Equal(1, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	clientIntents1 = otterizev2alpha1.ApprovedClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kafka: &otterizev2alpha1.KafkaTarget{
						Name: anotherServer,
						Topics: []otterizev2alpha1.KafkaTopic{
							{
								Name: "the-topic-I-want-to-talk-about",
								Operations: []otterizev2alpha1.KafkaOperation{
									otterizev2alpha1.KafkaOperationConsume,
									otterizev2alpha1.KafkaOperationProduce,
								},
							},
						},
					},
				},
			},
		},
	}

	s.applyIntent(clientIntents1)
	s.Require().Equal(1, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(1, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	clientIntents1 = otterizev2alpha1.ApprovedClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kafka: &otterizev2alpha1.KafkaTarget{
						Name: server,
						Topics: []otterizev2alpha1.KafkaTopic{
							{
								Name: "two-operations-topic",
								Operations: []otterizev2alpha1.KafkaOperation{
									otterizev2alpha1.KafkaOperationConsume,
									otterizev2alpha1.KafkaOperationProduce,
								},
							},
						},
					},
				},
				{
					Kafka: &otterizev2alpha1.KafkaTarget{
						Name: anotherServer,
						Topics: []otterizev2alpha1.KafkaTopic{
							{
								Name: "one-operation-topic",
								Operations: []otterizev2alpha1.KafkaOperation{
									otterizev2alpha1.KafkaOperationProduce,
								},
							},
						},
					},
				},
			},
		},
	}

	s.applyIntent(clientIntents1)
	s.Require().Equal(2, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(2, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	clientIntents2 := otterizev2alpha1.ApprovedClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "another-client-name",
			Namespace: testNamespace,
		},

		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: server},
				},
			},
		},
	}

	s.applyIntent(clientIntents2)
	s.Require().Equal(3, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(2, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	clientIntents3 := otterizev2alpha1.ApprovedClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "another-client-name",
			Namespace: testNamespace,
		},

		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kafka: &otterizev2alpha1.KafkaTarget{
						Name: server,
						Topics: []otterizev2alpha1.KafkaTopic{
							{
								Name: "a-topic",
								Operations: []otterizev2alpha1.KafkaOperation{
									otterizev2alpha1.KafkaOperationAll,
								},
							},
						},
					},
				},
			},
		},
	}

	s.applyIntent(clientIntents3)
	s.Require().Equal(3, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(3, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	s.removeIntent(clientIntents1)
	s.Require().Equal(1, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(1, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))
}

func (s *CountReconcilerTestSuite) TestHTTPIntentsTelemetries() {
	server := "test-server"
	anotherServer := "another-test-server"

	clientIntents1 := otterizev2alpha1.ApprovedClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: server},
				},
			},
		},
	}

	s.applyIntent(clientIntents1)
	s.Require().Equal(1, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	s.applyIntent(clientIntents1)
	s.Require().Equal(1, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	clientIntents1 = otterizev2alpha1.ApprovedClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{
						Name: anotherServer,
						HTTP: []otterizev2alpha1.HTTPTarget{
							{
								Path: "/the-path-to-success",
								Methods: []otterizev2alpha1.HTTPMethod{
									otterizev2alpha1.HTTPMethodGet,
									otterizev2alpha1.HTTPMethodPost,
								},
							},
						},
					},
				},
			},
		},
	}

	s.applyIntent(clientIntents1)
	s.Require().Equal(1, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(1, httpIntentsCount(s.Reconciler))

	clientIntents1 = otterizev2alpha1.ApprovedClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{
						Name: server,
						HTTP: []otterizev2alpha1.HTTPTarget{
							{
								Path: "/the-path-to-success",
								Methods: []otterizev2alpha1.HTTPMethod{
									otterizev2alpha1.HTTPMethodGet,
									otterizev2alpha1.HTTPMethodPost,
								},
							},
						},
					},
				},
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{
						Name: anotherServer,
						HTTP: []otterizev2alpha1.HTTPTarget{
							{
								Path: "/the-path-to-success",
								Methods: []otterizev2alpha1.HTTPMethod{
									otterizev2alpha1.HTTPMethodGet,
								},
							},
						},
					},
				},
			},
		},
	}

	s.applyIntent(clientIntents1)
	s.Require().Equal(2, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(2, httpIntentsCount(s.Reconciler))

	clientIntents2 := otterizev2alpha1.ApprovedClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "another-client-name",
			Namespace: testNamespace,
		},

		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: server},
				},
			},
		},
	}

	s.applyIntent(clientIntents2)
	s.Require().Equal(3, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(2, httpIntentsCount(s.Reconciler))

	clientIntents3 := otterizev2alpha1.ApprovedClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "another-client-name",
			Namespace: testNamespace,
		},

		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{
				Name: clientName,
			},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{
						Name: server,
						HTTP: []otterizev2alpha1.HTTPTarget{
							{
								Path: "/the-path-to-success",
								Methods: []otterizev2alpha1.HTTPMethod{
									otterizev2alpha1.HTTPMethodGet,
								},
							},
						},
					},
				},
			},
		},
	}

	s.applyIntent(clientIntents3)
	s.Require().Equal(3, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(3, httpIntentsCount(s.Reconciler))

	s.removeIntent(clientIntents1)
	s.Require().Equal(1, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(1, httpIntentsCount(s.Reconciler))
}

func (s *CountReconcilerTestSuite) applyIntent(clientIntents otterizev2alpha1.ApprovedClientIntents) {
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      intentsObjectName,
		},
	}

	emptyIntents := &otterizev2alpha1.ApprovedClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev2alpha1.ApprovedClientIntents, options ...client.ListOption) error {
			clientIntents.DeepCopyInto(intents)
			return nil
		})

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Equal(ctrl.Result{}, res)
}

func (s *CountReconcilerTestSuite) removeIntent(clientIntents otterizev2alpha1.ApprovedClientIntents) {
	clientIntents.DeletionTimestamp = &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)}
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      intentsObjectName,
		},
	}

	emptyIntents := &otterizev2alpha1.ApprovedClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev2alpha1.ApprovedClientIntents, options ...client.ListOption) error {
			clientIntents.DeepCopyInto(intents)
			return nil
		})

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Equal(ctrl.Result{}, res)
}

func appliedIntentsCount(reconciler *TelemetryReconciler) int {
	count := 0
	for _, calls := range reconciler.intentsCounter {
		count += calls
	}
	return count
}

func kafkaIntentsCount(reconciler *TelemetryReconciler) int {
	count := 0
	for _, calls := range reconciler.typedIntentsCounter {
		count += calls[otterizev2alpha1.IntentTypeKafka]
	}
	return count
}

func httpIntentsCount(reconciler *TelemetryReconciler) int {
	count := 0
	for _, calls := range reconciler.typedIntentsCounter {
		count += calls[otterizev2alpha1.IntentTypeHTTP]
	}
	return count
}

func TestCountReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(CountReconcilerTestSuite))
}
