package pod_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/operator/webhooks"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/sirupsen/logrus"
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
	logrus.Info("Setting up test suite")
	s.TestEnv = &envtest.Environment{Scheme: clientgoscheme.Scheme}
	var err error
	s.TestEnv.CRDDirectoryPaths = []string{filepath.Join("..", "..", "config", "crd")}
	s.TestEnv.WebhookInstallOptions = envtest.WebhookInstallOptions{
		Paths:            []string{filepath.Join("..", "..", "config", "webhook")},
		LocalServingHost: "localhost",
	}
	utilruntime.Must(apiextensionsv1.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(istiosecurityscheme.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(otterizev1alpha2.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(otterizev1alpha3.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(otterizev2alpha1.AddToScheme(s.TestEnv.Scheme))

	s.RestConfig, err = s.TestEnv.Start()
	s.Require().NoError(err)
	s.Require().NotNil(s.RestConfig)

	s.K8sDirectClient, err = kubernetes.NewForConfig(s.RestConfig)
	s.Require().NoError(err)
	s.Require().NotNil(s.K8sDirectClient)
}

func (s *WatcherPodLabelReconcilerTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()
	intentsValidator := webhooks.NewIntentsValidatorV1alpha2(s.Mgr.GetClient())
	s.Require().NoError((&otterizev1alpha2.ClientIntents{}).SetupWebhookWithManager(s.Mgr, intentsValidator))
	intentsValidator13 := webhooks.NewIntentsValidatorV1alpha3(s.Mgr.GetClient())
	s.Require().NoError((&otterizev1alpha3.ClientIntents{}).SetupWebhookWithManager(s.Mgr, intentsValidator13))
	intentsValidator2 := webhooks.NewIntentsValidatorV2alpha1(s.Mgr.GetClient())
	s.Require().NoError((&otterizev2alpha1.ClientIntents{}).SetupWebhookWithManager(s.Mgr, intentsValidator2))

	recorder := s.Mgr.GetEventRecorderFor("intents-operator")
	s.Reconciler = NewPodWatcher(s.Mgr.GetClient(), recorder, []string{}, true, true, nil, &mocks.MockIntentsReconcilerForTestEnv{})
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

	thisPodIdentity := (&serviceidentity.ServiceIdentity{Name: serviceID.Name, Namespace: s.TestNamespace}).GetFormattedOtterizeIdentityWithoutKind()

	_, err = s.AddIntents("test-intents", serviceID.Name, "", []otterizev2alpha1.Target{{
		Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: intentTargetServerName},
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
		assert.Contains(pod.Labels, otterizev2alpha1.OtterizeServiceLabelKey)
		assert.Equal(thisPodIdentity, pod.Labels[otterizev2alpha1.OtterizeServiceLabelKey])

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

	targetServerIdentity := (&serviceidentity.ServiceIdentity{Name: intentTargetServerName, Namespace: s.TestNamespace}).GetFormattedOtterizeIdentityWithoutKind()
	accessLabel := fmt.Sprintf(otterizev2alpha1.OtterizeAccessLabelKey, targetServerIdentity)
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

	_, err := s.AddIntents("test-intents", deploymentName, "Deployment", []otterizev2alpha1.Target{{
		Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: intentTargetServerName},
	},
	})
	s.Require().NoError(err)

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

	thisPodIdentity := (&serviceidentity.ServiceIdentity{Name: serviceID.Name, Namespace: s.TestNamespace}).GetFormattedOtterizeIdentityWithoutKind()

	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{
			Namespace: s.TestNamespace, Name: podName}, &pod)
		assert.NoError(err)
		assert.NotEmpty(pod)
		assert.Contains(pod.Labels, otterizev2alpha1.OtterizeServiceLabelKey)
		assert.Equal(thisPodIdentity, pod.Labels[otterizev2alpha1.OtterizeServiceLabelKey])

	})

	// access label is added
	_, err = s.Reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      podName,
		},
	})

	targetServerIdentity := (&serviceidentity.ServiceIdentity{Name: intentTargetServerName, Namespace: s.TestNamespace}).GetFormattedOtterizeIdentityWithoutKind()

	accessLabel := fmt.Sprintf(otterizev2alpha1.OtterizeAccessLabelKey, targetServerIdentity)
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
