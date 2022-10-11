package intents_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/external_traffic"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/otterize/intents-operator/src/watcher/reconcilers"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"testing"
)

type ExternalNetworkPolicyReconcilerTestSuite struct {
	testbase.ControllerManagerTestSuiteBase
	IngressReconciler       *external_traffic.IngressReconciler
	endpointReconciler      *external_traffic.EndpointsReconciler
	NetworkPolicyReconciler *NetworkPolicyReconciler
	podWatcher              *reconcilers.PodWatcher
}

func (s *ExternalNetworkPolicyReconcilerTestSuite) SetupSuite() {
	s.TestEnv = &envtest.Environment{}
	var err error
	s.TestEnv.CRDDirectoryPaths = []string{filepath.Join("..", "..", "config", "crd", "bases")}

	s.RestConfig, err = s.TestEnv.Start()
	s.Require().NoError(err)
	s.Require().NotNil(s.RestConfig)

	s.K8sDirectClient, err = kubernetes.NewForConfig(s.RestConfig)
	s.Require().NoError(err)
	s.Require().NotNil(s.K8sDirectClient)

	err = otterizev1alpha1.AddToScheme(s.TestEnv.Scheme)
	s.Require().NoError(err)
}

func (s *ExternalNetworkPolicyReconcilerTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()

	recorder := s.Mgr.GetEventRecorderFor("intents-operator")
	s.NetworkPolicyReconciler = NewNetworkPolicyReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme, nil, []string{}, true)
	s.NetworkPolicyReconciler.InjectRecorder(recorder)

	s.endpointReconciler = external_traffic.NewEndpointsReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme, true)
	s.endpointReconciler.InjectRecorder(recorder)
	err := s.endpointReconciler.InitIngressReferencedServicesIndex(s.Mgr)
	s.Require().NoError(err)

	s.IngressReconciler = external_traffic.NewIngressReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme, s.endpointReconciler)
	s.IngressReconciler.InjectRecorder(recorder)
	err = s.IngressReconciler.InitNetworkPoliciesByIngressNameIndex(s.Mgr)
	s.Require().NoError(err)

	s.podWatcher = reconcilers.NewPodWatcher(s.Mgr.GetClient())
	err = s.podWatcher.InitIntentsClientIndices(s.Mgr)
	s.Require().NoError(err)
}

// BeforeTest happens AFTER the SetupTest()
func (s *ExternalNetworkPolicyReconcilerTestSuite) BeforeTest(_, testName string) {
	s.ControllerManagerTestSuiteBase.BeforeTest("", testName)
}

func (s *ExternalNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateForIngress() {
	serviceName := "test-server-ingress-test"
	intents, err := s.AddIntents("test-intents", "test-client", []otterizev1alpha1.Intent{{
		Type: otterizev1alpha1.IntentTypeHTTP, Name: serviceName,
	},
	})
	s.Require().NoError(err)

	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	res, err := s.NetworkPolicyReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	// make sure the network policy was created between the two services based on the intents
	np := &v1.NetworkPolicy{}
	policyName := fmt.Sprintf(otterizev1alpha1.OtterizeNetworkPolicyNameTemplate, serviceName, s.TestNamespace)
	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: policyName}, np)
	s.Require().NoError(err)
	s.Require().NotEmpty(np)

	s.AddDeploymentWithService(serviceName, []string{"1.1.1.1"}, map[string]string{"app": "test"}, nil)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err = s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	// make sure the ingress network policy doesn't exist yet
	externalNetworkPolicyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, serviceName)
	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
	s.Require().True(errors.IsNotFound(err))

	s.AddIngress(serviceName)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
	res, err = s.IngressReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      serviceName + "-ingress",
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)

	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
	s.Require().NoError(err)
	s.Require().NotEmpty(np)
}

func (s *ExternalNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateForLoadBalancer() {
	serviceName := "test-server-load-balancer-test"
	intents, err := s.AddIntents("test-intents", "test-client", []otterizev1alpha1.Intent{{
		Type: otterizev1alpha1.IntentTypeHTTP, Name: serviceName,
	},
	})
	s.Require().NoError(err)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	res, err := s.NetworkPolicyReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	// make sure the network policy was created between the two services based on the intents
	np := &v1.NetworkPolicy{}
	policyName := fmt.Sprintf(otterizev1alpha1.OtterizeNetworkPolicyNameTemplate, serviceName, s.TestNamespace)
	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: policyName}, np)
	s.Require().NoError(err)
	s.Require().NotEmpty(np)

	podIps := []string{"1.1.2.1"}
	podLabels := map[string]string{"app": "test-load-balancer"}
	s.AddDeploymentWithService(serviceName, podIps, podLabels, nil)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err = s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	// make sure the load balancer network policy doesn't exist yet
	loadBalancerServiceName := serviceName + "-lb"
	externalNetworkPolicyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, loadBalancerServiceName)
	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
	s.Require().True(errors.IsNotFound(err))

	s.AddLoadBalancerService(loadBalancerServiceName, podIps, podLabels)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
	res, err = s.endpointReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      loadBalancerServiceName,
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)

	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
	s.Require().NoError(err)
	s.Require().NotEmpty(np)
}

func (s *ExternalNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateForNodePort() {
	serviceName := "test-server-node-port-test"
	intents, err := s.AddIntents("test-intents", "test-client", []otterizev1alpha1.Intent{{
		Type: otterizev1alpha1.IntentTypeHTTP, Name: serviceName,
	},
	})
	s.Require().NoError(err)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	res, err := s.NetworkPolicyReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      intents.Name,
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	// make sure the network policy was created between the two services based on the intents
	np := &v1.NetworkPolicy{}
	policyName := fmt.Sprintf(otterizev1alpha1.OtterizeNetworkPolicyNameTemplate, serviceName, s.TestNamespace)
	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: policyName}, np)
	s.Require().NoError(err)
	s.Require().NotEmpty(np)

	podIps := []string{"1.1.2.1"}
	podLabels := map[string]string{"app": "test-load-balancer"}
	s.AddDeploymentWithService(serviceName, podIps, podLabels, nil)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err = s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	// make sure the load balancer network policy doesn't exist yet
	nodePortServiceName := serviceName + "-np"
	externalNetworkPolicyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, nodePortServiceName)
	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
	s.Require().True(errors.IsNotFound(err))

	s.AddNodePortService(nodePortServiceName, podIps, podLabels)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
	res, err = s.endpointReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.TestNamespace,
			Name:      nodePortServiceName,
		},
	})

	s.Require().NoError(err)
	s.Require().Empty(res)

	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
	err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
	s.Require().NoError(err)
	s.Require().NotEmpty(np)
}

func TestExternalNetworkPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(ExternalNetworkPolicyReconcilerTestSuite))
}
