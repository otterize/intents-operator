package intents_reconcilers

import (
	"context"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	istioschema "istio.io/client-go/pkg/apis/security/v1beta1"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"testing"
)

type IstioPolicyReconcilerTestSuite struct {
	testbase.ControllerManagerTestSuiteBase
	Reconciler *IstioPolicyReconciler
}

func (s *IstioPolicyReconcilerTestSuite) initServerIndices(mgr manager.Manager) error {
	err := mgr.GetCache().IndexField(
		context.Background(),
		&otterizev1alpha2.ClientIntents{},
		otterizev1alpha2.OtterizeTargetServerIndexField,
		func(object client.Object) []string {
			var res []string
			intents := object.(*otterizev1alpha2.ClientIntents)
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

func (s *IstioPolicyReconcilerTestSuite) SetupSuite() {
	s.TestEnv = &envtest.Environment{}
	var err error
	s.TestEnv.CRDDirectoryPaths = []string{
		filepath.Join("..", "..", "config", "crd"),
		filepath.Join("..", "..", "..", "..", "..", "istio", "istio", "manifests", "charts", "base", "crds", "crd-all.gen.yaml"),
	}
	// Add istio CRDs from istio.io/istio github repo
	s.TestEnv.ErrorIfCRDPathMissing = true
	s.RestConfig, err = s.TestEnv.Start()
	s.Require().NoError(err)
	s.Require().NotNil(s.RestConfig)

	s.K8sDirectClient, err = kubernetes.NewForConfig(s.RestConfig)
	s.Require().NoError(err)
	s.Require().NotNil(s.K8sDirectClient)

	err = otterizev1alpha2.AddToScheme(s.TestEnv.Scheme)
	s.Require().NoError(err)

	err = istioschema.AddToScheme(s.TestEnv.Scheme)
	s.Require().NoError(err)
}

func (s *IstioPolicyReconcilerTestSuite) BeforeTest(_, testName string) {
	s.Require().NoError(s.initServerIndices(s.Mgr))
	s.ControllerManagerTestSuiteBase.BeforeTest("", testName)
}

func (s *IstioPolicyReconcilerTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()
	s.Reconciler = NewIstioPolicyReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme, []string{}, true, true)
	recorder := s.Mgr.GetEventRecorderFor("intents-operator")
	s.Reconciler.InjectRecorder(recorder)
}

func (s *IstioPolicyReconcilerTestSuite) TestReconcile() {
	// Not implemented yet, just here for getting a simple breakpoint for debugging
	s.T().SkipNow()

	s.AddDeployment(
		"whocares",
		[]string{"1.1.1.1"},
		map[string]string{otterizev1alpha2.OtterizeServerLabelKey: "test-client"},
		map[string]string{})

	intents, err := s.AddIntents("test-intents", "test-client", []otterizev1alpha2.Intent{{
		Type: otterizev1alpha2.IntentTypeHTTP, Name: "test-server",
	},
	})
	s.Require().NoError(err)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	res, err := s.Reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)

}

func TestIstioPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(IstioPolicyReconcilerTestSuite))
}
