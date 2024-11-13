package intents_reconcilers

import (
	"context"
	otterizev2 "github.com/otterize/intents-operator/src/operator/api/v2"
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

	clientIntents1 := otterizev2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2.IntentsSpec{
			Workload: otterizev2.Workload{
				Name: clientName,
			},
			Targets: []otterizev2.Target{
				{
					Kubernetes: &otterizev2.KubernetesTarget{
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

	clientIntents1 = otterizev2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2.IntentsSpec{
			Workload: otterizev2.Workload{
				Name: clientName,
			},
			Targets: []otterizev2.Target{
				{
					Kubernetes: &otterizev2.KubernetesTarget{
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

	clientIntents1 = otterizev2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2.IntentsSpec{
			Workload: otterizev2.Workload{
				Name: clientName,
			},
			Targets: []otterizev2.Target{
				{
					Kubernetes: &otterizev2.KubernetesTarget{Name: server},
				},
				{
					Kubernetes: &otterizev2.KubernetesTarget{Name: anotherServer},
				},
			},
		},
	}

	s.applyIntent(clientIntents1)
	s.Require().Equal(2, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	clientIntents2 := otterizev2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "another-client-name",
			Namespace: testNamespace,
		},

		Spec: &otterizev2.IntentsSpec{
			Workload: otterizev2.Workload{
				Name: clientName,
			},
			Targets: []otterizev2.Target{
				{
					Kubernetes: &otterizev2.KubernetesTarget{Name: server},
				},
			},
		},
	}

	s.applyIntent(clientIntents2)
	s.Require().Equal(3, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	clientIntents3 := otterizev2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "another-client-name",
			Namespace: "another-test-namespace",
		},

		Spec: &otterizev2.IntentsSpec{
			Workload: otterizev2.Workload{
				Name: clientName,
			},
			Targets: []otterizev2.Target{
				{
					Kubernetes: &otterizev2.KubernetesTarget{Name: server},
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

	clientIntents1 := otterizev2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2.IntentsSpec{
			Workload: otterizev2.Workload{
				Name: clientName,
			},
			Targets: []otterizev2.Target{
				{
					Kubernetes: &otterizev2.KubernetesTarget{Name: server},
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

	clientIntents1 = otterizev2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2.IntentsSpec{
			Workload: otterizev2.Workload{
				Name: clientName,
			},
			Targets: []otterizev2.Target{
				{
					Kafka: &otterizev2.KafkaTarget{
						Name: anotherServer,
						Topics: []otterizev2.KafkaTopic{
							{
								Name: "the-topic-I-want-to-talk-about",
								Operations: []otterizev2.KafkaOperation{
									otterizev2.KafkaOperationConsume,
									otterizev2.KafkaOperationProduce,
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

	clientIntents1 = otterizev2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2.IntentsSpec{
			Workload: otterizev2.Workload{
				Name: clientName,
			},
			Targets: []otterizev2.Target{
				{
					Kafka: &otterizev2.KafkaTarget{
						Name: server,
						Topics: []otterizev2.KafkaTopic{
							{
								Name: "two-operations-topic",
								Operations: []otterizev2.KafkaOperation{
									otterizev2.KafkaOperationConsume,
									otterizev2.KafkaOperationProduce,
								},
							},
						},
					},
				},
				{
					Kafka: &otterizev2.KafkaTarget{
						Name: anotherServer,
						Topics: []otterizev2.KafkaTopic{
							{
								Name: "one-operation-topic",
								Operations: []otterizev2.KafkaOperation{
									otterizev2.KafkaOperationProduce,
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

	clientIntents2 := otterizev2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "another-client-name",
			Namespace: testNamespace,
		},

		Spec: &otterizev2.IntentsSpec{
			Workload: otterizev2.Workload{
				Name: clientName,
			},
			Targets: []otterizev2.Target{
				{
					Kubernetes: &otterizev2.KubernetesTarget{Name: server},
				},
			},
		},
	}

	s.applyIntent(clientIntents2)
	s.Require().Equal(3, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(2, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	clientIntents3 := otterizev2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "another-client-name",
			Namespace: testNamespace,
		},

		Spec: &otterizev2.IntentsSpec{
			Workload: otterizev2.Workload{
				Name: clientName,
			},
			Targets: []otterizev2.Target{
				{
					Kafka: &otterizev2.KafkaTarget{
						Name: server,
						Topics: []otterizev2.KafkaTopic{
							{
								Name: "a-topic",
								Operations: []otterizev2.KafkaOperation{
									otterizev2.KafkaOperationAll,
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

	clientIntents1 := otterizev2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2.IntentsSpec{
			Workload: otterizev2.Workload{
				Name: clientName,
			},
			Targets: []otterizev2.Target{
				{
					Kubernetes: &otterizev2.KubernetesTarget{Name: server},
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

	clientIntents1 = otterizev2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2.IntentsSpec{
			Workload: otterizev2.Workload{
				Name: clientName,
			},
			Targets: []otterizev2.Target{
				{
					Kubernetes: &otterizev2.KubernetesTarget{
						Name: anotherServer,
						HTTP: []otterizev2.HTTPTarget{
							{
								Path: "/the-path-to-success",
								Methods: []otterizev2.HTTPMethod{
									otterizev2.HTTPMethodGet,
									otterizev2.HTTPMethodPost,
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

	clientIntents1 = otterizev2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev2.IntentsSpec{
			Workload: otterizev2.Workload{
				Name: clientName,
			},
			Targets: []otterizev2.Target{
				{
					Kubernetes: &otterizev2.KubernetesTarget{
						Name: server,
						HTTP: []otterizev2.HTTPTarget{
							{
								Path: "/the-path-to-success",
								Methods: []otterizev2.HTTPMethod{
									otterizev2.HTTPMethodGet,
									otterizev2.HTTPMethodPost,
								},
							},
						},
					},
				},
				{
					Kubernetes: &otterizev2.KubernetesTarget{
						Name: anotherServer,
						HTTP: []otterizev2.HTTPTarget{
							{
								Path: "/the-path-to-success",
								Methods: []otterizev2.HTTPMethod{
									otterizev2.HTTPMethodGet,
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

	clientIntents2 := otterizev2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "another-client-name",
			Namespace: testNamespace,
		},

		Spec: &otterizev2.IntentsSpec{
			Workload: otterizev2.Workload{
				Name: clientName,
			},
			Targets: []otterizev2.Target{
				{
					Kubernetes: &otterizev2.KubernetesTarget{Name: server},
				},
			},
		},
	}

	s.applyIntent(clientIntents2)
	s.Require().Equal(3, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(2, httpIntentsCount(s.Reconciler))

	clientIntents3 := otterizev2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "another-client-name",
			Namespace: testNamespace,
		},

		Spec: &otterizev2.IntentsSpec{
			Workload: otterizev2.Workload{
				Name: clientName,
			},
			Targets: []otterizev2.Target{
				{
					Kubernetes: &otterizev2.KubernetesTarget{
						Name: server,
						HTTP: []otterizev2.HTTPTarget{
							{
								Path: "/the-path-to-success",
								Methods: []otterizev2.HTTPMethod{
									otterizev2.HTTPMethodGet,
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

func (s *CountReconcilerTestSuite) applyIntent(clientIntents otterizev2.ClientIntents) {
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      intentsObjectName,
		},
	}

	emptyIntents := &otterizev2.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev2.ClientIntents, options ...client.ListOption) error {
			clientIntents.DeepCopyInto(intents)
			return nil
		})

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Equal(ctrl.Result{}, res)
}

func (s *CountReconcilerTestSuite) removeIntent(clientIntents otterizev2.ClientIntents) {
	clientIntents.DeletionTimestamp = &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)}
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      intentsObjectName,
		},
	}

	emptyIntents := &otterizev2.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev2.ClientIntents, options ...client.ListOption) error {
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
		count += calls[otterizev2.IntentTypeKafka]
	}
	return count
}

func httpIntentsCount(reconciler *TelemetryReconciler) int {
	count := 0
	for _, calls := range reconciler.typedIntentsCounter {
		count += calls[otterizev2.IntentTypeHTTP]
	}
	return count
}

func TestCountReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(CountReconcilerTestSuite))
}
