package controllers

import (
	"context"
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
)

type IntentsControllerTestSuite struct {
	testbase.MocksSuiteBase
	intentsReconciler *IntentsReconciler
}

func (s *IntentsControllerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.intentsReconciler = NewIntentsReconciler(context.Background(), s.Client, nil)
	s.intentsReconciler.InjectRecorder(s.Recorder)
	s.expectScheme()
}

func (s *IntentsControllerTestSuite) expectRemoveOrphanedIntents() {
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&v2alpha1.ClientIntentsList{})).AnyTimes()
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&v2alpha1.ApprovedClientIntentsList{})).AnyTimes()
}

func (s *IntentsControllerTestSuite) expectScheme() {
	s.Client.EXPECT().Scheme().Return(s.Scheme).AnyTimes()
}

func (s *IntentsControllerTestSuite) TestIntentsReconcile_UpdateIntents_FullApprovalFlow() {
	// This test case will test the full approval flow of an updated clientIntents:
	// 1. ClientIntents is updated, so it's generation is increased and the review status seems approved. The operator should:
	//    a. Set the review status to pending
	//    b. Set the observed generation to the new generation
	//    d. finish reconciliation
	// 2. ClientIntents review status is pending:
	//    a. The operator should approve the clientIntents (since we are on AutoApprove mode)
	//    b. finish reconciliation
	// 3. ClientIntents review status is approved:
	//    a. The operator should create the ApprovedClientIntents
	//    c. finish reconciliation

	s.expectRemoveOrphanedIntents()

	// Iteration 1: ClientIntents is up-to-date and review status is approved -
	// we should update the status to pending and observed generation to generation and up-to-date to false
	clientIntentsStatusApprovedNewGeneration := v2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "client-intents",
			Namespace:  "test-namespace",
			Generation: 2,
		},
		Spec: &v2alpha1.IntentsSpec{
			Workload: v2alpha1.Workload{
				Name: "workload",
			},
		},
		Status: v2alpha1.IntentsStatus{
			ReviewStatus:       v2alpha1.ReviewStatusApproved,
			ObservedGeneration: 1,
		},
	}

	s.Client.EXPECT().Get(gomock.Any(), gomock.Eq(client.ObjectKey{Name: "client-intents", Namespace: "test-namespace"}), gomock.Any()).DoAndReturn(func(_ context.Context, _ client.ObjectKey, intent *v2alpha1.ClientIntents, _ ...any) error {
		clientIntentsStatusApprovedNewGeneration.DeepCopyInto(intent)
		return nil
	})

	// The operator should update the status of the clientIntents to be up-to-date and review status to pending and observed generation to generation
	toBeUpdatedClientIntents := clientIntentsStatusApprovedNewGeneration.DeepCopy()
	toBeUpdatedClientIntents.Status.ReviewStatus = v2alpha1.ReviewStatusPending
	toBeUpdatedClientIntents.Status.ObservedGeneration = clientIntentsStatusApprovedNewGeneration.Generation

	s.Client.EXPECT().Status().DoAndReturn(func() client.StatusWriter {
		return s.StatusWriter
	})
	s.StatusWriter.EXPECT().Patch(gomock.Any(), gomock.Eq(toBeUpdatedClientIntents), intents_reconcilers.MatchPatch(client.MergeFrom(&clientIntentsStatusApprovedNewGeneration)))

	res, err := s.intentsReconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKey{Name: "client-intents", Namespace: "test-namespace"}})
	s.Require().NoError(err)
	s.Require().Equal(res, reconcile.Result{})
	// End of iteration 1

	// Iteration 2: ClientIntents is not up-to-date and review status is pending -
	//	we are on AutoApprove mode so we should patch the status to approved
	clientIntentsStatusPending := toBeUpdatedClientIntents

	afterSecondIterationClientIntents := clientIntentsStatusPending.DeepCopy()
	afterSecondIterationClientIntents.Status.ReviewStatus = v2alpha1.ReviewStatusApproved

	s.Client.EXPECT().Get(gomock.Any(), gomock.Eq(client.ObjectKey{Name: "client-intents", Namespace: "test-namespace"}), gomock.Any()).DoAndReturn(func(_ context.Context, _ client.ObjectKey, intent *v2alpha1.ClientIntents, _ ...any) error {
		clientIntentsStatusPending.DeepCopyInto(intent)
		return nil
	})

	s.Client.EXPECT().Status().DoAndReturn(func() client.StatusWriter {
		return s.StatusWriter
	})
	s.StatusWriter.EXPECT().Patch(gomock.Any(), gomock.Eq(afterSecondIterationClientIntents), intents_reconcilers.MatchPatch(client.MergeFrom(clientIntentsStatusPending)))

	res, err = s.intentsReconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKey{Name: "client-intents", Namespace: "test-namespace"}})
	s.Require().NoError(err)
	s.Require().Equal(res, reconcile.Result{})
	// End of iteration 2

	// Iteration 3: ClientIntents is not up-to-date and review status is approved - Now we will create the ApprovedClientIntents and set up-to-date to true
	clientsIntentsWithApprovedReviewStatus := afterSecondIterationClientIntents.DeepCopy()

	s.Client.EXPECT().Get(gomock.Any(), gomock.Eq(client.ObjectKey{Name: "client-intents", Namespace: "test-namespace"}), gomock.Any()).DoAndReturn(func(_ context.Context, _ client.ObjectKey, intent *v2alpha1.ClientIntents, _ ...any) error {
		clientsIntentsWithApprovedReviewStatus.DeepCopyInto(intent)
		return nil
	})

	approvedClientIntents := v2alpha1.ApprovedClientIntents{}
	approvedClientIntents.FromClientIntents(*afterSecondIterationClientIntents)
	approvedClientIntents.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "k8s.otterize.com/v2alpha1",
			Kind:       "ClientIntents",
			Name:       afterSecondIterationClientIntents.Name,
			UID:        afterSecondIterationClientIntents.UID,
		},
	}

	// check if the approvedClientIntents already exists - return NotFound error
	s.Client.EXPECT().Get(gomock.Any(), gomock.Eq(client.ObjectKey{Name: approvedClientIntents.Name, Namespace: "test-namespace"}), gomock.Any()).Return(&errors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound}})

	// create the approvedClientIntents
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(&approvedClientIntents))

	res, err = s.intentsReconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKey{Name: "client-intents", Namespace: "test-namespace"}})
	s.Require().NoError(err)
	s.Require().Equal(res, reconcile.Result{})
	s.ExpectEvent(ReasonApprovedIntentsCreated)
	// End of iteration 3
}

func (s *IntentsControllerTestSuite) TestIntentsReconcile_UpdateIntents_ExistingApprovedIntentsNotChanged() {
	// we won't test the full flow of the update of the clientIntents, we will only test the relevant reconcile iteration
	// where the clientIntents review status is approved and the approvedClientIntents already exists

	s.expectRemoveOrphanedIntents()

	clientIntentsStatusApprovedNewGeneration := v2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "client-intents",
			Namespace:  "test-namespace",
			Generation: 2,
		},
		Spec: &v2alpha1.IntentsSpec{
			Workload: v2alpha1.Workload{
				Name: "workload",
			},
		},
		Status: v2alpha1.IntentsStatus{
			ReviewStatus:       v2alpha1.ReviewStatusApproved,
			ObservedGeneration: 2,
		},
	}

	s.Client.EXPECT().Get(gomock.Any(), gomock.Eq(client.ObjectKey{Name: "client-intents", Namespace: "test-namespace"}), gomock.Any()).DoAndReturn(func(_ context.Context, _ client.ObjectKey, intent *v2alpha1.ClientIntents, _ ...any) error {
		clientIntentsStatusApprovedNewGeneration.DeepCopyInto(intent)
		return nil
	})

	approvedClientIntents := v2alpha1.ApprovedClientIntents{}
	approvedClientIntents.FromClientIntents(clientIntentsStatusApprovedNewGeneration)
	approvedClientIntents.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "k8s.otterize.com/v2alpha1",
			Kind:       "ClientIntents",
			Name:       clientIntentsStatusApprovedNewGeneration.Name,
			UID:        clientIntentsStatusApprovedNewGeneration.UID,
		},
	}

	// check if the approvedClientIntents already exists
	// no update should be done on the approvedClientIntents since I used the same approvedClientIntents object
	s.Client.EXPECT().Get(gomock.Any(), gomock.Eq(client.ObjectKey{Name: approvedClientIntents.Name, Namespace: "test-namespace"}), gomock.Any()).DoAndReturn(func(_ context.Context, _ client.ObjectKey, intent *v2alpha1.ApprovedClientIntents, _ ...any) error {
		approvedClientIntents.DeepCopyInto(intent)
		return nil
	})

	res, err := s.intentsReconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKey{Name: "client-intents", Namespace: "test-namespace"}})
	s.Require().NoError(err)
	s.Require().Equal(res, reconcile.Result{})
}

func (s *IntentsControllerTestSuite) TestIntentsReconcile_UpdateIntents_ExistingApprovedIntentsChanged() {
	// we won't test the full flow of the update of the clientIntents, we will only test the relevant reconcile iteration
	// where the clientIntents review status is approved and the approvedClientIntents already exists

	s.expectRemoveOrphanedIntents()

	clientIntentsStatusApprovedNewGeneration := v2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "client-intents",
			Namespace:  "test-namespace",
			Generation: 2,
		},
		Spec: &v2alpha1.IntentsSpec{
			Workload: v2alpha1.Workload{
				Name: "workload",
			},
		},
		Status: v2alpha1.IntentsStatus{
			ReviewStatus:       v2alpha1.ReviewStatusApproved,
			ObservedGeneration: 2,
		},
	}

	s.Client.EXPECT().Get(gomock.Any(), gomock.Eq(client.ObjectKey{Name: "client-intents", Namespace: "test-namespace"}), gomock.Any()).DoAndReturn(func(_ context.Context, _ client.ObjectKey, intent *v2alpha1.ClientIntents, _ ...any) error {
		clientIntentsStatusApprovedNewGeneration.DeepCopyInto(intent)
		return nil
	})

	approvedClientIntents := v2alpha1.ApprovedClientIntents{}
	approvedClientIntents.FromClientIntents(clientIntentsStatusApprovedNewGeneration)
	approvedClientIntents.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "k8s.otterize.com/v2alpha1",
			Kind:       "ClientIntents",
			Name:       clientIntentsStatusApprovedNewGeneration.Name,
			UID:        clientIntentsStatusApprovedNewGeneration.UID,
		},
	}

	// check if the approvedClientIntents already exists
	s.Client.EXPECT().Get(gomock.Any(), gomock.Eq(client.ObjectKey{Name: approvedClientIntents.Name, Namespace: "test-namespace"}), gomock.Any()).DoAndReturn(func(_ context.Context, _ client.ObjectKey, intent *v2alpha1.ApprovedClientIntents, _ ...any) error {
		oldApprovedClientIntents := approvedClientIntents.DeepCopy()
		oldApprovedClientIntents.Spec.Workload.Name = "old"
		oldApprovedClientIntents.DeepCopyInto(intent)
		return nil
	})

	// update the approvedClientIntents
	s.Client.EXPECT().Patch(gomock.Any(), gomock.Eq(&approvedClientIntents), intents_reconcilers.MatchPatch(client.MergeFrom(&approvedClientIntents)))

	res, err := s.intentsReconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKey{Name: "client-intents", Namespace: "test-namespace"}})
	s.Require().NoError(err)
	s.Require().Equal(res, reconcile.Result{})
	s.ExpectEvent(ReasonApprovedIntentsUpdated)
}

func TestIntentsControllerTestSuite(t *testing.T) {
	suite.Run(t, new(IntentsControllerTestSuite))
}
