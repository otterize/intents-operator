package reconcilergroup

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
)

type ReconcilerGroupTestSuite struct {
	suite.Suite
	group *Group
}

func (s *ReconcilerGroupTestSuite) SetupTest() {
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

func (s *ReconcilerGroupTestSuite) TestGroupError() {
	reconcilerWithError := &TestReconciler{Err: fmt.Errorf("test error"), Result: reconcile.Result{}}
	reconcilerWithResult := &TestReconciler{Err: nil, Result: reconcile.Result{Requeue: true}}
	happyReconciler := &TestReconciler{Err: nil, Result: reconcile.Result{}}
	s.group = NewGroup("test", nil, nil, reconcilerWithError, reconcilerWithResult, happyReconciler)
	res, err := s.group.Reconcile(nil, reconcile.Request{})
	s.Require().Equal(reconcilerWithError.Err, err)
	s.Require().Equal(reconcilerWithResult.Result, res)
	s.Require().True(reconcilerWithError.Reconciled)
	s.Require().True(reconcilerWithResult.Reconciled)
	s.Require().True(happyReconciler.Reconciled)
}

func (s *ReconcilerGroupTestSuite) TestAddToGroup() {
	happyReconciler := &TestReconciler{}
	addedReconciler := &TestReconciler{}
	s.group = NewGroup("test", nil, nil, happyReconciler)
	s.group.AddToGroup(addedReconciler)
	res, err := s.group.Reconcile(nil, reconcile.Request{})
	s.Require().NoError(err)
	s.Require().True(res.IsZero())
	s.Require().True(happyReconciler.Reconciled)
	s.Require().True(addedReconciler.Reconciled)
}

func (s *ReconcilerGroupTestSuite) TestGroupSuccess() {
	happyReconciler := &TestReconciler{Err: nil, Result: reconcile.Result{}}
	anotherHappyReconciler := &TestReconciler{Err: nil, Result: reconcile.Result{}}
	s.group = NewGroup("test", nil, nil, happyReconciler, anotherHappyReconciler)
	res, err := s.group.Reconcile(nil, reconcile.Request{})
	s.Require().NoError(err)
	s.Require().Equal(reconcile.Result{}, res)
	s.Require().True(anotherHappyReconciler.Reconciled)
	s.Require().True(happyReconciler.Reconciled)
}

func (s *ReconcilerGroupTestSuite) TestResultIsTheShortestRequeueTime() {
	happyReconciler := &TestReconciler{Err: nil, Result: reconcile.Result{RequeueAfter: 10}}
	anotherHappyReconciler := &TestReconciler{Err: nil, Result: reconcile.Result{RequeueAfter: 5}}
	s.group = NewGroup("test", nil, nil, happyReconciler, anotherHappyReconciler)
	res, err := s.group.Reconcile(nil, reconcile.Request{})
	s.Require().NoError(err)
	s.Require().Equal(reconcile.Result{RequeueAfter: 5}, res)
	s.Require().True(anotherHappyReconciler.Reconciled)
	s.Require().True(happyReconciler.Reconciled)
}

func (s *ReconcilerGroupTestSuite) TestResultIsTheEmptyRequeueAfter() {
	happyReconciler := &TestReconciler{Err: nil, Result: reconcile.Result{RequeueAfter: 10}}
	anotherHappyReconciler := &TestReconciler{Err: nil, Result: reconcile.Result{Requeue: true}}
	s.group = NewGroup("test", nil, nil, happyReconciler, anotherHappyReconciler)
	res, err := s.group.Reconcile(nil, reconcile.Request{})
	s.Require().NoError(err)
	s.Require().Equal(reconcile.Result{Requeue: true}, res)
	s.Require().True(anotherHappyReconciler.Reconciled)
	s.Require().True(happyReconciler.Reconciled)
}

func TestReconcilerGroup(t *testing.T) {
	suite.Run(t, new(ReconcilerGroupTestSuite))
}
