package intents_reconcilers

import (
	"context"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"istio.io/client-go/pkg/apis/security/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"testing"
)

type IstioPolicyReconcilerTestSuite struct {
	testbase.ControllerManagerTestSuiteBase
	Reconciler *IstioPolicyReconciler
}

func (s *IstioPolicyReconcilerTestSuite) SetupSuite() {
	s.TestEnv = &envtest.Environment{}
	var err error
	s.TestEnv.CRDDirectoryPaths = []string{filepath.Join("..", "..", "config", "crd")}

	s.RestConfig, err = s.TestEnv.Start()
	s.Require().NoError(err)
	s.Require().NotNil(s.RestConfig)

	s.K8sDirectClient, err = kubernetes.NewForConfig(s.RestConfig)
	s.Require().NoError(err)
	s.Require().NotNil(s.K8sDirectClient)

	err = otterizev1alpha2.AddToScheme(s.TestEnv.Scheme)
	s.Require().NoError(err)
}

func (s *IstioPolicyReconcilerTestSuite) BeforeTest(_, testName string) {
	s.ControllerManagerTestSuiteBase.BeforeTest("", testName)
	s.Reconciler = NewIstioPolicyReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme, []string{}, true, true)
	recorder := record.NewFakeRecorder(100)
	s.Reconciler.InjectRecorder(recorder)
}

func (s *IstioPolicyReconcilerTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()
}

func (s *IstioPolicyReconcilerTestSuite) TestGlobalEnforcementDisabled() {
	intents, err := s.AddIntents("test-intents", "test-client", []otterizev1alpha2.Intent{{
		Type: otterizev1alpha2.IntentTypeHTTP, Name: "test-server",
	},
	})
	s.Require().NoError(err)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	reconciler := NewIstioPolicyReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme, []string{}, true, false)
	recorder := record.NewFakeRecorder(100)
	reconciler.InjectRecorder(recorder)
	res, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)

	select {
	case event := <-recorder.Events:
		s.Require().Contains(event, ReasonEnforcementGloballyDisabled)
	default:
		s.Fail("event not raised")
	}
}

func (s *IstioPolicyReconcilerTestSuite) TestIstioPolicyEnforcementDisabled() {
	intents, err := s.AddIntents("test-intents", "test-client", []otterizev1alpha2.Intent{{
		Type: otterizev1alpha2.IntentTypeHTTP, Name: "test-server",
	},
	})
	s.Require().NoError(err)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	reconciler := NewIstioPolicyReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme, []string{}, false, true)
	recorder := record.NewFakeRecorder(100)
	reconciler.InjectRecorder(recorder)
	res, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)

	select {
	case event := <-recorder.Events:
		s.Require().Contains(event, ReasonIstioPolicyCreationDisabled)
	default:
		s.Fail("event not raised")
	}
}

func (s *IstioPolicyReconcilerTestSuite) TestIstioPolicyCreation() {
	intentObjectName := "client-intents"
	_ = s.AddDeploymentWithServiceAccount("client-deployment", "client-service-account", "")

	intents, err := s.AddIntents(intentObjectName, "test-client", []otterizev1alpha2.Intent{{
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
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	istioPolicyList := &v1beta1.AuthorizationPolicyList{}
	err = s.Mgr.GetClient().List(context.Background(), istioPolicyList, client.InNamespace(s.TestNamespace))
	s.Require().NoError(err)
	s.Require().Len(istioPolicyList.Items, 1)

}

func (s *IstioPolicyReconcilerTestSuite) TestIstioPolicyFinalizerAddedAndRemove() {
	intentObjectName := "finalizer-intents"
	intents, err := s.AddIntents(intentObjectName, "test-client", []otterizev1alpha2.Intent{{
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
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	intents = &otterizev1alpha2.ClientIntents{}
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intentObjectName,
		}, intents)
		assert.NoError(err)
		assert.NotEmpty(intents.Finalizers)
	})
	s.Require().Contains(intents.Finalizers, IstioPolicyFinalizerName)

	additionalFinalizerIntents := intents.DeepCopy()
	// We have to add another finalizer so the object won't actually be deleted after the reconciler finishes
	additionalFinalizerIntents.Finalizers = append(additionalFinalizerIntents.Finalizers, "finalizer-to-prevent-obj-deletion")
	err = s.Mgr.GetClient().Patch(context.Background(), additionalFinalizerIntents, client.MergeFrom(intents))
	s.Require().NoError(err)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

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

	intents = &otterizev1alpha2.ClientIntents{}
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetCache().Get(context.Background(), types.NamespacedName{
			Namespace: s.TestNamespace, Name: "finalizer-intents",
		}, intents)
		assert.True(len(intents.Finalizers) == 1 && intents.Finalizers[0] != otterizev1alpha2.NetworkPolicyFinalizerName)
	})
	s.Require().NotContains(intents.Finalizers, IstioPolicyFinalizerName)
}

func TestIstioPolicyReconcilerTestSuite(t *testing.T) {
	//suite.Run(t, new(IstioPolicyReconcilerTestSuite))
}
