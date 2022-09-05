package intents_reconcilers

import "github.com/otterize/intents-operator/src/shared/testbase"

type NetworkPolicyReconcilerTestSuite struct {
	testbase.ControllerManagerTestSuiteBase
	Reconciler NetworkPolicyReconciler
}

func (s *NetworkPolicyReconcilerTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.AddCRDs("../../config/crd/bases")
	s.ControllerManagerTestSuiteBase.SetupTest()
	var err error
	s.Require().NoError(err)
}
