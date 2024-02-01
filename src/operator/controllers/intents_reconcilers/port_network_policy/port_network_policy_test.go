package port_network_policy

import (
	"context"
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"
	"testing"
	"time"
)

const (
	protectedServicesResourceName        = "staging-protected-services"
	protectedService                     = "test-service"
	protectedServiceFormattedName        = "test-service-test-namespace-b0207e"
	anotherProtectedServiceResourceName  = "protect-other-services"
	anotherProtectedService              = "other-test-service"
	anotherProtectedServiceFormattedName = "other-test-service-test-namespace-398a04"
	testNamespace                        = "test-namespace"
)

type NetworkPolicyReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	Reconciler            *PortNetworkPolicyReconciler
	externalNetpolHandler *mocks.MockexternalNetpolHandler
	scheme                *runtime.Scheme
	EPIntentsReconciler   *intents_reconcilers.ServiceEffectivePolicyIntentsReconciler
}

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

func (s *NetworkPolicyReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.externalNetpolHandler = mocks.NewMockexternalNetpolHandler(s.Controller)
	restrictToNamespaces := make([]string, 0)
	s.scheme = runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s.scheme))

	s.Reconciler = NewPortNetworkPolicyReconciler(
		s.Client,
		s.scheme,
		s.externalNetpolHandler,
		restrictToNamespaces,
		true,
		true,
	)

	epReconciler := effectivepolicy.NewGroupReconciler(s.Client,
		s.scheme, s.Reconciler)
	s.EPIntentsReconciler = intents_reconcilers.NewServiceEffectiveIntentsReconciler(s.Client,
		s.scheme, epReconciler)

	s.Reconciler.Recorder = s.Recorder
	epReconciler.InjectableRecorder.Recorder = s.Recorder
	s.EPIntentsReconciler.Recorder = s.Recorder
}

func (s *NetworkPolicyReconcilerTestSuite) TearDownTest() {
	viper.Reset()
	s.Reconciler = nil
	s.externalNetpolHandler = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *NetworkPolicyReconcilerTestSuite) expectGetAllEffectivePolicies(clientIntents []otterizev1alpha3.ClientIntents) {
	var intentsList otterizev1alpha3.ClientIntentsList

	s.Client.EXPECT().List(gomock.Any(), &intentsList).DoAndReturn(func(_ context.Context, intents *otterizev1alpha3.ClientIntentsList, _ ...any) error {
		intents.Items = append(intents.Items, clientIntents...)
		return nil
	})

	// create service to ClientIntents pointing to it
	services := make(map[string][]otterizev1alpha3.ClientIntents)
	for _, clientIntent := range clientIntents {
		for _, intentCall := range clientIntent.GetCallsList() {
			serverName := intentCall.GetTargetServerName()
			if intentCall.IsTargetServerKubernetesService() {
				serverName = "svc." + serverName
			}
			server := otterizev1alpha3.GetFormattedOtterizeIdentity(serverName, intentCall.GetTargetServerNamespace(clientIntent.Namespace))
			services[server] = append(services[server], clientIntent)
		}
	}

	matchFieldsPtr := &client.MatchingFields{}
	s.Client.EXPECT().List(
		gomock.Any(),
		&otterizev1alpha3.ClientIntentsList{},
		gomock.AssignableToTypeOf(matchFieldsPtr),
	).DoAndReturn(func(_ context.Context, intents *otterizev1alpha3.ClientIntentsList, args ...any) error {
		matchFields := args[0].(*client.MatchingFields)
		intents.Items = services[(*matchFields)[otterizev1alpha3.OtterizeFormattedTargetServerIndexField]]
		return nil
	}).AnyTimes()

}

func (s *NetworkPolicyReconcilerTestSuite) expectRemoveOrphanFindsPolicies(netpols []v1.NetworkPolicy) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha3.OtterizeSvcNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
	}})
	s.Require().NoError(err)

	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), &client.ListOptions{LabelSelector: selector},
	).DoAndReturn(
		func(_ context.Context, netpolList *v1.NetworkPolicyList, _ ...any) error {
			netpolList.Items = append(netpolList.Items, netpols...)
			return nil
		},
	)
}

func (s *NetworkPolicyReconcilerTestSuite) ignoreRemoveOrphan() {
	s.expectRemoveOrphanFindsPolicies(nil)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNetworkPolicyFinalizerAdded() {
	clientIntentsName := "client-intents"
	policyName := "svc-access-to-test-server"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedTargetServer := "svc.test-server-test-namespace-ab42d5"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("svc:test-server.%s", serverNamespace)
	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Name: serverName,
			},
		},
	}

	clientIntents := otterizev1alpha3.ClientIntents{Spec: intentsSpec}
	clientIntents.Namespace = testNamespace
	clientIntents.Name = clientIntentsName
	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntents})

	svcObject := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-server",
			Namespace: testNamespace,
		},

		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"test": "selector"},
			Ports: []corev1.ServicePort{{
				TargetPort: intstr.IntOrString{
					IntVal: int32(8080),
				},
			}},
		},
	}

	// Just assume policy exist because the rest of the flow is tested in other tests
	existingPolicy := s.networkPolicyTemplate(
		policyName,
		serverNamespace,
		formattedTargetServer,
		testNamespace,
		&svcObject,
	)

	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{
		Namespace: testNamespace,
		Name:      "test-server"},
		gomock.AssignableToTypeOf(&corev1.Service{})).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, svc *corev1.Service, options ...client.ListOption) error {
			svcObject.DeepCopyInto(svc)
			return nil
		})

	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      policyName,
	}
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			existingPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	s.ignoreRemoveOrphan()
	s.Client.EXPECT().Patch(gomock.Any(), gomock.AssignableToTypeOf(emptyNetworkPolicy), gomock.Any()).Return(nil)

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) networkPolicyTemplate(
	policyName string,
	targetNamespace string,
	formattedTargetServer string,
	intentsObjNamespace string,
	svcObject *corev1.Service,
) *v1.NetworkPolicy {
	netpol := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: targetNamespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeSvcNetworkPolicy: formattedTargetServer,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: svcObject.Spec.Selector,
			},
			Ingress: []v1.NetworkPolicyIngressRule{
				{
					From: []v1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									fmt.Sprintf(
										otterizev1alpha3.OtterizeSvcAccessLabelKey, formattedTargetServer): "true",
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									otterizev1alpha3.KubernetesStandardNamespaceNameLabelKey: intentsObjNamespace,
								},
							},
						},
					},
				},
			},
		},
	}
	err := controllerutil.SetOwnerReference(svcObject, netpol, s.scheme)
	if err != nil {
		panic(err)
	}
	return netpol
}

func (s *NetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicyKubernetesService() {
	clientIntentsName := "client-intents"
	policyName := "svc-access-to-test-server"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedTargetServer := "svc.test-server-test-namespace-ab42d5"

	s.testCreateNetworkPolicyForKubernetesService(
		clientIntentsName,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
	)
	s.ExpectEvent(consts.ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) TestErrorWhenKubernetesServiceWithNoPods() {
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	serverNamespace := testNamespace

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := "svc:test-server"
	serverCall := fmt.Sprintf("%s.%s", serverName, serverNamespace)
	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Name: serverCall,
			},
		},
	}

	clientIntents := otterizev1alpha3.ClientIntents{Spec: intentsSpec}
	clientIntents.Namespace = testNamespace
	clientIntents.Name = clientIntentsName
	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntents})

	serverStrippedSVCPrefix := strings.ReplaceAll(serverName, "svc:", "")
	kubernetesSvcNamespacedName := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      serverStrippedSVCPrefix,
	}
	svcObject := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverStrippedSVCPrefix,
			Namespace: serverNamespace,
		},

		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				TargetPort: intstr.IntOrString{
					IntVal: int32(443),
				},
			}},
		},
	}

	s.Client.EXPECT().Get(gomock.Any(), kubernetesSvcNamespacedName, gomock.AssignableToTypeOf(&svcObject)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, service *corev1.Service, options ...client.ListOption) error {
			svcObject.DeepCopyInto(service)
			return nil
		})

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.Error(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatingNetworkPoliciesFailed)
}

func (s *NetworkPolicyReconcilerTestSuite) TestIgnoreKubernetesAPIServerService() {
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	serverNamespace := "default"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := "svc:kubernetes"
	serverCall := fmt.Sprintf("%s.%s", serverName, serverNamespace)
	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Name: serverCall,
			},
		},
	}

	clientIntents := otterizev1alpha3.ClientIntents{Spec: intentsSpec}
	clientIntents.Namespace = testNamespace
	clientIntents.Name = clientIntentsName
	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntents})

	s.ignoreRemoveOrphan()

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *NetworkPolicyReconcilerTestSuite) addExpectedKubernetesServiceCall(serverName string, port int, selector map[string]string) *corev1.Service {
	serverStrippedSVCPrefix := strings.ReplaceAll(serverName, "svc:", "")
	kubernetesSvcNamespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      serverStrippedSVCPrefix,
	}
	svcObject := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverStrippedSVCPrefix,
			Namespace: testNamespace,
		},

		Spec: corev1.ServiceSpec{
			Selector: selector,
			Ports: []corev1.ServicePort{{
				TargetPort: intstr.IntOrString{
					IntVal: int32(port),
				},
			}},
		},
	}

	s.Client.EXPECT().Get(gomock.Any(), kubernetesSvcNamespacedName, gomock.AssignableToTypeOf(&svcObject)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, service *corev1.Service, options ...client.ListOption) error {
			svcObject.DeepCopyInto(service)
			return nil
		})

	return &svcObject
}

func (s *NetworkPolicyReconcilerTestSuite) testCreateNetworkPolicyForKubernetesService(
	clientIntentsName string,
	serverNamespace string,
	serviceName string,
	policyName string,
	formattedTargetServer string,
) {
	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Name: fmt.Sprintf("svc:test-server.%s", serverNamespace),
			},
		},
	}

	clientIntents := otterizev1alpha3.ClientIntents{Spec: intentsSpec}
	clientIntents.Namespace = testNamespace
	clientIntents.Name = clientIntentsName
	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntents})

	svcSelector := map[string]string{"a": "b"}
	svcObject := s.addExpectedKubernetesServiceCall("test-server", 80, svcSelector)
	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      policyName,
	}
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			return apierrors.NewNotFound(v1.Resource("networkpolicy"), name.Name)
		})

	// Create NetworkPolicy
	newPolicy := s.networkPolicyTemplate(
		policyName,
		serverNamespace,
		formattedTargetServer,
		testNamespace,
		svcObject,
	)
	// Add target port and change selector in ingress to use svc
	newPolicy.Spec.Ingress[0].Ports = []v1.NetworkPolicyPort{{Port: &intstr.IntOrString{IntVal: 80}}}
	selectorTyped := labels.SelectorFromSet(svcSelector)
	newPolicy.Spec.PodSelector.MatchLabels = svcSelector

	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(newPolicy)).Return(nil)

	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), serverNamespace, selectorTyped)
	s.ignoreRemoveOrphan()

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *NetworkPolicyReconcilerTestSuite) TestUpdateNetworkPolicyForKubernetesService() {
	clientIntentsName := "client-intents"
	policyName := "svc-access-to-test-server"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedTargetServer := "svc.test-server-test-namespace-ab42d5"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("svc:test-server.%s", serverNamespace)
	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Name: serverName,
			},
		},
	}

	clientIntents := otterizev1alpha3.ClientIntents{Spec: intentsSpec}
	clientIntents.Namespace = testNamespace
	clientIntents.Name = clientIntentsName
	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntents})

	svcSelector := map[string]string{"a": "b"}
	svcObject := s.addExpectedKubernetesServiceCall("test-server", 80, svcSelector)
	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      policyName,
	}

	newPolicy := s.networkPolicyTemplate(
		policyName,
		serverNamespace,
		formattedTargetServer,
		testNamespace,
		svcObject,
	)
	// Add target port and change selector in ingress to use svc
	newPolicy.Spec.Ingress[0].Ports = []v1.NetworkPolicyPort{{Port: &intstr.IntOrString{IntVal: 80}}}
	existingBadPolicy := newPolicy.DeepCopy()
	existingBadPolicy.Spec.Ingress[0].Ports = []v1.NetworkPolicyPort{{Port: &intstr.IntOrString{IntVal: 90}}}

	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			existingBadPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	// Update NetworkPolicy
	s.Client.EXPECT().Patch(gomock.Any(), gomock.Eq(newPolicy), intents_reconcilers.MatchPatch(client.MergeFrom(existingBadPolicy))).Return(nil)
	s.ignoreRemoveOrphan()

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) TestCleanNetworkPolicyForKubernetesService() {
	clientIntentsName := "client-intents"
	policyName := "svc-access-to-test-server"
	serviceName := "test-client"
	formattedTargetServer := "test-server-other-namespace-f6a461"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("svc:test-server.%s", testNamespace)
	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Name: serverName,
			},
		},
	}

	// Initial call to get the ClientIntents object when reconciler starts
	clientIntentsObj := otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:              clientIntentsName,
			Namespace:         testNamespace,
			DeletionTimestamp: &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)},
		},
		Spec: intentsSpec,
	}
	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntentsObj})

	// Remove network policy:
	// 1. ep list is empty
	// 2. get into "removeNetworkPoliciesThatShouldNotExist" with empty list of "policiesThatShouldExist"
	// 3. list all policies and remove them

	selector := map[string]string{"test": "selector"}
	svcObject := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-server",
			Namespace: testNamespace,
		},

		Spec: corev1.ServiceSpec{
			Selector: selector,
			Ports: []corev1.ServicePort{{
				TargetPort: intstr.IntOrString{
					IntVal: int32(8080),
				},
			}},
		},
	}

	existingPolicy := s.networkPolicyTemplate(
		policyName,
		testNamespace,
		formattedTargetServer,
		testNamespace,
		&svcObject,
	)
	// Add target port and change selector in ingress to use svc
	existingPolicy.Spec.Ingress[0].Ports = []v1.NetworkPolicyPort{{Port: &intstr.IntOrString{IntVal: 80}}}
	existingPolicy.Spec.PodSelector.MatchLabels = selector

	s.expectRemoveOrphanFindsPolicies([]v1.NetworkPolicy{*existingPolicy})
	s.externalNetpolHandler.EXPECT().HandleBeforeAccessPolicyRemoval(gomock.Any(), existingPolicy)
	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(existingPolicy)).Return(nil)

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func TestNetworkPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(NetworkPolicyReconcilerTestSuite))
}
