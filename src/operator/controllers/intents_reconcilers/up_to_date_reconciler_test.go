package intents_reconcilers

import (
	"context"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

type UpToDateReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	Reconciler *UpToDateReconciler
}

func (s *UpToDateReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.Reconciler = NewUpToDateReconciler(s.Client, s.Scheme)
}

func (s *UpToDateReconcilerTestSuite) TestReconcile_NeedsUpdate_FalseToTrue() {
	approvedClientIntents := otterizev2alpha1.ApprovedClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},
		Status: otterizev2alpha1.ApprovedClientIntentsStatus{
			UpToDate: true,
		},
	}

	emptyIntents := &otterizev2alpha1.ApprovedClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Namespace: approvedClientIntents.Namespace, Name: approvedClientIntents.Name}, emptyIntents).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev2alpha1.ApprovedClientIntents, options ...client.ListOption) error {
			approvedClientIntents.DeepCopyInto(intents)
			return nil
		})

	clientIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},
		Status: otterizev2alpha1.IntentsStatus{
			UpToDate: false,
		},
	}

	emptyClientIntents := &otterizev2alpha1.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Namespace: clientIntents.Namespace, Name: clientIntents.Name}, emptyClientIntents).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev2alpha1.ClientIntents, options ...client.ListOption) error {
			clientIntents.DeepCopyInto(intents)
			return nil
		})

	toBeUpdatedClientIntents := clientIntents.DeepCopy()
	toBeUpdatedClientIntents.Status.UpToDate = true

	s.Client.EXPECT().Status().Return(s.StatusWriter)
	s.StatusWriter.EXPECT().Patch(gomock.Any(), gomock.Eq(toBeUpdatedClientIntents), MatchPatch(client.MergeFrom(&clientIntents)))

	res, err := s.Reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: approvedClientIntents.Namespace, Name: approvedClientIntents.Name}})
	s.Require().NoError(err)
	s.Require().Equal(ctrl.Result{}, res)

}

func (s *UpToDateReconcilerTestSuite) TestReconcile_NeedsUpdate_TrueToFalse() {
	approvedClientIntents := otterizev2alpha1.ApprovedClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},
		Status: otterizev2alpha1.ApprovedClientIntentsStatus{
			UpToDate: false,
		},
	}

	emptyIntents := &otterizev2alpha1.ApprovedClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Namespace: approvedClientIntents.Namespace, Name: approvedClientIntents.Name}, emptyIntents).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev2alpha1.ApprovedClientIntents, options ...client.ListOption) error {
			approvedClientIntents.DeepCopyInto(intents)
			return nil
		})

	clientIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},
		Status: otterizev2alpha1.IntentsStatus{
			UpToDate: true,
		},
	}

	emptyClientIntents := &otterizev2alpha1.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Namespace: clientIntents.Namespace, Name: clientIntents.Name}, emptyClientIntents).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev2alpha1.ClientIntents, options ...client.ListOption) error {
			clientIntents.DeepCopyInto(intents)
			return nil
		})

	toBeUpdatedClientIntents := clientIntents.DeepCopy()
	toBeUpdatedClientIntents.Status.UpToDate = false

	s.Client.EXPECT().Status().Return(s.StatusWriter)
	s.StatusWriter.EXPECT().Patch(gomock.Any(), gomock.Eq(toBeUpdatedClientIntents), MatchPatch(client.MergeFrom(&clientIntents)))

	res, err := s.Reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: approvedClientIntents.Namespace, Name: approvedClientIntents.Name}})
	s.Require().NoError(err)
	s.Require().Equal(ctrl.Result{}, res)
}

func (s *UpToDateReconcilerTestSuite) TestReconcile_NoUpdate() {
	approvedClientIntents := otterizev2alpha1.ApprovedClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},
		Status: otterizev2alpha1.ApprovedClientIntentsStatus{
			UpToDate: true,
		},
	}

	emptyIntents := &otterizev2alpha1.ApprovedClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Namespace: approvedClientIntents.Namespace, Name: approvedClientIntents.Name}, emptyIntents).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev2alpha1.ApprovedClientIntents, options ...client.ListOption) error {
			approvedClientIntents.DeepCopyInto(intents)
			return nil
		})

	clientIntents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      intentsObjectName,
			Namespace: testNamespace,
		},
		Status: otterizev2alpha1.IntentsStatus{
			UpToDate: true,
		},
	}

	emptyClientIntents := &otterizev2alpha1.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Namespace: clientIntents.Namespace, Name: clientIntents.Name}, emptyClientIntents).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev2alpha1.ClientIntents, options ...client.ListOption) error {
			clientIntents.DeepCopyInto(intents)
			return nil
		})

	res, err := s.Reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: approvedClientIntents.Namespace, Name: approvedClientIntents.Name}})
	s.Require().NoError(err)
	s.Require().Equal(ctrl.Result{}, res)
}

func TestUpToDateReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(UpToDateReconcilerTestSuite))
}
