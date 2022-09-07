package intents_reconcilers

import (
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type PodLabelReconcilerTestSuite struct {
	testbase.ControllerManagerTestSuiteBase
	Reconciler *PodLabelReconciler
}

func (p *PodLabelReconcilerTestSuite) SetupSuite() {
	p.TestEnv = &envtest.Environment{}
	var err error
	p.TestEnv.CRDDirectoryPaths = []string{"../../config/crd/bases"}

	p.RestConfig, err = p.TestEnv.Start()
	p.Require().NoError(err)
	p.Require().NotNil(p.RestConfig)

	p.K8sDirectClient, err = kubernetes.NewForConfig(p.RestConfig)
	p.Require().NoError(err)
	p.Require().NotNil(p.K8sDirectClient)

	err = otterizev1alpha1.AddToScheme(p.TestEnv.Scheme)
	p.Require().NoError(err)
}

func (p *PodLabelReconcilerTestSuite) BeforeTest(_, testName string) {
	p.ControllerManagerTestSuiteBase.BeforeTest("", testName)
}

func (p *PodLabelReconcilerTestSuite) SetupTest() {
	p.ControllerManagerTestSuiteBase.SetupTest()
	p.Reconciler = NewPodLabelReconciler(p.Mgr.GetClient(), p.TestEnv.Scheme)
	recorder := p.Mgr.GetEventRecorderFor("intents-operator")
	p.Reconciler.InjectRecorder(recorder)
}
