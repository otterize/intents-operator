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

// Run tests for namespace reconciler using gomock and testify suite
// This is the reconciler code:
// package pod_reconcilers
//
//import (
//	"context"
//	"fmt"
//	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
//	"github.com/otterize/intents-operator/src/shared/errors"
//	"github.com/samber/lo"
//	"github.com/sirupsen/logrus"
//	v1 "k8s.io/api/core/v1"
//	k8serrors "k8s.io/apimachinery/pkg/api/errors"
//	ctrl "sigs.k8s.io/controller-runtime"
//	"sigs.k8s.io/controller-runtime/pkg/client"
//	"sigs.k8s.io/controller-runtime/pkg/controller"
//	"sigs.k8s.io/controller-runtime/pkg/handler"
//	"sigs.k8s.io/controller-runtime/pkg/manager"
//	"sigs.k8s.io/controller-runtime/pkg/source"
//)
//
////+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;update;patch;list;watch
//
//type NamespaceWatcher struct {
//	client.Client
//}
//
//func NewNamespaceWatcher(c client.Client) *NamespaceWatcher {
//	return &NamespaceWatcher{c}
//}
//
//func (ns *NamespaceWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
//	logrus.Infof("Reconciling due to namespace change: %s", req.Name)
//	namespace := &v1.Namespace{}
//	err := ns.Get(ctx, req.NamespacedName, namespace)
//	if k8serrors.IsNotFound(err) {
//		return ctrl.Result{}, nil
//	}
//
//	if err != nil {
//		return ctrl.Result{}, errors.Wrap(err)
//	}
//
//	if ns.hasKubernetesNameLabel(namespace) {
//		return ctrl.Result{}, nil
//	}
//
//	// Add Kubernetes standard namespace label so this namespace is a viable selector in network policies
//	updatedNS := namespace.DeepCopy()
//	if updatedNS.Labels == nil {
//		updatedNS.Labels = make(map[string]string)
//	}
//	updatedNS.Labels[otterizev1alpha3.KubernetesStandardNamespaceNameLabelKey] = req.Name
//	err = ns.Patch(ctx, updatedNS, client.MergeFrom(namespace))
//	if err != nil {
//		return ctrl.Result{}, errors.Wrap(err)
//	}
//	return ctrl.Result{}, nil
//
//}
//
//func (ns *NamespaceWatcher) hasKubernetesNameLabel(namespace *v1.Namespace) bool {
//	_, exists := namespace.Labels[otterizev1alpha3.KubernetesStandardNamespaceNameLabelKey]
//	return exists
//}
//
//func (ns *NamespaceWatcher) Register(mgr manager.Manager) error {
//	watcher, err := controller.New("namespace-watcher", mgr, controller.Options{
//		Reconciler:   ns,
//		RecoverPanic: lo.ToPtr(true),
//	})
//	if err != nil {
//		return fmt.Errorf("unable to set up namespace controller: %w", err)
//	}
//
//	if err = watcher.Watch(source.Kind(mgr.GetCache(), &v1.Namespace{}), &handler.EnqueueRequestForObject{}); err != nil {
//		return fmt.Errorf("unable to watch Namespaces: %w", err)
//	}
//
//	return nil
//}

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

	s.Client.EXPECT().Patch(gomock.Any(), gomock.Eq(nsWithLabel), intents_reconcilers.MatchPatch(client.MergeFrom(&namespaceWithoutLabels))).Return(nil)

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

	s.Client.EXPECT().Patch(gomock.Any(), gomock.Eq(nsWithLabel), intents_reconcilers.MatchPatch(client.MergeFrom(&namespaceWithOtherLabels))).Return(nil)

	res, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Empty(res)
}

func TestNamespaceWatcherSuite(t *testing.T) {
	suite.Run(t, new(NamespaceWatcherTestSuite))
}
