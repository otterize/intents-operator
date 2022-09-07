package intents_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"testing"
)

type NetworkPolicyReconcilerTestSuite struct {
	testbase.ControllerManagerTestSuiteBase
	Reconciler *NetworkPolicyReconciler
}

func (s *NetworkPolicyReconcilerTestSuite) SetupSuite() {
	s.ControllerManagerTestSuiteBase.SetupSuite([]string{"../../config/crd/bases"})
	err := otterizev1alpha1.AddToScheme(s.TestEnv.Scheme)
	s.Require().NoError(err)
}

func (s *NetworkPolicyReconcilerTestSuite) BeforeTest(_, testName string) {
	s.Require().NoError(s.initServerIndices(s.Mgr))
	s.ControllerManagerTestSuiteBase.BeforeTest("", testName)
}

func (s *NetworkPolicyReconcilerTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()
	s.Reconciler = NewNetworkPolicyReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme, []string{})
	recorder := s.Mgr.GetEventRecorderFor("intents-operator")
	s.Reconciler.InjectRecorder(recorder)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreate() {
	intents := s.AddIntents("test-intents", "test-client", []otterizev1alpha1.Intent{{
		Type: otterizev1alpha1.IntentTypeHTTP, Name: "test-server",
	},
	})
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	res, err := s.Reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)

	np := v1.NetworkPolicy{}
	policyName := fmt.Sprintf(OtterizeNetworkPolicyNameTemplate, "test-server", s.TestNamespace)
	err = s.Mgr.GetCache().Get(context.Background(), types.NamespacedName{
		Namespace: s.TestNamespace, Name: policyName,
	}, &np)
	s.Require().NoError(err)
	s.Require().NotEmpty(np)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateCrossNamespace() {
	// Create namespace
	otherNamespace := "test-cross-ns-create"

	_, err := s.K8sDirectClient.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: otherNamespace}}, metav1.CreateOptions{})
	s.Require().NoError(err)

	intents := s.AddIntents("cross-ns-test-intents", "test-client", []otterizev1alpha1.Intent{{
		Type: otterizev1alpha1.IntentTypeHTTP, Name: "test-server", Namespace: otherNamespace}})
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	res, err := s.Reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)

	np := v1.NetworkPolicy{}
	policyName := fmt.Sprintf(OtterizeNetworkPolicyNameTemplate, "test-server", s.TestNamespace)
	err = s.Mgr.GetCache().Get(context.Background(), types.NamespacedName{
		Namespace: otherNamespace, Name: policyName,
	}, &np)
	s.Require().NoError(err)
	s.Require().NotEmpty(np)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNetworkPolicyCleanup() {
	intents := s.AddIntents(
		"cleanup-test", "test-client", []otterizev1alpha1.Intent{{
			Type: otterizev1alpha1.IntentTypeHTTP, Name: "test-server"}})

	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
	res, err := s.Reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
	s.Require().NoError(err)
	s.Require().Empty(res)

	np := v1.NetworkPolicy{}
	policyName := fmt.Sprintf(OtterizeNetworkPolicyNameTemplate, "test-server", s.TestNamespace)
	err = s.Mgr.GetCache().Get(context.Background(), types.NamespacedName{
		Namespace: s.TestNamespace, Name: policyName,
	}, &np)
	s.Require().NoError(err)

	err = s.Mgr.GetClient().Delete(context.Background(), intents, &client.DeleteOptions{GracePeriodSeconds: lo.ToPtr(int64(0))})
	s.Require().NoError(err)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	s.WaitForDeletionToBeMarked(intents)

	res = ctrl.Result{Requeue: true}
	for res.Requeue {
		res, err = s.Reconciler.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: s.TestNamespace,
				Name:      intents.Name,
			},
		})
		s.Require().NoError(err)
		s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
	}

	deletedPolicy := v1.NetworkPolicy{}
	// Policy should have been deleted because intents were removed
	err = s.Mgr.GetCache().Get(context.Background(), types.NamespacedName{
		Namespace: s.TestNamespace, Name: policyName,
	}, &deletedPolicy)

	// We expect an error to have occurred
	s.Require().Error(err)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNetworkPolicyCleanupCrossNamespace() {
	otherNamespace := "test-cross-namespace-delete"
	_, err := s.K8sDirectClient.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: otherNamespace}}, metav1.CreateOptions{})
	s.Require().NoError(err)

	intents := s.AddIntents(
		"cross-ns-cleanup-test", "test-client", []otterizev1alpha1.Intent{{
			Type: otterizev1alpha1.IntentTypeHTTP, Name: "test-server", Namespace: otherNamespace}})

	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
	res, err := s.Reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
	s.Require().NoError(err)
	s.Require().Empty(res)

	np := v1.NetworkPolicy{}
	policyName := fmt.Sprintf(OtterizeNetworkPolicyNameTemplate, "test-server", s.TestNamespace)
	err = s.Mgr.GetCache().Get(context.Background(), types.NamespacedName{
		Namespace: otherNamespace, Name: policyName,
	}, &np)
	s.Require().NoError(err)

	err = s.Mgr.GetClient().Delete(context.Background(), intents, &client.DeleteOptions{GracePeriodSeconds: lo.ToPtr(int64(0))})
	s.Require().NoError(err)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	s.WaitForDeletionToBeMarked(intents)

	res = ctrl.Result{Requeue: true}
	for res.Requeue {
		res, err = s.Reconciler.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: s.TestNamespace,
				Name:      intents.Name,
			},
		})
		s.Require().NoError(err)
		s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
	}

	deletedPolicy := v1.NetworkPolicy{}
	// Policy should have been deleted because intents were removed
	err = s.Mgr.GetCache().Get(context.Background(), types.NamespacedName{
		Namespace: otherNamespace, Name: policyName,
	}, &deletedPolicy)

	// We expect an error to have occurred
	s.Require().Error(err)
}

func (s *NetworkPolicyReconcilerTestSuite) TestPolicyNotDeletedForTwoClientsWithSameServer() {
	_ = s.AddIntents(
		"no-cleanup-test", "test-client", []otterizev1alpha1.Intent{{
			Type: otterizev1alpha1.IntentTypeHTTP, Name: "test-server"}})

	intents := s.AddIntents(
		"other-no-cleanup-test", "test-client-2", []otterizev1alpha1.Intent{{
			Type: otterizev1alpha1.IntentTypeHTTP, Name: "test-server"}})

	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
	res, err := s.Reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
	s.Require().NoError(err)
	s.Require().Empty(res)

	np := v1.NetworkPolicy{}
	policyName := fmt.Sprintf(OtterizeNetworkPolicyNameTemplate, "test-server", s.TestNamespace)
	err = s.Mgr.GetCache().Get(context.Background(), types.NamespacedName{
		Namespace: s.TestNamespace, Name: policyName,
	}, &np)
	s.Require().NoError(err)

	// We delete one of the policies, and expect the reconciler to NOT REMOVE the policy since there's another
	// Intents resource that contains that server
	err = s.Mgr.GetClient().Delete(context.Background(), intents, &client.DeleteOptions{GracePeriodSeconds: lo.ToPtr(int64(0))})
	s.Require().NoError(err)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	s.WaitForDeletionToBeMarked(intents)

	res = ctrl.Result{Requeue: true}
	for res.Requeue {
		res, err = s.Reconciler.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: s.TestNamespace,
				Name:      intents.Name,
			},
		})
		s.Require().NoError(err)
		s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
	}

	np = v1.NetworkPolicy{}
	// Policy should have been deleted because intents were removed
	err = s.Mgr.GetCache().Get(context.Background(), types.NamespacedName{
		Namespace: s.TestNamespace, Name: policyName,
	}, &np)

	// We expect an error to have occurred
	s.Require().NoError(err)
}

func (s *NetworkPolicyReconcilerTestSuite) initServerIndices(mgr manager.Manager) error {
	err := mgr.GetCache().IndexField(
		context.Background(),
		&otterizev1alpha1.ClientIntents{},
		otterizev1alpha1.OtterizeTargetServerIndexField,
		func(object client.Object) []string {
			var res []string
			intents := object.(*otterizev1alpha1.ClientIntents)
			if intents.Spec == nil {
				return nil
			}

			for _, intent := range intents.GetCallsList() {
				res = append(res, intent.Name)
			}

			return res
		})

	if err != nil {
		return err
	}
	return nil
}

func TestNetworkPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(NetworkPolicyReconcilerTestSuite))
}
