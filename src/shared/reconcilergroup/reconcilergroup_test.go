package reconcilergroup

import (
	"context"
	"errors"
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
	"time"
)

const (
	testFinalizer = "test-finalizer"
)

type ReconcilerGroupTestSuite struct {
	suite.Suite
	group  *Group
	client *mocks.MockClient
}

func (s *ReconcilerGroupTestSuite) SetupTest() {
	controller := gomock.NewController(s.T())
	s.client = mocks.NewMockClient(controller)
	s.group = NewGroup("test",
		s.client,
		&runtime.Scheme{},
		&otterizev1alpha3.ClientIntents{},
		testFinalizer,
		nil,
	)
}

type TestReconciler struct {
	Reconciled bool
	Err        error
	Result     reconcile.Result
}

func (t *TestReconciler) Reconcile(_ context.Context, _ reconcile.Request) (reconcile.Result, error) {
	t.Reconciled = true
	return t.Result, t.Err
}

func (t *TestReconciler) InjectRecorder(recorder record.EventRecorder) {
	// Just to implement the interface
}

func (s *ReconcilerGroupTestSuite) ExpectIntentWithFinalizer() {
	emptyIntents := &otterizev1alpha3.ClientIntents{}
	s.client.EXPECT().Get(gomock.Any(), types.NamespacedName{}, gomock.Eq(emptyIntents)).DoAndReturn(
		func(_ context.Context, _ types.NamespacedName, intents *otterizev1alpha3.ClientIntents, _ ...client.GetOption) error {
			*intents = *emptyIntents
			controllerutil.AddFinalizer(intents, testFinalizer)
			return nil
		})
}

func (s *ReconcilerGroupTestSuite) TestGroupError() {
	reconcilerWithError := &TestReconciler{Err: fmt.Errorf("test error"), Result: reconcile.Result{}}
	reconcilerWithResult := &TestReconciler{Err: nil, Result: reconcile.Result{Requeue: true}}
	happyReconciler := &TestReconciler{Err: nil, Result: reconcile.Result{}}
	s.group.AddToGroup(reconcilerWithError)
	s.group.AddToGroup(reconcilerWithResult)
	s.group.AddToGroup(happyReconciler)

	s.ExpectIntentWithFinalizer()

	res, err := s.group.Reconcile(context.Background(), reconcile.Request{})
	s.Require().Equal(reconcilerWithError.Err, err)
	s.Require().Equal(reconcilerWithResult.Result, res)
	s.Require().True(reconcilerWithError.Reconciled)
	s.Require().True(reconcilerWithResult.Reconciled)
	s.Require().True(happyReconciler.Reconciled)
}

func (s *ReconcilerGroupTestSuite) TestAddToGroup() {
	happyReconciler := &TestReconciler{}
	addedReconciler := &TestReconciler{}
	s.group.AddToGroup(happyReconciler)
	s.group.AddToGroup(addedReconciler)

	s.ExpectIntentWithFinalizer()

	res, err := s.group.Reconcile(context.Background(), reconcile.Request{})
	s.Require().NoError(err)
	s.Require().True(res.IsZero())
	s.Require().True(happyReconciler.Reconciled)
	s.Require().True(addedReconciler.Reconciled)
}

func (s *ReconcilerGroupTestSuite) TestGroupSuccess() {
	happyReconciler := &TestReconciler{Err: nil, Result: reconcile.Result{}}
	anotherHappyReconciler := &TestReconciler{Err: nil, Result: reconcile.Result{}}
	s.group.AddToGroup(happyReconciler)
	s.group.AddToGroup(anotherHappyReconciler)

	s.ExpectIntentWithFinalizer()

	res, err := s.group.Reconcile(context.Background(), reconcile.Request{})
	s.Require().NoError(err)
	s.Require().Equal(reconcile.Result{}, res)
	s.Require().True(anotherHappyReconciler.Reconciled)
	s.Require().True(happyReconciler.Reconciled)
}

func (s *ReconcilerGroupTestSuite) TestResultIsTheShortestRequeueTime() {
	happyReconciler := &TestReconciler{Err: nil, Result: reconcile.Result{RequeueAfter: 10}}
	anotherHappyReconciler := &TestReconciler{Err: nil, Result: reconcile.Result{RequeueAfter: 5}}
	s.group.AddToGroup(happyReconciler)
	s.group.AddToGroup(anotherHappyReconciler)

	s.ExpectIntentWithFinalizer()

	res, err := s.group.Reconcile(context.Background(), reconcile.Request{})
	s.Require().NoError(err)
	s.Require().Equal(reconcile.Result{RequeueAfter: 5}, res)
	s.Require().True(anotherHappyReconciler.Reconciled)
	s.Require().True(happyReconciler.Reconciled)
}

func (s *ReconcilerGroupTestSuite) TestResultIsTheEmptyRequeueAfter() {
	happyReconciler := &TestReconciler{Err: nil, Result: reconcile.Result{RequeueAfter: 10}}
	anotherHappyReconciler := &TestReconciler{Err: nil, Result: reconcile.Result{Requeue: true}}
	s.group.AddToGroup(happyReconciler)
	s.group.AddToGroup(anotherHappyReconciler)

	s.ExpectIntentWithFinalizer()

	res, err := s.group.Reconcile(context.Background(), reconcile.Request{})
	s.Require().NoError(err)
	s.Require().Equal(reconcile.Result{Requeue: true}, res)
	s.Require().True(anotherHappyReconciler.Reconciled)
	s.Require().True(happyReconciler.Reconciled)
}

func (s *ReconcilerGroupTestSuite) TestFinalizerAddedBefore() {
	reconciler := &TestReconciler{Err: nil, Result: reconcile.Result{}}
	s.group.AddToGroup(reconciler)

	resourceName := types.NamespacedName{
		Name:      "my-resource",
		Namespace: "the-happy-place-we-live-in",
	}

	emptyIntents := &otterizev1alpha3.ClientIntents{}
	s.client.EXPECT().Get(gomock.Any(), resourceName, gomock.Eq(emptyIntents)).Return(nil)
	intentsWithFinalizer := emptyIntents.DeepCopy()
	controllerutil.AddFinalizer(intentsWithFinalizer, testFinalizer)
	s.client.EXPECT().Update(gomock.Any(), gomock.Eq(intentsWithFinalizer)).Return(nil)

	res, err := s.group.Reconcile(context.Background(), reconcile.Request{NamespacedName: resourceName})
	s.Require().NoError(err)
	s.Require().Empty(res)
	s.Require().True(reconciler.Reconciled)
}

func (s *ReconcilerGroupTestSuite) TestFinalizerGetError() {
	reconciler := &TestReconciler{Err: nil, Result: reconcile.Result{}}
	s.group.AddToGroup(reconciler)

	resourceName := types.NamespacedName{
		Name:      "my-resource",
		Namespace: "the-happy-place-we-live-in",
	}

	emptyIntents := &otterizev1alpha3.ClientIntents{}
	s.client.EXPECT().Get(gomock.Any(), resourceName, gomock.Eq(emptyIntents)).Return(errors.New("test error"))

	res, err := s.group.Reconcile(context.Background(), reconcile.Request{NamespacedName: resourceName})
	s.Require().Error(err)
	s.Require().Empty(res)
	s.Require().False(reconciler.Reconciled)
}

func (s *ReconcilerGroupTestSuite) TestDeletedObjectNothingRun() {
	reconciler := &TestReconciler{Err: nil, Result: reconcile.Result{}}
	s.group.AddToGroup(reconciler)

	resourceName := types.NamespacedName{
		Name:      "my-resource",
		Namespace: "the-happy-place-we-live-in",
	}

	emptyIntents := &otterizev1alpha3.ClientIntents{}
	notFoundErr := k8serrors.NewNotFound(schema.GroupResource{}, resourceName.Name)
	s.client.EXPECT().Get(gomock.Any(), resourceName, gomock.Eq(emptyIntents)).Return(notFoundErr)

	res, err := s.group.Reconcile(context.Background(), reconcile.Request{NamespacedName: resourceName})
	s.Require().NoError(err)
	s.Require().Empty(res)
	s.Require().False(reconciler.Reconciled)
}

func (s *ReconcilerGroupTestSuite) TestDoNothingIfFinalizerIsThere() {
	reconciler := &TestReconciler{Err: nil, Result: reconcile.Result{}}
	s.group.AddToGroup(reconciler)

	resourceName := types.NamespacedName{
		Name:      "my-resource",
		Namespace: "the-happy-place-we-live-in",
	}

	emptyIntents := &otterizev1alpha3.ClientIntents{}
	intentsWithFinalizer := emptyIntents.DeepCopy()
	controllerutil.AddFinalizer(intentsWithFinalizer, testFinalizer)
	s.client.EXPECT().Get(gomock.Any(), resourceName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(_ context.Context, _ types.NamespacedName, intents *otterizev1alpha3.ClientIntents, _ ...client.GetOption) error {
			*intents = *intentsWithFinalizer
			return nil
		})

	res, err := s.group.Reconcile(context.Background(), reconcile.Request{NamespacedName: resourceName})
	s.Require().NoError(err)
	s.Require().Empty(res)
	s.Require().True(reconciler.Reconciled)
}

func (s *ReconcilerGroupTestSuite) TestRemoveLegacyFinalizer() {
	legacyFinalizers := []string{
		"lemon-juice-reconciler-finalizer",
		"tabasco-reconciler-finalizer",
	}

	s.group.legacyFinalizers = legacyFinalizers

	reconciler := &TestReconciler{Err: nil, Result: reconcile.Result{}}
	s.group.AddToGroup(reconciler)

	resourceName := types.NamespacedName{
		Name:      "my-resource",
		Namespace: "the-happy-place-we-live-in",
	}

	emptyIntents := &otterizev1alpha3.ClientIntents{}
	intentsWithLegacyFinalizers := emptyIntents.DeepCopy()
	controllerutil.AddFinalizer(intentsWithLegacyFinalizers, legacyFinalizers[0])
	controllerutil.AddFinalizer(intentsWithLegacyFinalizers, legacyFinalizers[1])
	s.client.EXPECT().Get(gomock.Any(), resourceName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(_ context.Context, _ types.NamespacedName, intents *otterizev1alpha3.ClientIntents, _ ...client.GetOption) error {
			intentsWithLegacyFinalizers.DeepCopyInto(intents)
			return nil
		})

	intentsWithAllFinalizers := intentsWithLegacyFinalizers.DeepCopy()
	controllerutil.AddFinalizer(intentsWithAllFinalizers, testFinalizer)
	s.client.EXPECT().Update(gomock.Any(), gomock.Eq(intentsWithAllFinalizers)).Return(nil)

	intentsWithValidFinalizer := intentsWithAllFinalizers.DeepCopy()
	controllerutil.RemoveFinalizer(intentsWithValidFinalizer, legacyFinalizers[0])
	controllerutil.RemoveFinalizer(intentsWithValidFinalizer, legacyFinalizers[1])
	s.client.EXPECT().Update(gomock.Any(), gomock.Eq(intentsWithValidFinalizer)).Return(nil)

	res, err := s.group.Reconcile(context.Background(), reconcile.Request{NamespacedName: resourceName})
	s.Require().NoError(err)
	s.Require().Empty(res)
	s.Require().True(reconciler.Reconciled)
}

func (s *ReconcilerGroupTestSuite) TestFinalizerRemoveAfter() {
	reconciler := &TestReconciler{Err: nil, Result: reconcile.Result{}}
	s.group.AddToGroup(reconciler)

	resourceName := types.NamespacedName{
		Name:      "my-resource",
		Namespace: "the-happy-place-we-live-in",
	}

	emptyIntents := &otterizev1alpha3.ClientIntents{}
	intentsWithFinalizer := emptyIntents.DeepCopy()
	controllerutil.AddFinalizer(intentsWithFinalizer, testFinalizer)
	intentsWithFinalizer.DeletionTimestamp = &v1.Time{Time: time.Date(1992, 4, 25, 19, 30, 0, 0, time.UTC)}

	s.client.EXPECT().Get(gomock.Any(), resourceName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(_ context.Context, _ types.NamespacedName, intents *otterizev1alpha3.ClientIntents, _ ...client.GetOption) error {
			*intents = *intentsWithFinalizer
			return nil
		})

	intentsWithoutFinalizer := intentsWithFinalizer.DeepCopy()
	controllerutil.RemoveFinalizer(intentsWithoutFinalizer, testFinalizer)
	s.client.EXPECT().Update(gomock.Any(), gomock.Eq(intentsWithoutFinalizer)).Return(nil)

	res, err := s.group.Reconcile(context.Background(), reconcile.Request{NamespacedName: resourceName})
	s.Require().NoError(err)
	s.Require().Empty(res)
	s.Require().True(reconciler.Reconciled)
}

func (s *ReconcilerGroupTestSuite) TestFinalizerUpdateFailedAfterDelete() {
	reconciler := &TestReconciler{Err: nil, Result: reconcile.Result{}}
	s.group.AddToGroup(reconciler)

	resourceName := types.NamespacedName{
		Name:      "my-resource",
		Namespace: "the-happy-place-we-live-in",
	}

	emptyIntents := &otterizev1alpha3.ClientIntents{}
	intentsWithFinalizer := emptyIntents.DeepCopy()
	controllerutil.AddFinalizer(intentsWithFinalizer, testFinalizer)
	intentsWithFinalizer.DeletionTimestamp = &v1.Time{Time: time.Date(1992, 4, 25, 19, 30, 0, 0, time.UTC)}

	s.client.EXPECT().Get(gomock.Any(), resourceName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(_ context.Context, _ types.NamespacedName, intents *otterizev1alpha3.ClientIntents, _ ...client.GetOption) error {
			*intents = *intentsWithFinalizer
			return nil
		})

	intentsWithoutFinalizer := intentsWithFinalizer.DeepCopy()
	controllerutil.RemoveFinalizer(intentsWithoutFinalizer, testFinalizer)
	s.client.EXPECT().Update(gomock.Any(), gomock.Eq(intentsWithoutFinalizer)).Return(errors.New("test error"))

	res, err := s.group.Reconcile(context.Background(), reconcile.Request{NamespacedName: resourceName})
	s.Require().Error(err)
	s.Require().Empty(res)
	s.Require().True(reconciler.Reconciled)
}

func (s *ReconcilerGroupTestSuite) TestFinalizerNotDeletedIfReconcilerFailed() {
	reconciler := &TestReconciler{Err: errors.New("test error"), Result: reconcile.Result{}}
	s.group.AddToGroup(reconciler)

	resourceName := types.NamespacedName{
		Name:      "my-resource",
		Namespace: "the-happy-place-we-live-in",
	}

	emptyIntents := &otterizev1alpha3.ClientIntents{}
	intentsWithFinalizer := emptyIntents.DeepCopy()
	controllerutil.AddFinalizer(intentsWithFinalizer, testFinalizer)
	intentsWithFinalizer.DeletionTimestamp = &v1.Time{Time: time.Date(2015, 11, 10, 18, 7, 0, 0, time.UTC)}

	s.client.EXPECT().Get(gomock.Any(), resourceName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(_ context.Context, _ types.NamespacedName, intents *otterizev1alpha3.ClientIntents, _ ...client.GetOption) error {
			*intents = *intentsWithFinalizer
			return nil
		})

	res, err := s.group.Reconcile(context.Background(), reconcile.Request{NamespacedName: resourceName})
	s.Require().Error(err)
	s.Require().Empty(res)
	s.Require().True(reconciler.Reconciled)
}

func (s *ReconcilerGroupTestSuite) TestFinalizerNotDeletedIfReconcilerRequeue() {
	reconciler := &TestReconciler{Err: nil, Result: reconcile.Result{Requeue: true}}
	s.group.AddToGroup(reconciler)

	resourceName := types.NamespacedName{
		Name:      "my-resource",
		Namespace: "the-happy-place-we-live-in",
	}

	emptyIntents := &otterizev1alpha3.ClientIntents{}
	intentsWithFinalizer := emptyIntents.DeepCopy()
	controllerutil.AddFinalizer(intentsWithFinalizer, testFinalizer)
	intentsWithFinalizer.DeletionTimestamp = &v1.Time{Time: time.Date(1992, 4, 25, 19, 30, 0, 0, time.UTC)}

	s.client.EXPECT().Get(gomock.Any(), resourceName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(_ context.Context, _ types.NamespacedName, intents *otterizev1alpha3.ClientIntents, _ ...client.GetOption) error {
			*intents = *intentsWithFinalizer
			return nil
		})

	res, err := s.group.Reconcile(context.Background(), reconcile.Request{NamespacedName: resourceName})
	s.Require().NoError(err)
	s.Require().NotEmpty(res)
	s.Require().True(reconciler.Reconciled)
}

func TestReconcilerGroup(t *testing.T) {
	suite.Run(t, new(ReconcilerGroupTestSuite))
}
