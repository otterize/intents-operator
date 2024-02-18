package builders

import (
	"context"
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"testing"
	"time"
)

type PortEgressNetworkPolicyReconcilerTestSuite struct {
	RulesBuilderTestSuiteBase
	Builder *PortEgressRulesBuilder
}

func (s *PortEgressNetworkPolicyReconcilerTestSuite) SetupTest() {
	s.RulesBuilderTestSuiteBase.SetupTest()
	s.Builder = NewPortEgressRulesBuilder(s.Client)
	s.Reconciler.AddEgressRuleBuilder(s.Builder)
}

func (s *PortEgressNetworkPolicyReconcilerTestSuite) TearDownTest() {
	s.RulesBuilderTestSuiteBase.TearDownTest()
	s.Builder = nil
}

func (s *PortEgressNetworkPolicyReconcilerTestSuite) networkPolicyTemplate(
	policyName string,
	targetNamespace string,
	formattedClient string,
	formattedServer string,
	intentsObjNamespace string,
	svcObject *corev1.Service,
) *v1.NetworkPolicy {
	netpol := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: targetNamespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeNetworkPolicy: formattedClient,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha3.OtterizeServiceLabelKey: formattedClient,
				},
			},
			Ingress: make([]v1.NetworkPolicyIngressRule, 0),
			Egress: []v1.NetworkPolicyEgressRule{
				{
					To: []v1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: svcObject.Spec.Selector,
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
	return netpol
}

func (s *PortEgressNetworkPolicyReconcilerTestSuite) TestErrorWhenKubernetesServiceWithNoPods() {
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
	clientIntents.Name = clientIntentsName
	clientIntents.Namespace = testClientNamespace
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
	s.ExpectEvent(consts.ReasonCreatingEgressNetworkPoliciesFailed)
}

func (s *PortEgressNetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicyKubernetesService() {
	clientIntentsName := "client-intents"
	policyName := "test-client-access"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedClient := "test-client-test-client-namespac-edb3a2"
	formattedServer := "test-server-test-namespace-8ddecb"

	s.testCreateNetworkPolicyForKubernetesService(
		clientIntentsName,
		serverNamespace,
		serviceName,
		policyName,
		formattedClient,
		formattedServer,
	)
	s.ExpectEvent(consts.ReasonCreatedEgressNetworkPolicies)
}

func (s *PortEgressNetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicyForAPIServerServiceWithoutPod() {
	clientIntentsName := "client-intents"
	policyName := "test-client-access"
	serviceName := "test-client"
	serverName := "svc:kubernetes"
	serverNamespace := "default"
	formattedClient := "test-client-test-client-namespac-edb3a2"

	namespacedName := types.NamespacedName{
		Namespace: testClientNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Name: fmt.Sprintf("%s.%s", serverName, serverNamespace),
			},
		},
	}

	clientIntents := otterizev1alpha3.ClientIntents{Spec: intentsSpec}
	clientIntents.Name = clientIntentsName
	clientIntents.Namespace = testClientNamespace
	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntents})

	serverStrippedSVCPrefix := strings.ReplaceAll(serverName, "svc:", "")
	kubernetesSvcNamespacedName := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      serverStrippedSVCPrefix,
	}
	servicePort := int32(443)
	serviceIp := "10.96.1.1"
	svcObject := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverStrippedSVCPrefix,
			Namespace: serverNamespace,
		},

		Spec: corev1.ServiceSpec{
			ClusterIP: serviceIp,
			Selector:  nil,
			Ports: []corev1.ServicePort{{
				TargetPort: intstr.IntOrString{
					IntVal: servicePort,
				},
				Protocol: corev1.ProtocolTCP,
			}},
		},
	}

	s.Client.EXPECT().Get(gomock.Any(), kubernetesSvcNamespacedName, gomock.AssignableToTypeOf(&svcObject)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, service *corev1.Service, options ...client.ListOption) error {
			svcObject.DeepCopyInto(service)
			return nil
		})

	endpointIp := "10.128.0.9"
	endpointPort := int32(8443)
	apiServerEndpoint := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubernetes",
			Namespace: "default",
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{
						IP: endpointIp,
					},
				},
				Ports: []corev1.EndpointPort{
					{
						Port:     endpointPort,
						Protocol: corev1.ProtocolTCP,
					},
				},
			},
		},
	}
	emptyEndpoints := &corev1.Endpoints{}
	s.Client.EXPECT().Get(gomock.Any(), kubernetesSvcNamespacedName, gomock.Eq(emptyEndpoints)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, endpoints *corev1.Endpoints, options ...client.ListOption) error {
			apiServerEndpoint.DeepCopyInto(endpoints)
			return nil
		})

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: testClientNamespace,
		Name:      policyName,
	}
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			return apierrors.NewNotFound(v1.Resource("networkpolicy"), name.Name)
		})

	// Create NetworkPolicy
	newPolicy := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: testClientNamespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeNetworkPolicy: formattedClient,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha3.OtterizeServiceLabelKey: formattedClient,
				},
			},
			Ingress: make([]v1.NetworkPolicyIngressRule, 0),
			Egress: []v1.NetworkPolicyEgressRule{
				{
					To: []v1.NetworkPolicyPeer{{
						IPBlock: &v1.IPBlock{CIDR: endpointIp + "/32"},
					}},
					Ports: []v1.NetworkPolicyPort{{
						Port:     &intstr.IntOrString{IntVal: endpointPort},
						Protocol: lo.ToPtr(corev1.ProtocolTCP),
					}},
				},
			},
		},
	}

	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(newPolicy)).Return(nil)
	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), gomock.Any(), gomock.Any())

	s.ignoreRemoveOrphan()

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedEgressNetworkPolicies)
}

func (s *PortEgressNetworkPolicyReconcilerTestSuite) testCreateNetworkPolicyForKubernetesService(
	clientIntentsName string,
	serverNamespace string,
	serviceName string,
	policyName string,
	formattedClient string,
	formattedServer string,
) {
	namespacedName := types.NamespacedName{
		Namespace: testClientNamespace,
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
	clientIntents.Name = clientIntentsName
	clientIntents.Namespace = testClientNamespace
	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntents})

	svcSelector := map[string]string{"a": "b"}
	svcObject := s.addExpectedKubernetesServiceCall("test-server", testNamespace, 80, svcSelector)
	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: testClientNamespace,
		Name:      policyName,
	}
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			return apierrors.NewNotFound(v1.Resource("networkpolicy"), name.Name)
		})

	// Create NetworkPolicy
	newPolicy := s.networkPolicyTemplate(
		policyName,
		testClientNamespace,
		formattedClient,
		formattedServer,
		testNamespace,
		svcObject,
	)
	// Add target port and change selector in ingress to use svc
	newPolicy.Spec.Egress[0].Ports = []v1.NetworkPolicyPort{{Port: &intstr.IntOrString{IntVal: 80}}}

	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), gomock.Any(), gomock.Any())
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(newPolicy)).Return(nil)

	s.ignoreRemoveOrphan()

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *PortEgressNetworkPolicyReconcilerTestSuite) TestUpdateNetworkPolicyForKubernetesService() {
	clientIntentsName := "client-intents"
	policyName := "test-client-access"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedClient := "test-client-test-client-namespac-edb3a2"
	formattedServer := "test-server-test-namespace-8ddecb"

	namespacedName := types.NamespacedName{
		Namespace: testClientNamespace,
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
	clientIntents.Name = clientIntentsName
	clientIntents.Namespace = testClientNamespace
	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntents})

	svcSelector := map[string]string{"a": "b"}
	svcObject := s.addExpectedKubernetesServiceCall("test-server", testNamespace, 80, svcSelector)
	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: testClientNamespace,
		Name:      policyName,
	}

	newPolicy := s.networkPolicyTemplate(
		policyName,
		testClientNamespace,
		formattedClient,
		formattedServer,
		testNamespace,
		svcObject,
	)
	// Add target port and change selector in egress to use svc
	newPolicy.Spec.Egress[0].Ports = []v1.NetworkPolicyPort{{Port: &intstr.IntOrString{IntVal: 80}}}
	existingBadPolicy := newPolicy.DeepCopy()
	existingBadPolicy.Spec.Egress[0].Ports = []v1.NetworkPolicyPort{{Port: &intstr.IntOrString{IntVal: 90}}}

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
	s.ExpectEvent(consts.ReasonCreatedEgressNetworkPolicies)
}

func (s *PortEgressNetworkPolicyReconcilerTestSuite) TestCleanNetworkPolicyForKubernetesService() {
	clientIntentsName := "client-intents"
	policyName := "test-client-access"
	serviceName := "test-client"
	formattedClient := "test-client-test-client-namespac-edb3a2"
	formattedTargetServer := "test-server-test-namespace-8ddecb"

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

	clientIntentsObj := otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:              clientIntentsName,
			Namespace:         testNamespace,
			DeletionTimestamp: &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)},
		},
		Spec: intentsSpec,
	}
	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntentsObj})

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
		formattedClient,
		formattedTargetServer,
		testNamespace,
		&svcObject,
	)
	// Add target port and change selector in ingress to use svc
	existingPolicy.Spec.Egress[0].Ports = []v1.NetworkPolicyPort{{Port: &intstr.IntOrString{IntVal: 80}}}

	s.externalNetpolHandler.EXPECT().HandleBeforeAccessPolicyRemoval(gomock.Any(), gomock.Eq(existingPolicy))
	s.expectRemoveOrphanFindsPolicies([]v1.NetworkPolicy{*existingPolicy})

	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(existingPolicy)).Return(nil)

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func TestPortEgressNetworkPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(PortEgressNetworkPolicyReconcilerTestSuite))
}
