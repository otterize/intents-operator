package reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	istiosecurityscheme "istio.io/client-go/pkg/apis/security/v1beta1"
	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"testing"
)

type WatcherPodLabelReconcilerTestSuite struct {
	testbase.ControllerManagerTestSuiteBase
	Reconciler *PodWatcher
}

func (s *WatcherPodLabelReconcilerTestSuite) SetupSuite() {
	s.TestEnv = &envtest.Environment{}
	var err error
	s.TestEnv.CRDDirectoryPaths = []string{filepath.Join("..", "..", "operator", "config", "crd")}

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

func (s *WatcherPodLabelReconcilerTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()
	recorder := s.Mgr.GetEventRecorderFor("intents-operator")
	s.Reconciler = NewPodWatcher(s.Mgr.GetClient(), recorder, []string{})
	s.Require().NoError(s.Reconciler.InitIntentsClientIndices(s.Mgr))
}

func (s *WatcherPodLabelReconcilerTestSuite) TestServerLabelAddedWithNilLabels() {
	podName := "podname"
	intentTargetServerName := "test-server"

	s.AddPod(
		"podname",
		"1.1.1.1",
		nil,
		map[string]string{})

	pod := v1.Pod{}
	err := s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{
		Namespace: s.TestNamespace, Name: podName}, &pod)
	s.Require().NoError(err)

	serviceID, err := s.Reconciler.serviceIdResolver.ResolvePodToServiceIdentity(context.Background(), &pod)
	s.Require().NoError(err)

	thisPodIdentity := otterizev1alpha2.GetFormattedOtterizeIdentity(
		serviceID.Name, s.TestNamespace)

	_, err = s.AddIntents("test-intents", serviceID.Name, []otterizev1alpha2.Intent{{
		Type: otterizev1alpha2.IntentTypeHTTP, Name: intentTargetServerName,
	},
	})
	s.Require().NoError(err)

	// server label is added
	res, err := s.Reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      podName,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{
			Namespace: s.TestNamespace, Name: podName}, &pod)
		assert.NoError(err)
		assert.NotEmpty(pod)
		assert.Contains(pod.Labels, otterizev1alpha2.OtterizeServerLabelKey)
		assert.Equal(thisPodIdentity, pod.Labels[otterizev1alpha2.OtterizeServerLabelKey])

	})

	// access label is added
	res, err = s.Reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      podName,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	targetServerIdentity := otterizev1alpha2.GetFormattedOtterizeIdentity(
		intentTargetServerName, s.TestNamespace)
	accessLabel := fmt.Sprintf(otterizev1alpha2.OtterizeAccessLabelKey, targetServerIdentity)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{
			Namespace: s.TestNamespace, Name: podName}, &pod)
		assert.NoError(err)
		assert.NotEmpty(pod)
		assert.Contains(pod.Labels, accessLabel)
	})
}

func (s *WatcherPodLabelReconcilerTestSuite) TestClientAccessLabelAdded() {
	deploymentName := "deploymentname"
	intentTargetServerName := "test-server"

	s.AddDeployment(
		deploymentName,
		[]string{"1.1.1.1"},
		map[string]string{"someLabel": "cake"},
		map[string]string{})

	_, err := s.AddIntents("test-intents", deploymentName, []otterizev1alpha2.Intent{{
		Type: otterizev1alpha2.IntentTypeHTTP, Name: intentTargetServerName,
	},
	})
	s.Require().NoError(err)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	podName := fmt.Sprintf("%s-0", deploymentName)

	// server label is added
	res, err := s.Reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      podName,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	pod := v1.Pod{}
	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{
		Namespace: s.TestNamespace, Name: podName}, &pod)
	s.Require().NoError(err)

	serviceID, err := s.Reconciler.serviceIdResolver.ResolvePodToServiceIdentity(context.Background(), &pod)
	s.Require().NoError(err)

	thisPodIdentity := otterizev1alpha2.GetFormattedOtterizeIdentity(
		serviceID.Name, s.TestNamespace)

	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{
			Namespace: s.TestNamespace, Name: podName}, &pod)
		assert.NoError(err)
		assert.NotEmpty(pod)
		assert.Contains(pod.Labels, otterizev1alpha2.OtterizeServerLabelKey)
		assert.Equal(thisPodIdentity, pod.Labels[otterizev1alpha2.OtterizeServerLabelKey])

	})

	// access label is added
	_, err = s.Reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      podName,
		},
	})

	targetServerIdentity := otterizev1alpha2.GetFormattedOtterizeIdentity(
		intentTargetServerName, s.TestNamespace)

	accessLabel := fmt.Sprintf(otterizev1alpha2.OtterizeAccessLabelKey, targetServerIdentity)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{
			Namespace: s.TestNamespace, Name: podName}, &pod)
		assert.NoError(err)
		assert.NotEmpty(pod)
		assert.Contains(pod.Labels, accessLabel)
	})
}

func TestPodLabelReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(WatcherPodLabelReconcilerTestSuite))
}
