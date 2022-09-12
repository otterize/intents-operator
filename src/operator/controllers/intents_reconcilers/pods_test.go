package intents_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"testing"
)

type PodLabelReconcilerTestSuite struct {
	testbase.ControllerManagerTestSuiteBase
	Reconciler *PodLabelReconciler
}

func (s *PodLabelReconcilerTestSuite) SetupSuite() {
	s.TestEnv = &envtest.Environment{}
	var err error
	s.TestEnv.CRDDirectoryPaths = []string{"../../config/crd/bases"}

	s.RestConfig, err = s.TestEnv.Start()
	s.Require().NoError(err)
	s.Require().NotNil(s.RestConfig)

	s.K8sDirectClient, err = kubernetes.NewForConfig(s.RestConfig)
	s.Require().NoError(err)
	s.Require().NotNil(s.K8sDirectClient)

	err = otterizev1alpha1.AddToScheme(s.TestEnv.Scheme)
	s.Require().NoError(err)
}

func (s *PodLabelReconcilerTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()
	s.Reconciler = NewPodLabelReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme)
	recorder := s.Mgr.GetEventRecorderFor("intents-operator")
	s.Reconciler.InjectRecorder(recorder)
}

func (s *PodLabelReconcilerTestSuite) TestClientAccessLabelAdded() {
	deploymentName := "whocares"
	intentTargetServerName := "test-server"
	otterizeSvcIdentity := otterizev1alpha1.GetFormattedOtterizeIdentity(deploymentName, s.TestNamespace)
	// We add the otterize/server label here, to mock the watcher's behaviour
	s.AddDeployment("whocares", []string{"1.1.1.1"}, map[string]string{otterizev1alpha1.OtterizeServerLabelKey: otterizeSvcIdentity})

	intents := s.AddIntents("test-intents", deploymentName, []otterizev1alpha1.Intent{{
		Type: otterizev1alpha1.IntentTypeHTTP, Name: intentTargetServerName,
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

	pod := v1.Pod{}
	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{
		Namespace: s.TestNamespace, Name: fmt.Sprintf("%s-0", deploymentName)}, &pod)
	s.Require().NoError(err)
	s.Require().NotEmpty(pod)
	targetServerIdentity := otterizev1alpha1.GetFormattedOtterizeIdentity(
		intentTargetServerName, s.TestNamespace)

	accessLabel := fmt.Sprintf(otterizev1alpha1.OtterizeAccessLabelKey, targetServerIdentity)
	s.Require().Contains(pod.Labels, accessLabel)
}

func (s *PodLabelReconcilerTestSuite) TestClientAccessLabelRemovedAndAnnotationMarked() {
	// Tests for removal of intents for client + marking annotations of "All intents removed"
	deploymentName := "whocares"
	intentTargetServerName := "test-server"
	otterizeSvcIdentity := otterizev1alpha1.GetFormattedOtterizeIdentity(deploymentName, s.TestNamespace)
	targetServerIdentity := otterizev1alpha1.GetFormattedOtterizeIdentity(
		intentTargetServerName, s.TestNamespace)
	accessLabel := fmt.Sprintf(otterizev1alpha1.OtterizeAccessLabelKey, targetServerIdentity)

	intents := s.AddIntents("test-intents", deploymentName, []otterizev1alpha1.Intent{{
		Type: otterizev1alpha1.IntentTypeHTTP, Name: intentTargetServerName,
	},
	})

	s.AddDeployment(deploymentName, []string{"1.1.1.1"}, map[string]string{
		otterizev1alpha1.OtterizeServerLabelKey: otterizeSvcIdentity,
		accessLabel:                             "true"},
	)
	s.PatchAnnotations(fmt.Sprintf("%s-0", deploymentName), map[string]string{"a": "b"})
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

	err = s.Mgr.GetClient().Delete(context.Background(), intents)
	s.Require().NoError(err)
	s.WaitForDeletionToBeMarked(intents)

	res, err = s.Reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	pod := v1.Pod{}
	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{
		Namespace: s.TestNamespace, Name: fmt.Sprintf("%s-0", deploymentName)}, &pod)
	s.Require().NoError(err)
	s.Require().NotEmpty(pod)
	targetServerIdentity = otterizev1alpha1.GetFormattedOtterizeIdentity(
		intentTargetServerName, s.TestNamespace)

	s.Require().NotContains(pod.Labels, accessLabel)
	s.Require().Contains(pod.Annotations, otterizev1alpha1.AllIntentsRemoved)
}

func (s *PodLabelReconcilerTestSuite) TestClientAccessLabelRemoved() {
	deploymentName := "whocares"
	intentTargetServerName := "test-server"
	otterizeSvcIdentity := otterizev1alpha1.GetFormattedOtterizeIdentity(deploymentName, s.TestNamespace)
	targetServerIdentity := otterizev1alpha1.GetFormattedOtterizeIdentity(
		intentTargetServerName, s.TestNamespace)
	accessLabel := fmt.Sprintf(otterizev1alpha1.OtterizeAccessLabelKey, targetServerIdentity)

	intents := s.AddIntents("test-intents", deploymentName, []otterizev1alpha1.Intent{{
		Type: otterizev1alpha1.IntentTypeHTTP, Name: intentTargetServerName,
	},
	})

	s.AddDeployment(deploymentName, []string{"1.1.1.1"}, map[string]string{
		otterizev1alpha1.OtterizeServerLabelKey: otterizeSvcIdentity,
		accessLabel:                             "true"},
	)

	res, err := s.Reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	err = s.Mgr.GetClient().Delete(context.Background(), intents)
	s.Require().NoError(err)
	s.WaitForDeletionToBeMarked(intents)

	res, err = s.Reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	pod := v1.Pod{}
	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{
		Namespace: s.TestNamespace, Name: fmt.Sprintf("%s-0", deploymentName)}, &pod)
	s.Require().NoError(err)
	s.Require().NotEmpty(pod)
	targetServerIdentity = otterizev1alpha1.GetFormattedOtterizeIdentity(
		intentTargetServerName, s.TestNamespace)

	s.Require().NotContains(pod.Labels, accessLabel)
	s.Require().Contains(pod.Annotations, otterizev1alpha1.AllIntentsRemoved)
}

func TestPodLabelReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(PodLabelReconcilerTestSuite))
}
