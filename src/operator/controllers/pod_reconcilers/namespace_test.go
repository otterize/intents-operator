package pod_reconcilers

import (
	"context"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

type NamespaceWatcherTestSuite struct {
	testbase.MocksSuiteBase
	reconciler *NamespaceWatcher
}

func (s *NamespaceWatcherTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.reconciler = NewNamespaceWatcher(s.Client)
}

func (s *NamespaceWatcherTestSuite) TestAddLabelToNewNamespace() {
	nsName := "test-namespace"
	namespaceWithoutLabels := v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	}

	emptyNs := v1.Namespace{}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: nsName,
		},
	}

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, &emptyNs).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, namespace *v1.Namespace, opts ...client.GetOption) error {
			namespaceWithoutLabels.DeepCopyInto(namespace)
			return nil
		})

	nsWithLabel := namespaceWithoutLabels.DeepCopy()
	nsWithLabel.Labels = map[string]string{
		"kubernetes.io/metadata.name": nsName,
	}

	s.Client.EXPECT().Patch(gomock.Any(), gomock.Eq(nsWithLabel), intents_reconcilers.MatchMergeFromPatch(client.StrategicMergeFrom(&namespaceWithoutLabels), nsWithLabel)).Return(nil)

	res, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Empty(res)
}

func (s *NamespaceWatcherTestSuite) TestNothingToDoIfLabelExists() {
	nsName := "test-namespace"
	namespaceWithLabel := v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
			Labels: map[string]string{
				"kubernetes.io/metadata.name": nsName,
			},
		},
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: nsName,
		},
	}

	emptyNs := v1.Namespace{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, &emptyNs).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, namespace *v1.Namespace, opts ...client.GetOption) error {
			namespaceWithLabel.DeepCopyInto(namespace)
			return nil
		})

	res, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Empty(res)
}

func (s *NamespaceWatcherTestSuite) TestOtherLabelsExists() {
	nsName := "test-namespace"
	namespaceWithOtherLabels := v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
			Labels: map[string]string{
				"my-app": "my-rules",
			},
		},
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: nsName,
		},
	}

	emptyNs := v1.Namespace{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, &emptyNs).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, namespace *v1.Namespace, opts ...client.GetOption) error {
			namespaceWithOtherLabels.DeepCopyInto(namespace)
			return nil
		})

	nsWithLabel := namespaceWithOtherLabels.DeepCopy()
	nsWithLabel.Labels = map[string]string{
		"my-app":                      "my-rules",
		"kubernetes.io/metadata.name": nsName,
	}

	s.Client.EXPECT().Patch(gomock.Any(), gomock.Eq(nsWithLabel), intents_reconcilers.MatchMergeFromPatch(client.StrategicMergeFrom(&namespaceWithOtherLabels), nsWithLabel)).Return(nil)

	res, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Empty(res)
}

func TestNamespaceWatcherSuite(t *testing.T) {
	suite.Run(t, new(NamespaceWatcherTestSuite))
}
