package intents_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"testing"
)

type NetworkPolicyReconcilerTestSuite struct {
	testbase.ControllerManagerTestSuiteBase
	Reconciler NetworkPolicyReconciler
}

func (s *NetworkPolicyReconcilerTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()
	var err error
	s.Require().NoError(err)
}

func (s *NetworkPolicyReconcilerTestSuite) TestSomething() {
	//s.AddDeployment("nginx", []string{"1.1.1.1"}, map[string]string{"app": "test"})
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
	fmt.Printf("%v", res)
}

func TestNetworkPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(NetworkPolicyReconcilerTestSuite))
}
