package external_traffic_network_policy

import (
	"context"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers"
	"github.com/otterize/intents-operator/src/operator/controllers/external_traffic"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/operator/controllers/pod_reconcilers"
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
	"testing"
)

type ExternalNetworkPolicyReconcilerWithNoIntentsTestSuite struct {
	testbase.ControllerManagerTestSuiteBase
	IngressReconciler       *external_traffic.IngressReconciler
	endpointReconciler      external_traffic.EndpointsReconciler
	NetworkPolicyReconciler *intents_reconcilers.NetworkPolicyReconciler
	podWatcher              *pod_reconcilers.PodWatcher
}

func (s *ExternalNetworkPolicyReconcilerWithNoIntentsTestSuite) SetupSuite() {
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
	utilruntime.Must(otterizev1alpha2.AddToScheme(s.TestEnv.Scheme))
}

func (s *ExternalNetworkPolicyReconcilerWithNoIntentsTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()

	recorder := s.Mgr.GetEventRecorderFor("intents-operator")
	createEvenIfNoIntentsFound := true
	netpolHandler := external_traffic.NewNetworkPolicyHandler(s.Mgr.GetClient(), s.TestEnv.Scheme, true, createEvenIfNoIntentsFound)
	s.NetworkPolicyReconciler = intents_reconcilers.NewNetworkPolicyReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme, netpolHandler, []string{}, true, true, createEvenIfNoIntentsFound)
	s.Require().NoError((&controllers.IntentsReconciler{}).InitIntentsServerIndices(s.Mgr))
	s.NetworkPolicyReconciler.InjectRecorder(recorder)

	s.endpointReconciler = external_traffic.NewEndpointsReconciler(s.Mgr.GetClient(), netpolHandler)
	s.endpointReconciler.InjectRecorder(recorder)
	err := s.endpointReconciler.InitIngressReferencedServicesIndex(s.Mgr)
	s.Require().NoError(err)

	s.IngressReconciler = external_traffic.NewIngressReconciler(s.Mgr.GetClient(), netpolHandler)
	s.IngressReconciler.InjectRecorder(recorder)
	err = s.IngressReconciler.InitNetworkPoliciesByIngressNameIndex(s.Mgr)
	s.Require().NoError(err)

	s.podWatcher = pod_reconcilers.NewPodWatcher(s.Mgr.GetClient(), recorder, []string{}, true, true)
	err = s.podWatcher.InitIntentsClientIndices(s.Mgr)
	s.Require().NoError(err)

	err = (&controllers.IntentsReconciler{}).InitEndpointsPodNamesIndex(s.Mgr)
	s.Require().NoError(err)
}

func (s *ExternalNetworkPolicyReconcilerWithNoIntentsTestSuite) TestNetworkPolicyCreateForIngress() {
	serviceName := "test-server-ingress-test"

	// make sure the network policy was created between the two services based on the intents
	np := &v1.NetworkPolicy{}

	s.AddDeploymentWithService(serviceName, []string{"1.1.1.1"}, map[string]string{"app": "test"}, nil)

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err := s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	// make sure the ingress network policy doesn't exist yet
	externalNetworkPolicyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, serviceName)
	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
	s.Require().True(errors.IsNotFound(err))

	s.AddIngress(serviceName)

	res, err := s.IngressReconciler.Reconcile(context.Background(), ctrl.Request{
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

func (s *ExternalNetworkPolicyReconcilerWithNoIntentsTestSuite) TestNetworkPolicyCreateForLoadBalancer() {
	serviceName := "test-server-load-balancer-test"

	// make sure the network policy was created between the two services based on the intents
	np := &v1.NetworkPolicy{}

	podIps := []string{"1.1.2.1"}
	podLabels := map[string]string{"app": "test-load-balancer"}
	s.AddDeploymentWithService(serviceName, podIps, podLabels, nil)

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err := s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	// make sure the load balancer network policy doesn't exist yet
	loadBalancerServiceName := serviceName + "-lb"
	externalNetworkPolicyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, loadBalancerServiceName)
	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
	s.Require().True(errors.IsNotFound(err))

	s.AddLoadBalancerService(loadBalancerServiceName, podIps, podLabels)
	res, err := s.endpointReconciler.Reconcile(context.Background(), ctrl.Request{
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

func (s *ExternalNetworkPolicyReconcilerWithNoIntentsTestSuite) TestNetworkPolicyCreateForNodePort() {
	serviceName := "test-server-node-port-test"

	// make sure the network policy was created between the two services based on the intents
	np := &v1.NetworkPolicy{}

	podIps := []string{"1.1.2.1"}
	podLabels := map[string]string{"app": "test-load-balancer"}
	s.AddDeploymentWithService(serviceName, podIps, podLabels, nil)

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err := s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	// make sure the load balancer network policy doesn't exist yet
	nodePortServiceName := serviceName + "-np"
	externalNetworkPolicyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, nodePortServiceName)
	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
	s.Require().True(errors.IsNotFound(err))

	s.AddNodePortService(nodePortServiceName, podIps, podLabels)
	res, err := s.endpointReconciler.Reconcile(context.Background(), ctrl.Request{
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

func (s *ExternalNetworkPolicyReconcilerWithNoIntentsTestSuite) TestEndpointsReconcilerEnforcementDisabled() {
	serviceName := "test-endpoints-reconciler-enforcement-disabled"

	// make sure the network policy was created between the two services based on the intents
	np := &v1.NetworkPolicy{}

	podIps := []string{"1.1.2.1"}
	podLabels := map[string]string{"app": "test-load-balancer"}
	s.AddDeploymentWithService(serviceName, podIps, podLabels, nil)

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err := s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	// make sure the load balancer network policy doesn't exist yet
	nodePortServiceName := serviceName + "-np"
	externalNetworkPolicyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, nodePortServiceName)
	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
	s.Require().True(errors.IsNotFound(err))

	s.AddNodePortService(nodePortServiceName, podIps, podLabels)

	netpolHandler := external_traffic.NewNetworkPolicyHandler(s.Mgr.GetClient(), s.TestEnv.Scheme, false, true)
	endpointReconcilerWithEnforcementDisabled := external_traffic.NewEndpointsReconciler(s.Mgr.GetClient(), netpolHandler)
	recorder := record.NewFakeRecorder(10)
	endpointReconcilerWithEnforcementDisabled.InjectRecorder(recorder)

	res, err := endpointReconcilerWithEnforcementDisabled.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      nodePortServiceName,
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)
	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
	s.Require().True(errors.IsNotFound(err))
	select {
	case event := <-recorder.Events:
		s.Require().Contains(event, external_traffic.ReasonEnforcementGloballyDisabled)
	default:
		s.Fail("event not raised")
	}
}

func (s *ExternalNetworkPolicyReconcilerWithNoIntentsTestSuite) TestNetworkPolicyCreateForLoadBalancerCreatedDespiteLastIntentDeleted() {
	serviceName := "test-server-load-balancer-test"
	intents, err := s.AddIntents("test-intents", "test-client", []otterizev1alpha3.Intent{{
		Name: serviceName,
	},
	})
	s.Require().NoError(err)

	res, err := s.NetworkPolicyReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	// make sure the network policy was created between the two services based on the intents
	netpol := &v1.NetworkPolicy{}
	intentNetworkPolicyName := fmt.Sprintf(otterizev1alpha2.OtterizeNetworkPolicyNameTemplate, serviceName, s.TestNamespace)
	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: intentNetworkPolicyName}, netpol)
	s.Require().NoError(err)
	s.Require().NotEmpty(netpol)

	podIps := []string{"1.1.2.1"}
	podLabels := map[string]string{"app": "test-load-balancer"}
	s.AddDeploymentWithService(serviceName, podIps, podLabels, nil)

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err = s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	// make sure the load balancer network policy doesn't exist yet
	loadBalancerServiceName := serviceName + "-lb"
	externalNetworkPolicyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, loadBalancerServiceName)
	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, netpol)
	s.Require().True(errors.IsNotFound(err))

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

	res, err = s.NetworkPolicyReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: intentNetworkPolicyName}, &v1.NetworkPolicy{})
		assert.True(errors.IsNotFound(err))
	})

	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, &v1.NetworkPolicy{})
	s.Require().NoError(err)
}

func TestExternalNetworkPolicyReconcilerWithNoIntentsTestSuite(t *testing.T) {
	suite.Run(t, new(ExternalNetworkPolicyReconcilerWithNoIntentsTestSuite))
}
