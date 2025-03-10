package external_traffic_network_policy

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
	"github.com/otterize/intents-operator/src/operator/controllers/external_traffic"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/networkpolicy"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/networkpolicy/builders"
	"github.com/otterize/intents-operator/src/operator/controllers/pod_reconcilers"
	podreconcilersmocks "github.com/otterize/intents-operator/src/operator/controllers/pod_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/operator/controllers/protected_service_reconcilers"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/operator/mirrorevents"
	"github.com/otterize/intents-operator/src/operator/webhooks"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/allowexternaltraffic"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	istiosecurityscheme "istio.io/client-go/pkg/apis/security/v1beta1"
	v1 "k8s.io/api/networking/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

const ingressControllerName = "ingress-controller"
const ingressControllerNamespace = "ingress-nginx"
const ingressControllerKind = "Deployment"
const ingressControllerHashedName = "ingress-controller-ingress-nginx-53b476"

type ExternalNetworkPolicyReconcilerWithIngressControllersConfiguredTestSuite struct {
	testbase.ControllerManagerTestSuiteBase
	IngressReconciler                *external_traffic.IngressReconciler
	endpointReconciler               external_traffic.EndpointsReconciler
	EffectivePolicyIntentsReconciler *intents_reconcilers.ServiceEffectivePolicyIntentsReconciler
	podWatcher                       *pod_reconcilers.PodWatcher
	defaultDenyReconciler            *protected_service_reconcilers.DefaultDenyReconciler
	netpolHandler                    *external_traffic.NetworkPolicyHandler
}

func (s *ExternalNetworkPolicyReconcilerWithIngressControllersConfiguredTestSuite) SetupSuite() {
	logrus.Info("Setting up test suite")
	s.TestEnv = &envtest.Environment{Scheme: clientgoscheme.Scheme}
	var err error
	s.TestEnv.CRDDirectoryPaths = []string{filepath.Join("..", "..", "..", "config", "crd")}
	s.TestEnv.WebhookInstallOptions = envtest.WebhookInstallOptions{
		Paths:            []string{filepath.Join("..", "..", "..", "config", "webhook")},
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

func (s *ExternalNetworkPolicyReconcilerWithIngressControllersConfiguredTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()
	intentsValidator := webhooks.NewIntentsValidatorV1alpha2(s.Mgr.GetClient())
	s.Require().NoError((&otterizev1alpha2.ClientIntents{}).SetupWebhookWithManager(s.Mgr, intentsValidator))
	intentsValidator13 := webhooks.NewIntentsValidatorV1alpha3(s.Mgr.GetClient())
	s.Require().NoError((&otterizev1alpha3.ClientIntents{}).SetupWebhookWithManager(s.Mgr, intentsValidator13))
	s.Require().NoError((&otterizev2alpha1.ApprovedClientIntents{}).SetupWebhookWithManager(s.Mgr))
	intentsValidator2beta1 := webhooks.NewIntentsValidatorV2beta1(s.Mgr.GetClient())
	s.Require().NoError((&otterizev2beta1.ClientIntents{}).SetupWebhookWithManager(s.Mgr, intentsValidator2beta1))

	recorder := mirrorevents.GetMirrorToClientIntentsEventRecorderFor(s.Mgr, "intents-operator")
	testName := s.T().Name()
	isShadowMode := strings.Contains(testName, "ShadowMode")
	defaultActive := !isShadowMode
	netpolHandler := external_traffic.NewNetworkPolicyHandler(s.Mgr.GetClient(), s.TestEnv.Scheme, allowexternaltraffic.IfBlockedByOtterize, []serviceidentity.ServiceIdentity{
		{
			Kind:      "Deployment",
			Namespace: ingressControllerNamespace,
			Name:      ingressControllerName,
		},
	}, false)
	s.defaultDenyReconciler = protected_service_reconcilers.NewDefaultDenyReconciler(s.Mgr.GetClient(), true)
	netpolReconciler := networkpolicy.NewReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme, netpolHandler, []string{}, goset.NewSet[string](), true, defaultActive, false, []networkpolicy.IngressRuleBuilder{builders.NewIngressNetpolBuilder(), builders.NewPortNetworkPolicyReconciler(s.Mgr.GetClient())}, nil)
	serviceIdResolver := serviceidresolver.NewResolver(s.Mgr.GetClient())
	epReconciler := effectivepolicy.NewGroupReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme, serviceIdResolver, netpolReconciler)
	s.EffectivePolicyIntentsReconciler = intents_reconcilers.NewServiceEffectiveIntentsReconciler(s.Mgr.GetClient(), s.TestEnv.Scheme, epReconciler)
	s.Require().NoError((&controllers.ApprovedIntentsReconciler{}).InitIntentsServerIndices(s.Mgr))
	s.EffectivePolicyIntentsReconciler.InjectRecorder(recorder)

	s.endpointReconciler = external_traffic.NewEndpointsReconciler(s.Mgr.GetClient(), netpolHandler)
	s.endpointReconciler.InjectRecorder(recorder)
	err := s.endpointReconciler.InitIngressReferencedServicesIndex(s.Mgr)
	s.Require().NoError(err)

	s.IngressReconciler = external_traffic.NewIngressReconciler(s.Mgr.GetClient(), netpolHandler)
	s.IngressReconciler.InjectRecorder(recorder)
	s.Require().NoError(err)

	s.netpolHandler = netpolHandler

	controller := gomock.NewController(s.T())
	serviceEffectivePolicyReconciler := podreconcilersmocks.NewMockGroupReconciler(controller)
	s.podWatcher = pod_reconcilers.NewPodWatcher(s.Mgr.GetClient(), recorder, []string{}, true, true, goset.NewSet[string](), &mocks.MockIntentsReconcilerForTestEnv{}, serviceEffectivePolicyReconciler)
	err = s.podWatcher.InitIntentsClientIndices(s.Mgr)
	s.Require().NoError(err)

	err = (&controllers.ApprovedIntentsReconciler{}).InitEndpointsPodNamesIndex(s.Mgr)
	s.Require().NoError(err)
}

func (s *ExternalNetworkPolicyReconcilerWithIngressControllersConfiguredTestSuite) TestNetworkPolicyCreateForIngress() {
	serviceName := "test-server-ingress-test"
	intents, err := s.AddIntents("test-intents", "test-client", "Deployment", []otterizev2alpha1.Target{{
		Kubernetes: lo.ToPtr(otterizev2alpha1.KubernetesTarget{Name: serviceName}),
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

	s.Require().Len(np.Spec.Ingress, 1)
	s.Require().Len(np.Spec.Ingress[0].From, 1)
	s.Require().Equal([]v1.NetworkPolicyPeer{
		{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
			"intents.otterize.com/owner-kind": ingressControllerKind,
			"intents.otterize.com/service":    ingressControllerHashedName,
		}},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
				"kubernetes.io/metadata.name": ingressControllerNamespace,
			},
			},
		}}, np.Spec.Ingress[0].From)
}

func (s *ExternalNetworkPolicyReconcilerWithIngressControllersConfiguredTestSuite) TestNetworkPolicyCreateForIngressWithIntentToSVC() {
	serviceName := "test-server-ingress-test"
	intents, err := s.AddIntents("test-intents", "test-client", "Deployment", []otterizev2alpha1.Target{{
		Kubernetes: lo.ToPtr(otterizev2alpha1.KubernetesTarget{Name: serviceName, Kind: serviceidentity.KindService}),
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
	policyName := fmt.Sprintf(otterizev2alpha1.OtterizeSingleNetworkPolicyNameTemplate, fmt.Sprintf("%s-service", serviceName))
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

	s.Require().Len(np.Spec.Ingress, 1)
	s.Require().Len(np.Spec.Ingress[0].From, 1)
	s.Require().Equal([]v1.NetworkPolicyPeer{
		{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
			"intents.otterize.com/owner-kind": ingressControllerKind,
			"intents.otterize.com/service":    ingressControllerHashedName,
		}},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
				"kubernetes.io/metadata.name": ingressControllerNamespace,
			},
			},
		}}, np.Spec.Ingress[0].From)
}

func (s *ExternalNetworkPolicyReconcilerWithIngressControllersConfiguredTestSuite) TestNetworkPolicyCreateForIngressWithIntentToDeployment() {
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

	s.Require().Len(np.Spec.Ingress, 1)
	s.Require().Len(np.Spec.Ingress[0].From, 1)
	s.Require().Equal([]v1.NetworkPolicyPeer{
		{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
			"intents.otterize.com/owner-kind": ingressControllerKind,
			"intents.otterize.com/service":    ingressControllerHashedName,
		}},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
				"kubernetes.io/metadata.name": ingressControllerNamespace,
			},
			},
		}}, np.Spec.Ingress[0].From)
}

func (s *ExternalNetworkPolicyReconcilerWithIngressControllersConfiguredTestSuite) TestIngressProtectedService_ShadowMode() {
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

func (s *ExternalNetworkPolicyReconcilerWithIngressControllersConfiguredTestSuite) TestIngressWithIntentsProtectedService_ShadowMode() {
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

	s.Require().Len(np.Spec.Ingress, 1)
	s.Require().Len(np.Spec.Ingress[0].From, 1)
	s.Require().Equal([]v1.NetworkPolicyPeer{
		{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
			"intents.otterize.com/owner-kind": ingressControllerKind,
			"intents.otterize.com/service":    ingressControllerHashedName,
		}},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
				"kubernetes.io/metadata.name": ingressControllerNamespace,
			},
			},
		}}, np.Spec.Ingress[0].From)

}

func (s *ExternalNetworkPolicyReconcilerWithIngressControllersConfiguredTestSuite) TestNetworkPolicyCreateForLoadBalancer() {
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

	s.Require().Len(np.Spec.Ingress, 1)
	s.Require().Len(np.Spec.Ingress[0].From, 0)
}

func (s *ExternalNetworkPolicyReconcilerWithIngressControllersConfiguredTestSuite) TestNetworkPolicyCreateForLoadBalancerCreatedAndDeletedWhenLastIntentDeleted() {
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
		intentsDeleted := &otterizev2alpha1.ApprovedClientIntents{}
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

func (s *ExternalNetworkPolicyReconcilerWithIngressControllersConfiguredTestSuite) TestNetworkPolicyCreateForLoadBalancerCreatedAndDoesNotGetDeletedEvenWhenIntentRemovedAsLongAsOneRemains() {
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
		intentsDeleted := &otterizev2alpha1.ApprovedClientIntents{}
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

	s.Require().Len(externalNetpol.Spec.Ingress, 1)
	s.Require().Len(externalNetpol.Spec.Ingress[0].From, 0)
}

func (s *ExternalNetworkPolicyReconcilerWithIngressControllersConfiguredTestSuite) TestNetworkPolicyCreateForNodePort() {
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

	s.Require().Len(np.Spec.Ingress, 1)
	s.Require().Len(np.Spec.Ingress[0].From, 0)
}

func (s *ExternalNetworkPolicyReconcilerWithIngressControllersConfiguredTestSuite) TestEndpointsReconcilerNetworkPoliciesDisabled() {
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
	externalNetworkPolicyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, nodePortServiceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: s.TestNamespace, Name: externalNetworkPolicyName}, np)
		assert.True(errors.IsNotFound(err))
	})

	s.AddNodePortService(nodePortServiceName, podIps, podLabels)

	netpolHandler := external_traffic.NewNetworkPolicyHandler(s.Mgr.GetClient(), s.TestEnv.Scheme, allowexternaltraffic.Off, []serviceidentity.ServiceIdentity{
		{
			Namespace: s.TestNamespace,
			Name:      ingressControllerName,
			Kind:      "Deployment",
		},
	}, false)
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

func (s *ExternalNetworkPolicyReconcilerWithIngressControllersConfiguredTestSuite) TestNetworkPolicyForAWSALBExemption_enabled() {
	serviceName := "ingress-service"
	ingressName := "test-ingress-alb"
	ingressNamespace := s.TestNamespace
	s.netpolHandler.SetIngressControllerALBAllowAll(true)

	// Add Ingress with the annotation "alb.ingress.kubernetes.io/scheme": "internet-facing"
	ingress := s.AddIngressWithAnnotation(ingressName, ingressNamespace, serviceName, map[string]string{
		"alb.ingress.kubernetes.io/target-type": "ip",
	})

	intents, err := s.AddIntents("test-intents", "test-client", "Deployment", []otterizev2alpha1.Target{{
		Service: &otterizev2alpha1.ServiceTarget{Name: ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name},
	},
	})
	s.Require().NoError(err)

	_, err = s.EffectivePolicyIntentsReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: intents.Namespace,
			Name:      intents.Name,
		},
	})

	s.Require().NoError(err)

	// Reconcile the ingress
	res, err := s.IngressReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: ingressNamespace, Name: ingressName},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	// Verify that the network policy allows all ingress traffic
	np := &v1.NetworkPolicy{}
	policyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, serviceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err := s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: ingressNamespace, Name: policyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
		assert.Len(np.Spec.Ingress, 1)
		if len(np.Spec.Ingress) == 1 {
			assert.Len(np.Spec.Ingress[0].From, 0) // Allow all ingress traffic
		}
	})
}

func (s *ExternalNetworkPolicyReconcilerWithIngressControllersConfiguredTestSuite) TestNetworkPolicyForAWSALBExemption_disabled() {
	serviceName := "ingress-service"
	ingressName := "test-ingress-alb"
	ingressNamespace := s.TestNamespace
	s.netpolHandler.SetIngressControllerALBAllowAll(false)

	// Add Ingress with the annotation "alb.ingress.kubernetes.io/scheme": "internet-facing"
	ingress := s.AddIngressWithAnnotation(ingressName, ingressNamespace, serviceName, map[string]string{
		"alb.ingress.kubernetes.io/scheme": "internet-facing",
	})

	intents, err := s.AddIntents("test-intents", "test-client", "Deployment", []otterizev2alpha1.Target{{
		Service: &otterizev2alpha1.ServiceTarget{Name: ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name},
	},
	})
	s.Require().NoError(err)

	_, err = s.EffectivePolicyIntentsReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: intents.Namespace,
			Name:      intents.Name,
		},
	})

	s.Require().NoError(err)

	// Reconcile the ingress
	res, err := s.IngressReconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: ingressNamespace, Name: ingressName},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)

	// Verify that the network policy allows all ingress traffic
	np := &v1.NetworkPolicy{}
	policyName := fmt.Sprintf(external_traffic.OtterizeExternalNetworkPolicyNameTemplate, serviceName)
	s.WaitUntilCondition(func(assert *assert.Assertions) {
		err := s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Namespace: ingressNamespace, Name: policyName}, np)
		assert.NoError(err)
		assert.NotEmpty(np)
		assert.Len(np.Spec.Ingress, 1)
		if len(np.Spec.Ingress) == 1 {
			assert.Len(np.Spec.Ingress[0].From, 1) // Only allow traffic from the ingress controller
		}
	})
}

func (s *ExternalNetworkPolicyReconcilerWithIngressControllersConfiguredTestSuite) AddIngressWithAnnotation(name, namespace, serviceName string, annotations map[string]string) *v1.Ingress {
	ingress := &v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Spec: v1.IngressSpec{
			Rules: []v1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: v1.IngressRuleValue{
						HTTP: &v1.HTTPIngressRuleValue{
							Paths: []v1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: lo.ToPtr(v1.PathTypePrefix),
									Backend: v1.IngressBackend{
										Service: &v1.IngressServiceBackend{
											Name: serviceName,
											Port: v1.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	s.Require().NoError(s.Mgr.GetClient().Create(context.Background(), ingress))
	s.WaitForObjectToBeCreated(ingress)

	s.AddDeploymentWithService(serviceName, []string{"3.3.3.3"}, map[string]string{"app": "test"}, nil)

	// the ingress reconciler expect the pod watcher labels in order to work
	_, err := s.podWatcher.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: s.TestNamespace, Name: serviceName + "-0"}})
	s.Require().NoError(err)

	return ingress
}

func TestExternalNetworkPolicyReconcilerWithIngressControllersConfiguredTestSuite(t *testing.T) {
	suite.Run(t, new(ExternalNetworkPolicyReconcilerWithIngressControllersConfiguredTestSuite))
}
