package intents_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	s.TestEnv.CRDDirectoryPaths = []string{filepath.Join("..", "..", "config", "crd")}

	s.RestConfig, err = s.TestEnv.Start()
	s.Require().NoError(err)
	s.Require().NotNil(s.RestConfig)

	s.K8sDirectClient, err = kubernetes.NewForConfig(s.RestConfig)
	s.Require().NoError(err)
	s.Require().NotNil(s.K8sDirectClient)

	utilruntime.Must(apiextensionsv1.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(istiosecurityscheme.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(otterizev1alpha2.AddToScheme(s.TestEnv.Scheme))
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
	otterizeSvcIdentity := otterizev1alpha2.GetFormattedOtterizeIdentity(deploymentName, s.TestNamespace)
	// We add the otterize/server label here, to mock the watcher's behaviour
	s.AddDeployment(
		"whocares",
		[]string{"1.1.1.1"},
		map[string]string{otterizev1alpha2.OtterizeServerLabelKey: otterizeSvcIdentity},
		map[string]string{})

	intents, err := s.AddIntents("test-intents", deploymentName, []otterizev1alpha2.Intent{{
		Type: otterizev1alpha2.IntentTypeHTTP, Name: intentTargetServerName,
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

	pod := v1.Pod{}
	targetServerIdentity := otterizev1alpha2.GetFormattedOtterizeIdentity(
		intentTargetServerName, s.TestNamespace)

	accessLabel := fmt.Sprintf(otterizev1alpha2.OtterizeAccessLabelKey, targetServerIdentity)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{
			Namespace: s.TestNamespace, Name: fmt.Sprintf("%s-0", deploymentName)}, &pod)
		assert.NoError(err)
		assert.NotEmpty(pod)
		assert.Contains(pod.Labels, accessLabel)
	})
}

func (s *PodLabelReconcilerTestSuite) testClientAccessLabelRemovedWithParams(podAnnotations map[string]string) {
	// Tests for removal of intents for client + marking annotations of "All intents removed"
	deploymentName := "whocares"
	intentTargetServerName := "test-server"
	otterizeSvcIdentity := otterizev1alpha2.GetFormattedOtterizeIdentity(deploymentName, s.TestNamespace)
	targetServerIdentity := otterizev1alpha2.GetFormattedOtterizeIdentity(
		intentTargetServerName, s.TestNamespace)
	accessLabel := fmt.Sprintf(otterizev1alpha2.OtterizeAccessLabelKey, targetServerIdentity)

	intents, err := s.AddIntents("test-intents", deploymentName, []otterizev1alpha2.Intent{{
		Type: otterizev1alpha2.IntentTypeHTTP, Name: intentTargetServerName,
	},
	})
	s.Require().NoError(err)
	s.AddDeployment(deploymentName, []string{"1.1.1.1"}, map[string]string{
		otterizev1alpha2.OtterizeServerLabelKey: otterizeSvcIdentity,
		accessLabel:                             "true"},
		podAnnotations,
	)
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
	s.Require().NotContains(pod.Labels, accessLabel)
	s.Require().Contains(pod.Annotations, otterizev1alpha2.AllIntentsRemovedAnnotation)
}

func (s *PodLabelReconcilerTestSuite) TestClientAccessLabelRemoved() {
	s.testClientAccessLabelRemovedWithParams(map[string]string{"a": "b"})
}

func (s *PodLabelReconcilerTestSuite) TestClientAccessLabelRemovedNoPodAnnotations() {
	s.testClientAccessLabelRemovedWithParams(nil)
}

func (s *PodLabelReconcilerTestSuite) TestAccessLabelChangedOnIntentsEdit() {
	deploymentName := "whocares"
	intentTargetServerName := "test-server"
	otterizeSvcIdentity := otterizev1alpha2.GetFormattedOtterizeIdentity(deploymentName, s.TestNamespace)

	// We add the otterize/server label here, to mock the watcher's behaviour
	s.AddDeployment(
		"whocares",
		[]string{"1.1.1.1"},
		map[string]string{otterizev1alpha2.OtterizeServerLabelKey: otterizeSvcIdentity},
		map[string]string{})

	intents, err := s.AddIntents("test-intents", deploymentName, []otterizev1alpha2.Intent{{
		Type: otterizev1alpha2.IntentTypeHTTP, Name: intentTargetServerName,
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

	targetServerIdentity := otterizev1alpha2.GetFormattedOtterizeIdentity(intentTargetServerName, s.TestNamespace)

	originalAccessLabel := fmt.Sprintf(otterizev1alpha2.OtterizeAccessLabelKey, targetServerIdentity)
	pod := v1.Pod{}
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{
			Namespace: s.TestNamespace, Name: fmt.Sprintf("%s-0", deploymentName)}, &pod)
		assert.NoError(err)
		assert.NotEmpty(pod)
		assert.Contains(pod.Labels, originalAccessLabel)
	})

	updatedIntents := intents.DeepCopy()
	updatedIntents.Spec.Calls[0].Name = "test-server-new"
	err = s.Mgr.GetClient().Patch(context.Background(), updatedIntents, client.MergeFrom(intents))
	s.Require().NoError(err)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
	s.Require().Empty(res)

	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err := s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{
			Namespace: s.TestNamespace, Name: "test-intents"}, intents)
		assert.NoError(err)
		assert.Equal(intents.Spec.Calls[0].Name, "test-server-new")
	})

	res, err = s.Reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	newTargetSrvIdentity := otterizev1alpha2.GetFormattedOtterizeIdentity(
		"test-server-new", s.TestNamespace)
	newAccessLabel := fmt.Sprintf(otterizev1alpha2.OtterizeAccessLabelKey, newTargetSrvIdentity)

	pod = v1.Pod{}
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err := s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{
			Namespace: s.TestNamespace, Name: fmt.Sprintf("%s-0", deploymentName)}, &pod)
		assert.NoError(err)
		assert.NotContains(pod.Labels, originalAccessLabel)
		assert.Contains(pod.Labels, newAccessLabel)
	})
}

func (s *PodLabelReconcilerTestSuite) TestPodLabelFinalizerAdded() {
	intentTargetServerName := "test-server"
	// We add the otterize/server label here, to mock the watcher's behaviour
	s.AddDeployment("whocares", []string{"1.1.1.1"}, map[string]string{"A": "b"}, map[string]string{})

	intents, err := s.AddIntents("test-intents", "abc", []otterizev1alpha2.Intent{{
		Type: otterizev1alpha2.IntentTypeHTTP, Name: intentTargetServerName,
	},
	})
	s.Require().NoError(err)
	res, err := s.Reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	intents = &otterizev1alpha2.ClientIntents{}
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{
			Namespace: s.TestNamespace, Name: "test-intents"}, intents)
		assert.NoError(err)
		assert.NotEmpty(intents.Finalizers)
	})
}

func (s *PodLabelReconcilerTestSuite) TestPodLabelFinalizerRemoved() {
	intents, err := s.AddIntents(
		"finalizer-intents", "test-client", []otterizev1alpha2.Intent{{
			Type: otterizev1alpha2.IntentTypeHTTP, Name: "test-server"}})
	s.Require().NoError(err)

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

	additionalFinalizerIntents := intents.DeepCopy()

	// We have to add another finalizer so the object won't actually be deleted after the netpol reconciler finishes
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
	// Policy should have been deleted because intents were removed
	err = s.Mgr.GetCache().Get(context.Background(), types.NamespacedName{
		Namespace: s.TestNamespace, Name: "finalizer-intents",
	}, intents)

	s.Require().NoError(err)
	s.Require().True(len(intents.Finalizers) == 1 && intents.Finalizers[0] != otterizev1alpha2.NetworkPolicyFinalizerName)
}

func TestPodLabelReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(PodLabelReconcilerTestSuite))
}
