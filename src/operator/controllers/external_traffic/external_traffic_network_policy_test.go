package external_traffic

import (
	"context"
	"fmt"
	"github.com/amit7itz/goset"
	"github.com/google/uuid"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	otterizev1beta1 "github.com/otterize/intents-operator/src/operator/api/v1beta1"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	otterizev2beta1 "github.com/otterize/intents-operator/src/operator/api/v2beta1"
	"github.com/otterize/intents-operator/src/operator/controllers"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/networkpolicy"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/networkpolicy/builders"
	"github.com/otterize/intents-operator/src/operator/controllers/pod_reconcilers"
	podreconcilersmocks "github.com/otterize/intents-operator/src/operator/controllers/pod_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/operator/controllers/protected_service_reconcilers"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/operator/webhooks"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/automate_third_party_network_policy"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	istiosecurityscheme "istio.io/client-go/pkg/apis/security/v1beta1"
	v1 "k8s.io/api/networking/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"strings"
	"testing"
)

type ExternalNetworkPolicyReconcilerTestSuite struct {
	testbase.ControllerManagerTestSuiteBase
	IngressReconciler                *IngressReconciler
	endpointReconciler               EndpointsReconciler
	EffectivePolicyIntentsReconciler *intents_reconcilers.ServiceEffectivePolicyIntentsReconciler
	podWatcher                       *pod_reconcilers.PodWatcher
	defaultDenyReconciler            *protected_service_reconcilers.DefaultDenyReconciler
}

func (s *ExternalNetworkPolicyReconcilerTestSuite) SetupSuite() {
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
	utilruntime.Must(otterizev1beta1.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(otterizev2alpha1.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(otterizev2beta1.AddToScheme(s.TestEnv.Scheme))

	s.RestConfig, err = s.TestEnv.Start()
	s.Require().NoError(err)
	s.Require().NotNil(s.RestConfig)

	s.K8sDirectClient, err = kubernetes.NewForConfig(s.RestConfig)
	s.Require().NoError(err)
	s.Require().NotNil(s.K8sDirectClient)
}

func (s *ExternalNetworkPolicyReconcilerTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()
	intentsValidator := webhooks.NewIntentsValidatorV1alpha2(s.Mgr.GetClient())
	s.Require().NoError((&otterizev1alpha2.ClientIntents{}).SetupWebhookWithManager(s.Mgr, intentsValidator))
	intentsValidator13 := webhooks.NewIntentsValidatorV1alpha3(s.Mgr.GetClient())
	s.Require().NoError((&otterizev1alpha3.ClientIntents{}).SetupWebhookWithManager(s.Mgr, intentsValidator13))
	intentsValidator2 := webhooks.NewIntentsValidatorV2alpha1(s.Mgr.GetClient())
	s.Require().NoError((&otterizev2alpha1.ClientIntents{}).SetupWebhookWithManager(s.Mgr, intentsValidator2))
	intentsValidator2beta1 := webhooks.NewIntentsValidatorV2beta1(s.Mgr.GetClient())
	s.Require().NoError((&otterizev2beta1.ClientIntents{}).SetupWebhookWithManager(s.Mgr, intentsValidator2beta1))

	recorder := s.Mgr.GetEventRecorderFor("intents-operator")
	testName := s.T().Name()
	isShadowMode := strings.Contains(testName, "ShadowMode")
	defaultActive := !isShadowMode
	netpolHandler := NewNetworkPolicyHandler(s.Mgr.GetClient(), s.TestEnv.Scheme, automate_third_party_network_policy.IfBlockedByOtterize, make([]serviceidentity.ServiceIdentity, 0), false)
	s.defaultDenyReconciler = protected_service_reconcilers.NewDefaultDenyReconciler(s.Mgr.GetClient(), true)
	netpolReconciler := networkpolicy.NewReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme, netpolHandler, []string{}, goset.NewSet[string](), true, defaultActive, false, []networkpolicy.IngressRuleBuilder{builders.NewIngressNetpolBuilder(), builders.NewPortNetworkPolicyReconciler(s.Mgr.GetClient())}, nil)
	serviceIdResolver := serviceidresolver.NewResolver(s.Mgr.GetClient())
	epReconciler := effectivepolicy.NewGroupReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme, serviceIdResolver, netpolReconciler)
	s.EffectivePolicyIntentsReconciler = intents_reconcilers.NewServiceEffectiveIntentsReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme, epReconciler)
	s.Require().NoError((&controllers.IntentsReconciler{}).InitIntentsServerIndices(s.Mgr))
	s.EffectivePolicyIntentsReconciler.InjectRecorder(recorder)

	s.endpointReconciler = NewEndpointsReconciler(s.Mgr.GetClient(), netpolHandler)
	s.endpointReconciler.InjectRecorder(recorder)
	err := s.endpointReconciler.InitIngressReferencedServicesIndex(s.Mgr)
	s.Require().NoError(err)

	s.IngressReconciler = NewIngressReconciler(s.Mgr.GetClient(), netpolHandler)
	s.IngressReconciler.InjectRecorder(recorder)
	s.Require().NoError(err)

	controller := gomock.NewController(s.T())
	serviceEffectivePolicyReconciler := podreconcilersmocks.NewMockGroupReconciler(controller)
	s.podWatcher = pod_reconcilers.NewPodWatcher(s.Mgr.GetClient(), recorder, []string{}, defaultActive, true, goset.NewSet[string](), &mocks.MockIntentsReconcilerForTestEnv{}, serviceEffectivePolicyReconciler)
	err = s.podWatcher.InitIntentsClientIndices(s.Mgr)
	s.Require().NoError(err)

	err = (&controllers.IntentsReconciler{}).InitEndpointsPodNamesIndex(s.Mgr)
	s.Require().NoError(err)
}

func (s *ExternalNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateForIngress() {
	serviceName := "test-server-ingress-test"
	intents, err := s.AddIntents("test-intents", "test-client", "Deployment", []otterizev2alpha1.Target{{
		Kubernetes: &otterizev2alpha1.KubernetesTarget{
			Name: serviceName,
		},
	},
	})
	s.Require().NoError(err)

	res, err := s.EffectivePolicyIntentsReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	// make sure the network policy was created between the two services based on the intents
	np := &v1.NetworkPolicy{}
	policyName := fmt.Sprintf(otterizev2alpha1.OtterizeSingleNetworkPolicyNameTemplate, serviceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: policyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
	})

	s.AddDeploymentWithService(serviceName, []string{"1.1.1.1"}, map[string]string{"app": "test"}, nil)

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err = s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	// make sure the ingress network policy doesn't exist yet
	externalNetworkPolicyName := fmt.Sprintf(OtterizeExternalNetworkPolicyNameTemplate, serviceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
		assert.True(errors.IsNotFound(err))
	})
	s.AddIngress(serviceName)

	res, err = s.IngressReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      serviceName + "-ingress",
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)

	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
	})
}

func (s *ExternalNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateForIngressWithIntentToSVC() {
	serviceName := "test-server-ingress-test"
	intents, err := s.AddIntents("test-intents", "test-client", "Deployment", []otterizev2alpha1.Target{{Service: &otterizev2alpha1.ServiceTarget{Name: serviceName}}})
	s.Require().NoError(err)
	s.AddDeploymentWithService(serviceName, []string{"1.1.1.1"}, map[string]string{"app": "test"}, nil)

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err = s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	res, err := s.EffectivePolicyIntentsReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	// make sure the network policy was created between the two services based on the intents
	np := &v1.NetworkPolicy{}
	policyName := fmt.Sprintf(otterizev2alpha1.OtterizeSingleNetworkPolicyNameTemplate, fmt.Sprintf("%s-service", serviceName))
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: policyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
	})

	// make sure the ingress network policy doesn't exist yet
	externalNetworkPolicyName := fmt.Sprintf(OtterizeExternalNetworkPolicyNameTemplate, serviceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
		assert.True(errors.IsNotFound(err))
	})
	s.AddIngress(serviceName)

	res, err = s.IngressReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      serviceName + "-ingress",
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)

	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
	})
}

func (s *ExternalNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateForIngressWithIntentToDeployment() {
	serviceName := "test-server-ingress-test"
	intents, err := s.AddIntents("test-intents", "test-client", "Deployment", []otterizev2alpha1.Target{{
		Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: serviceName, Kind: "Deployment"},
	},
	})
	s.Require().NoError(err)
	s.AddDeploymentWithService(serviceName, []string{"1.1.1.1"}, map[string]string{"app": "test"}, nil)

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err = s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	res, err := s.EffectivePolicyIntentsReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	// make sure the network policy was created between the two services based on the intents
	np := &v1.NetworkPolicy{}
	policyName := fmt.Sprintf(otterizev2alpha1.OtterizeSingleNetworkPolicyNameTemplate, fmt.Sprintf("%s-deployment", serviceName))
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: policyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
	})

	// make sure the ingress network policy doesn't exist yet
	externalNetworkPolicyName := fmt.Sprintf(OtterizeExternalNetworkPolicyNameTemplate, serviceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
		assert.True(errors.IsNotFound(err))
	})
	s.AddIngress(serviceName)

	res, err = s.IngressReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      serviceName + "-ingress",
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)

	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
	})
}

func (s *ExternalNetworkPolicyReconcilerTestSuite) TestIngressProtectedService_ShadowMode() {
	serviceName := "test-server-ingress-test"

	s.AddDeploymentWithService(serviceName, []string{"1.1.1.1"}, map[string]string{"app": "test"}, nil)

	protectedServiceResourceName := "test-protected-service"
	protectedService, err := s.AddProtectedService(protectedServiceResourceName, serviceName, s.TestNamespace)
	s.Require().NoError(err)
	s.Require().NotNil(protectedService)

	protectedService = &otterizev2alpha1.ProtectedService{}
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: protectedServiceResourceName}, protectedService)
		assert.NoError(err)
		assert.NotEmpty(protectedService)
	})

	res, err := s.defaultDenyReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      protectedServiceResourceName,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	defaultDenyPolicy := &v1.NetworkPolicy{}
	defaultDenyPolicyName := fmt.Sprintf("default-deny-%s", serviceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: defaultDenyPolicyName}, defaultDenyPolicy)
		assert.NoError(err)
		assert.NotEmpty(defaultDenyPolicy)
	})

	res, err = s.EffectivePolicyIntentsReconciler.Reconcile(context.Background(), ctrl.Request{})
	s.Require().NoError(err)
	s.Require().Empty(res)

	// make sure the network policy was created between the two services based on the intents
	np := &v1.NetworkPolicy{}
	policyName := fmt.Sprintf(otterizev2alpha1.OtterizeSingleNetworkPolicyNameTemplate, serviceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: policyName}, np)
		assert.True(errors.IsNotFound(err))
	})

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err = s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	// make sure the ingress network policy doesn't exist yet
	externalNetworkPolicyName := fmt.Sprintf(OtterizeExternalNetworkPolicyNameTemplate, serviceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
		assert.True(errors.IsNotFound(err))
	})
	s.AddIngress(serviceName)

	res, err = s.IngressReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      serviceName + "-ingress",
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)

	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
	})
}

func (s *ExternalNetworkPolicyReconcilerTestSuite) TestIngressWithIntentsProtectedService_ShadowMode() {
	serviceName := "test-server-ingress-test"

	s.AddDeploymentWithService(serviceName, []string{"1.1.1.1"}, map[string]string{"app": "test"}, nil)

	protectedServiceResourceName := "test-protected-service"
	protectedService, err := s.AddProtectedService(protectedServiceResourceName, serviceName, s.TestNamespace)
	s.Require().NoError(err)
	s.Require().NotNil(protectedService)

	protectedService = &otterizev2alpha1.ProtectedService{}
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: protectedServiceResourceName}, protectedService)
		assert.NoError(err)
		assert.NotEmpty(protectedService)
	})

	res, err := s.defaultDenyReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      protectedServiceResourceName,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	defaultDenyPolicy := &v1.NetworkPolicy{}
	defaultDenyPolicyName := fmt.Sprintf("default-deny-%s", serviceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: defaultDenyPolicyName}, defaultDenyPolicy)
		assert.NoError(err)
		assert.NotEmpty(defaultDenyPolicy)
	})

	_, err = s.AddIntents("test-intents", "test-client", "Deployment", []otterizev2alpha1.Target{{
		Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: serviceName},
	},
	})
	s.Require().NoError(err)

	res, err = s.EffectivePolicyIntentsReconciler.Reconcile(context.Background(), ctrl.Request{})
	s.Require().NoError(err)
	s.Require().Empty(res)

	// make sure the network policy was created between the two services based on the intents
	np := &v1.NetworkPolicy{}
	policyName := fmt.Sprintf(otterizev2alpha1.OtterizeSingleNetworkPolicyNameTemplate, serviceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: policyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
	})

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err = s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	// make sure the ingress network policy doesn't exist yet
	externalNetworkPolicyName := fmt.Sprintf(OtterizeExternalNetworkPolicyNameTemplate, serviceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
		assert.True(errors.IsNotFound(err))
	})
	s.AddIngress(serviceName)

	res, err = s.IngressReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      serviceName + "-ingress",
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)

	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
	})
}

func (s *ExternalNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateForLoadBalancer() {
	serviceName := "test-server-load-balancer-test"
	intents, err := s.AddIntents("test-intents", "test-client", "Deployment", []otterizev2alpha1.Target{{
		Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: serviceName},
	},
	})
	s.Require().NoError(err)

	res, err := s.EffectivePolicyIntentsReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	// make sure the network policy was created between the two services based on the intents
	np := &v1.NetworkPolicy{}
	policyName := fmt.Sprintf(otterizev2alpha1.OtterizeSingleNetworkPolicyNameTemplate, serviceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: policyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
	})

	podIps := []string{"1.1.2.1"}
	podLabels := map[string]string{"app": "test-load-balancer"}
	s.AddDeploymentWithService(serviceName, podIps, podLabels, nil)

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err = s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	// make sure the load balancer network policy doesn't exist yet
	loadBalancerServiceName := serviceName + "-lb"
	externalNetworkPolicyName := fmt.Sprintf(OtterizeExternalNetworkPolicyNameTemplate, loadBalancerServiceName)

	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
		assert.True(errors.IsNotFound(err))
	})

	s.AddLoadBalancerService(loadBalancerServiceName, podIps, podLabels)
	res, err = s.endpointReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      loadBalancerServiceName,
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)

	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
	})
}

// This one is flaky
func (s *ExternalNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateForLoadBalancerCreatedAndDeletedWhenLastIntentDeleted() {
	serviceName := "test-server-load-balancer-test"
	intents, err := s.AddIntents("test-intents", "test-client", "Deployment", []otterizev2alpha1.Target{{
		Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: serviceName},
	},
	})
	s.Require().NoError(err)

	res, err := s.EffectivePolicyIntentsReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	// make sure the network policy was created between the two services based on the intents
	netpol := &v1.NetworkPolicy{}
	intentNetworkPolicyName := fmt.Sprintf(otterizev2alpha1.OtterizeSingleNetworkPolicyNameTemplate, serviceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: intentNetworkPolicyName}, netpol)
		assert.NoError(err)
		assert.NotEmpty(netpol)
	})

	podIps := []string{"1.1.2.1"}
	podLabels := map[string]string{"app": "test-load-balancer"}
	s.AddDeploymentWithService(serviceName, podIps, podLabels, nil)

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err = s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	// make sure the load balancer network policy doesn't exist yet
	loadBalancerServiceName := serviceName + "-lb"
	externalNetworkPolicyName := fmt.Sprintf(OtterizeExternalNetworkPolicyNameTemplate, loadBalancerServiceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, netpol)
		assert.True(errors.IsNotFound(err))
	})

	s.AddLoadBalancerService(loadBalancerServiceName, podIps, podLabels)
	res, err = s.endpointReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      loadBalancerServiceName,
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)

	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, netpol)
		assert.NoError(err)
		assert.NotEmpty(netpol)
	})

	// Delete the intent and reconcile it
	s.Require().NoError(s.Mgr.GetClient().Delete(context.Background(), intents))
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		intentsDeleted := &otterizev2alpha1.ClientIntents{}
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: intents.Name}, intentsDeleted)
		assert.NoError(err)
		assert.NotNil(intentsDeleted.DeletionTimestamp)
	})

	s.WaitUntilCondition(func(assert *assert.Assertions) {
		res, err = s.EffectivePolicyIntentsReconciler.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: s.TestNamespace,
				Name:      intents.Name,
			},
		})
		s.Require().NoError(err)
		s.Require().Empty(res)
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: intentNetworkPolicyName}, &v1.NetworkPolicy{})
		assert.True(errors.IsNotFound(err))
	})

	s.WaitUntilCondition(func(assert *assert.Assertions) {
		res, err = s.EffectivePolicyIntentsReconciler.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: s.TestNamespace,
				Name:      intents.Name,
			},
		})
		s.Require().NoError(err)
		s.Require().Empty(res)
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, &v1.NetworkPolicy{})
		assert.True(errors.IsNotFound(err))
	})
}

func (s *ExternalNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateForLoadBalancerCreatedAndDoesNotGetDeletedEvenWhenIntentRemovedAsLongAsOneRemains() {
	serviceName := "test-server-load-balancer-test"
	intents, err := s.AddIntents("test-intents", "test-client", "Deployment", []otterizev2alpha1.Target{{
		Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: serviceName},
	},
	})
	s.Require().NoError(err)

	secondaryNamespace := "ns-" + uuid.New().String() + "e"
	s.CreateNamespace(secondaryNamespace)
	secondIntents, err := s.AddIntentsInNamespace("test-intents-other-ns", "test-client-other-ns", "", secondaryNamespace, []otterizev2alpha1.Target{{
		Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: fmt.Sprintf("%s.%s", serviceName, s.TestNamespace)},
	}})
	s.Require().NoError(err)

	res, err := s.EffectivePolicyIntentsReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	res2, err := s.EffectivePolicyIntentsReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: secondaryNamespace,
			Name:      secondIntents.Name,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res2)

	// make sure the network policy was created between the two services based on the intents
	np := &v1.NetworkPolicy{}
	intentNetworkPolicyName := fmt.Sprintf(otterizev2alpha1.OtterizeSingleNetworkPolicyNameTemplate, serviceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: intentNetworkPolicyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
		assert.Len(np.Spec.Ingress, 2)
	})

	podIps := []string{"1.1.2.1"}
	podLabels := map[string]string{"app": "test-load-balancer"}
	s.AddDeploymentWithService(serviceName, podIps, podLabels, nil)

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err = s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	// make sure the load balancer network policy doesn't exist yet
	loadBalancerServiceName := serviceName + "-lb"
	externalNetworkPolicyName := fmt.Sprintf(OtterizeExternalNetworkPolicyNameTemplate, loadBalancerServiceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
		assert.True(errors.IsNotFound(err))
	})

	s.AddLoadBalancerService(loadBalancerServiceName, podIps, podLabels)
	res, err = s.endpointReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      loadBalancerServiceName,
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)

	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
	})

	// Delete the intent and reconcile it
	s.Require().NoError(s.Mgr.GetClient().Delete(context.Background(), intents))
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		intentsDeleted := &otterizev2alpha1.ClientIntents{}
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: intents.Name}, intentsDeleted)
		assert.NoError(err)
		assert.NotNil(intentsDeleted.DeletionTimestamp)
	})

	res, err = s.EffectivePolicyIntentsReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	s.WaitUntilCondition(func(assert *assert.Assertions) {
		netpol := &v1.NetworkPolicy{}
		_ = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: intentNetworkPolicyName}, netpol)
		assert.NotNil(netpol)
		assert.Len(netpol.Spec.Ingress, 1)
	})

	// Check that external policy was not deleted.
	externalNetpol := &v1.NetworkPolicy{}
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, externalNetpol)
		assert.NoError(err)
		assert.Nil(externalNetpol.DeletionTimestamp)
	})
}

func (s *ExternalNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateForNodePort() {
	serviceName := "test-server-node-port-test"
	intents, err := s.AddIntents("test-intents", "test-client", "Deployment", []otterizev2alpha1.Target{{
		Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: serviceName},
	},
	})
	s.Require().NoError(err)

	res, err := s.EffectivePolicyIntentsReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	// make sure the network policy was created between the two services based on the intents
	np := &v1.NetworkPolicy{}
	policyName := fmt.Sprintf(otterizev2alpha1.OtterizeSingleNetworkPolicyNameTemplate, serviceName)

	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: policyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
	})

	podIps := []string{"1.1.2.1"}
	podLabels := map[string]string{"app": "test-load-balancer"}
	s.AddDeploymentWithService(serviceName, podIps, podLabels, nil)

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err = s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	// make sure the load balancer network policy doesn't exist yet
	nodePortServiceName := serviceName + "-np"
	externalNetworkPolicyName := fmt.Sprintf(OtterizeExternalNetworkPolicyNameTemplate, nodePortServiceName)

	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
		assert.True(errors.IsNotFound(err))
	})

	s.AddNodePortService(nodePortServiceName, podIps, podLabels)
	res, err = s.endpointReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      nodePortServiceName,
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
	})
}

func (s *ExternalNetworkPolicyReconcilerTestSuite) TestEndpointsReconcilerNetworkPoliciesDisabled() {
	serviceName := "test-endpoints-reconciler-enforcement-disabled"
	intents, err := s.AddIntents("test-intents", "test-client", "Deployment", []otterizev2alpha1.Target{{
		Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: serviceName},
	},
	})
	s.Require().NoError(err)

	res, err := s.EffectivePolicyIntentsReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	// make sure the network policy was created between the two services based on the intents
	np := &v1.NetworkPolicy{}
	policyName := fmt.Sprintf(otterizev2alpha1.OtterizeSingleNetworkPolicyNameTemplate, serviceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: policyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
	})

	podIps := []string{"1.1.2.1"}
	podLabels := map[string]string{"app": "test-load-balancer"}
	s.AddDeploymentWithService(serviceName, podIps, podLabels, nil)

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err = s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	// make sure the load balancer network policy doesn't exist yet
	nodePortServiceName := serviceName + "-np"
	externalNetworkPolicyName := fmt.Sprintf(OtterizeExternalNetworkPolicyNameTemplate, nodePortServiceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
		assert.True(errors.IsNotFound(err))
	})

	s.AddNodePortService(nodePortServiceName, podIps, podLabels)

	netpolHandler := NewNetworkPolicyHandler(s.Mgr.GetClient(), s.TestEnv.Scheme, automate_third_party_network_policy.Off, make([]serviceidentity.ServiceIdentity, 0), false)
	endpointReconcilerWithEnforcementDisabled := NewEndpointsReconciler(s.Mgr.GetClient(), netpolHandler)
	recorder := record.NewFakeRecorder(10)
	endpointReconcilerWithEnforcementDisabled.InjectRecorder(recorder)

	res, err = endpointReconcilerWithEnforcementDisabled.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      nodePortServiceName,
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
		assert.True(errors.IsNotFound(err))
	})
	s.ExpectNoEvent(recorder)
}

func TestExternalNetworkPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(ExternalNetworkPolicyReconcilerTestSuite))
}
