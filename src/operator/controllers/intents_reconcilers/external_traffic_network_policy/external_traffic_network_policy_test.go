package external_traffic_network_policy

import (
	"context"
	"fmt"
	"github.com/amit7itz/goset"
	"github.com/google/uuid"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers"
	"github.com/otterize/intents-operator/src/operator/controllers/external_traffic"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/networkpolicy"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/networkpolicy/builders"
	"github.com/otterize/intents-operator/src/operator/controllers/pod_reconcilers"
	"github.com/otterize/intents-operator/src/operator/controllers/protected_service_reconcilers"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/allowexternaltraffic"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
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
	IngressReconciler                *external_traffic.IngressReconciler
	endpointReconciler               external_traffic.EndpointsReconciler
	EffectivePolicyIntentsReconciler *intents_reconcilers.ServiceEffectivePolicyIntentsReconciler
	podWatcher                       *pod_reconcilers.PodWatcher
	defaultDenyReconciler            *protected_service_reconcilers.DefaultDenyReconciler
}

func (s *ExternalNetworkPolicyReconcilerTestSuite) SetupSuite() {
	s.TestEnv = &envtest.Environment{}
	var err error
	s.TestEnv.CRDDirectoryPaths = []string{filepath.Join("..", "..", "..", "config", "crd")}

	s.RestConfig, err = s.TestEnv.Start()
	s.Require().NoError(err)
	s.Require().NotNil(s.RestConfig)

	s.K8sDirectClient, err = kubernetes.NewForConfig(s.RestConfig)
	s.Require().NoError(err)
	s.Require().NotNil(s.K8sDirectClient)

	utilruntime.Must(apiextensionsv1.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(istiosecurityscheme.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(otterizev1alpha3.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(otterizev1alpha3.AddToScheme(s.TestEnv.Scheme))
}

func (s *ExternalNetworkPolicyReconcilerTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()

	recorder := s.Mgr.GetEventRecorderFor("intents-operator")
	testName := s.T().Name()
	isShadowMode := strings.Contains(testName, "ShadowMode")
	defaultActive := !isShadowMode
	netpolHandler := external_traffic.NewNetworkPolicyHandler(s.Mgr.GetClient(), s.TestEnv.Scheme, allowexternaltraffic.IfBlockedByOtterize)
	s.defaultDenyReconciler = protected_service_reconcilers.NewDefaultDenyReconciler(s.Mgr.GetClient(), netpolHandler, true)
	netpolReconciler := networkpolicy.NewReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme, netpolHandler, []string{}, goset.NewSet[string](), true, defaultActive, []networkpolicy.IngressRuleBuilder{builders.NewIngressNetpolBuilder(), builders.NewPortNetworkPolicyReconciler(s.Mgr.GetClient())}, nil)
	serviceIdResolver := serviceidresolver.NewResolver(s.Mgr.GetClient())
	epReconciler := effectivepolicy.NewGroupReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme, serviceIdResolver, netpolReconciler)
	s.EffectivePolicyIntentsReconciler = intents_reconcilers.NewServiceEffectiveIntentsReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme, epReconciler)
	s.Require().NoError((&controllers.IntentsReconciler{}).InitIntentsServerIndices(s.Mgr))
	s.EffectivePolicyIntentsReconciler.InjectRecorder(recorder)

	s.endpointReconciler = external_traffic.NewEndpointsReconciler(s.Mgr.GetClient(), netpolHandler)
	s.endpointReconciler.InjectRecorder(recorder)
	err := s.endpointReconciler.InitIngressReferencedServicesIndex(s.Mgr)
	s.Require().NoError(err)

	s.IngressReconciler = external_traffic.NewIngressReconciler(s.Mgr.GetClient(), netpolHandler)
	s.IngressReconciler.InjectRecorder(recorder)
	s.Require().NoError(err)

	s.podWatcher = pod_reconcilers.NewPodWatcher(s.Mgr.GetClient(), recorder, []string{}, defaultActive, true, goset.NewSet[string](), &mocks.MockIntentsReconcilerForTestEnv{})
	err = s.podWatcher.InitIntentsClientIndices(s.Mgr)
	s.Require().NoError(err)

	err = (&controllers.IntentsReconciler{}).InitEndpointsPodNamesIndex(s.Mgr)
	s.Require().NoError(err)
}

func (s *ExternalNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateForIngress() {
	serviceName := "test-server-ingress-test"
	intents, err := s.AddIntents("test-intents", "test-client", "Deployment", []otterizev1alpha3.Intent{{
		Type: otterizev1alpha3.IntentTypeHTTP, Name: serviceName,
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
	policyName := fmt.Sprintf(otterizev1alpha3.OtterizeSingleNetworkPolicyNameTemplate, serviceName)
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
	externalNetworkPolicyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, serviceName)
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
	intents, err := s.AddIntents("test-intents", "test-client", "Deployment", []otterizev1alpha3.Intent{{
		Type: otterizev1alpha3.IntentTypeHTTP, Name: serviceName, Kind: serviceidentity.KindService,
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
	policyName := fmt.Sprintf(otterizev1alpha3.OtterizeSingleNetworkPolicyNameTemplate, fmt.Sprintf("%s-service", serviceName))
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: policyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
	})

	// make sure the ingress network policy doesn't exist yet
	externalNetworkPolicyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, serviceName)
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
	intents, err := s.AddIntents("test-intents", "test-client", "Deployment", []otterizev1alpha3.Intent{{
		Type: otterizev1alpha3.IntentTypeHTTP, Name: serviceName, Kind: "Deployment",
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
	policyName := fmt.Sprintf(otterizev1alpha3.OtterizeSingleNetworkPolicyNameTemplate, fmt.Sprintf("%s-deployment", serviceName))
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: policyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
	})

	// make sure the ingress network policy doesn't exist yet
	externalNetworkPolicyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, serviceName)
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

	protectedService = &otterizev1alpha3.ProtectedService{}
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
	policyName := fmt.Sprintf(otterizev1alpha3.OtterizeSingleNetworkPolicyNameTemplate, serviceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: policyName}, np)
		assert.True(errors.IsNotFound(err))
	})

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err = s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	// make sure the ingress network policy doesn't exist yet
	externalNetworkPolicyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, serviceName)
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

	protectedService = &otterizev1alpha3.ProtectedService{}
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

	_, err = s.AddIntents("test-intents", "test-client", "Deployment", []otterizev1alpha3.Intent{{
		Type: otterizev1alpha3.IntentTypeHTTP, Name: serviceName,
	},
	})
	s.Require().NoError(err)

	res, err = s.EffectivePolicyIntentsReconciler.Reconcile(context.Background(), ctrl.Request{})
	s.Require().NoError(err)
	s.Require().Empty(res)

	// make sure the network policy was created between the two services based on the intents
	np := &v1.NetworkPolicy{}
	policyName := fmt.Sprintf(otterizev1alpha3.OtterizeSingleNetworkPolicyNameTemplate, serviceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: policyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
	})

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err = s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	// make sure the ingress network policy doesn't exist yet
	externalNetworkPolicyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, serviceName)
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
	intents, err := s.AddIntents("test-intents", "test-client", "Deployment", []otterizev1alpha3.Intent{{
		Type: otterizev1alpha3.IntentTypeHTTP, Name: serviceName,
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
	policyName := fmt.Sprintf(otterizev1alpha3.OtterizeSingleNetworkPolicyNameTemplate, serviceName)
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
	externalNetworkPolicyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, loadBalancerServiceName)

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
	intents, err := s.AddIntents("test-intents", "test-client", "Deployment", []otterizev1alpha3.Intent{{
		Name: serviceName,
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
	intentNetworkPolicyName := fmt.Sprintf(otterizev1alpha3.OtterizeSingleNetworkPolicyNameTemplate, serviceName)
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
	externalNetworkPolicyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, loadBalancerServiceName)
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
		intentsDeleted := &otterizev1alpha3.ClientIntents{}
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
	intents, err := s.AddIntents("test-intents", "test-client", "Deployment", []otterizev1alpha3.Intent{{
		Name: serviceName,
	},
	})
	s.Require().NoError(err)

	secondaryNamespace := "ns-" + uuid.New().String() + "e"
	s.CreateNamespace(secondaryNamespace)
	secondIntents, err := s.AddIntentsInNamespace("test-intents-other-ns", "test-client-other-ns", "", secondaryNamespace, []otterizev1alpha3.Intent{{
		Name: fmt.Sprintf("%s.%s", serviceName, s.TestNamespace),
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
	intentNetworkPolicyName := fmt.Sprintf(otterizev1alpha3.OtterizeSingleNetworkPolicyNameTemplate, serviceName)
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
	externalNetworkPolicyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, loadBalancerServiceName)
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
		intentsDeleted := &otterizev1alpha3.ClientIntents{}
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
	intents, err := s.AddIntents("test-intents", "test-client", "Deployment", []otterizev1alpha3.Intent{{
		Name: serviceName,
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
	policyName := fmt.Sprintf(otterizev1alpha3.OtterizeSingleNetworkPolicyNameTemplate, serviceName)

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
	externalNetworkPolicyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, nodePortServiceName)

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
	intents, err := s.AddIntents("test-intents", "test-client", "Deployment", []otterizev1alpha3.Intent{{
		Name: serviceName,
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
	policyName := fmt.Sprintf(otterizev1alpha3.OtterizeSingleNetworkPolicyNameTemplate, serviceName)
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
	externalNetworkPolicyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, nodePortServiceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
		assert.True(errors.IsNotFound(err))
	})

	s.AddNodePortService(nodePortServiceName, podIps, podLabels)

	netpolHandler := external_traffic.NewNetworkPolicyHandler(s.Mgr.GetClient(), s.TestEnv.Scheme, allowexternaltraffic.Off)
	endpointReconcilerWithEnforcementDisabled := external_traffic.NewEndpointsReconciler(s.Mgr.GetClient(), netpolHandler)
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
	select {
	case event := <-recorder.Events:
		s.Require().Contains(event, external_traffic.ReasonEnforcementGloballyDisabled)
	default:
		s.Fail("event not raised")
	}
}

func TestExternalNetworkPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(ExternalNetworkPolicyReconcilerTestSuite))
}
