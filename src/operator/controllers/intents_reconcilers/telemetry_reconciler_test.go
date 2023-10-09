package intents_reconcilers

import (
	"context"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
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

	clientIntents1 := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: server,
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

	clientIntents1 = otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: anotherServer,
				},
			},
		},
	}

	s.applyIntent(clientIntents1)
	s.Require().Equal(1, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	clientIntents1 = otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: server,
				},
				{
					Name: anotherServer,
				},
			},
		},
	}

	s.applyIntent(clientIntents1)
	s.Require().Equal(2, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	clientIntents2 := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "another-client-name",
			Namespace: testNamespace,
		},

		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: server,
				},
			},
		},
	}

	s.applyIntent(clientIntents2)
	s.Require().Equal(3, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	clientIntents3 := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "another-client-name",
			Namespace: "another-test-namespace",
		},

		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: server,
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

	clientIntents1 := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: server,
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

	clientIntents1 = otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: anotherServer,
					Type: otterizev1alpha2.IntentTypeKafka,
					Topics: []otterizev1alpha2.KafkaTopic{
						{
							Name: "the-topic-I-want-to-talk-about",
							Operations: []otterizev1alpha2.KafkaOperation{
								otterizev1alpha2.KafkaOperationConsume,
								otterizev1alpha2.KafkaOperationProduce,
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

	clientIntents1 = otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: server,
					Type: otterizev1alpha2.IntentTypeKafka,
					Topics: []otterizev1alpha2.KafkaTopic{
						{
							Name: "two-operations-topic",
							Operations: []otterizev1alpha2.KafkaOperation{
								otterizev1alpha2.KafkaOperationConsume,
								otterizev1alpha2.KafkaOperationProduce,
							},
						},
					},
				},
				{
					Name: anotherServer,
					Type: otterizev1alpha2.IntentTypeKafka,
					Topics: []otterizev1alpha2.KafkaTopic{
						{
							Name: "one-operation-topic",
							Operations: []otterizev1alpha2.KafkaOperation{
								otterizev1alpha2.KafkaOperationProduce,
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

	clientIntents2 := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "another-client-name",
			Namespace: testNamespace,
		},

		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: server,
				},
			},
		},
	}

	s.applyIntent(clientIntents2)
	s.Require().Equal(3, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(2, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(0, httpIntentsCount(s.Reconciler))

	clientIntents3 := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "another-client-name",
			Namespace: testNamespace,
		},

		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: server,
					Type: otterizev1alpha2.IntentTypeKafka,
					Topics: []otterizev1alpha2.KafkaTopic{
						{
							Name: "a-topic",
							Operations: []otterizev1alpha2.KafkaOperation{
								otterizev1alpha2.KafkaOperationAll,
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

	clientIntents1 := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: server,
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

	clientIntents1 = otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: anotherServer,
					Type: otterizev1alpha2.IntentTypeHTTP,
					HTTPResources: []otterizev1alpha2.HTTPResource{
						{
							Path: "/the-path-to-success",
							Methods: []otterizev1alpha2.HTTPMethod{
								otterizev1alpha2.HTTPMethodGet,
								otterizev1alpha2.HTTPMethodPost,
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

	clientIntents1 = otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},

		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: server,
					Type: otterizev1alpha2.IntentTypeHTTP,
					HTTPResources: []otterizev1alpha2.HTTPResource{
						{
							Path: "/the-path-to-success",
							Methods: []otterizev1alpha2.HTTPMethod{
								otterizev1alpha2.HTTPMethodGet,
								otterizev1alpha2.HTTPMethodPost,
							},
						},
					},
				},
				{
					Name: anotherServer,
					Type: otterizev1alpha2.IntentTypeHTTP,
					HTTPResources: []otterizev1alpha2.HTTPResource{
						{
							Path: "/the-path-to-success",
							Methods: []otterizev1alpha2.HTTPMethod{
								otterizev1alpha2.HTTPMethodGet,
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

	clientIntents2 := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "another-client-name",
			Namespace: testNamespace,
		},

		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: server,
				},
			},
		},
	}

	s.applyIntent(clientIntents2)
	s.Require().Equal(3, appliedIntentsCount(s.Reconciler))
	s.Require().Equal(0, kafkaIntentsCount(s.Reconciler))
	s.Require().Equal(2, httpIntentsCount(s.Reconciler))

	clientIntents3 := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "another-client-name",
			Namespace: testNamespace,
		},

		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{
				Name: clientName,
			},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: server,
					Type: otterizev1alpha2.IntentTypeHTTP,
					HTTPResources: []otterizev1alpha2.HTTPResource{
						{
							Path: "/the-path-to-success",
							Methods: []otterizev1alpha2.HTTPMethod{
								otterizev1alpha2.HTTPMethodGet,
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

func (s *CountReconcilerTestSuite) applyIntent(clientIntents otterizev1alpha2.ClientIntents) {
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      intentsObjectName,
		},
	}

	emptyIntents := &otterizev1alpha2.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			clientIntents.DeepCopyInto(intents)
			return nil
		})

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Equal(ctrl.Result{}, res)
}

func (s *CountReconcilerTestSuite) removeIntent(clientIntents otterizev1alpha2.ClientIntents) {
	clientIntents.DeletionTimestamp = &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)}
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      intentsObjectName,
		},
	}

	emptyIntents := &otterizev1alpha2.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
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
		count += calls[otterizev1alpha2.IntentTypeKafka]
	}
	return count
}

func httpIntentsCount(reconciler *TelemetryReconciler) int {
	count := 0
	for _, calls := range reconciler.typedIntentsCounter {
		count += calls[otterizev1alpha2.IntentTypeHTTP]
	}
	return count
}

func TestCountReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(CountReconcilerTestSuite))
}
