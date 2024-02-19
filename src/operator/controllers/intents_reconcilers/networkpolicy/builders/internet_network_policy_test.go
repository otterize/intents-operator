package builders

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"
)

type InternetNetworkPolicyReconcilerTestSuite struct {
	RulesBuilderTestSuiteBase
	Builder *InternetEgressRulesBuilder
}

func (s *InternetNetworkPolicyReconcilerTestSuite) SetupTest() {
	s.RulesBuilderTestSuiteBase.SetupTest()
	s.Builder = NewInternetEgressRulesBuilder()
	s.Builder.Recorder = s.Recorder
	s.Reconciler.AddEgressRuleBuilder(s.Builder)

}

func (s *InternetNetworkPolicyReconcilerTestSuite) TearDownTest() {
	s.RulesBuilderTestSuiteBase.TearDownTest()
	s.Builder = nil
}

func (s *InternetNetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicySingleEndpoint() {
	s.Reconciler.EnforcementDefaultState = true

	clientIntentsName := "client-intents"
	policyName := "test-client-access"
	serviceName := "test-client"
	clientNamespace := testClientNamespace
	formattedTargetClient := "test-client-test-client-namespac-edb3a2"
	ips := []string{"10.1.2.2/32", "254.3.4.0/24"}

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
				Type: otterizev1alpha3.IntentTypeInternet,
				Internet: &otterizev1alpha3.Internet{
					Ips: ips,
				},
			},
		},
	}

	clientIntents := otterizev1alpha3.ClientIntents{Spec: intentsSpec}
	clientIntents.Namespace = clientNamespace
	clientIntents.Name = clientIntentsName
	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntents})

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: clientNamespace,
		Name:      policyName,
	}
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			return apierrors.NewNotFound(v1.Resource("networkpolicy"), name.Name)
		})

	newPolicy := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: clientNamespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeNetworkPolicy: formattedTargetClient,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha3.OtterizeServiceLabelKey: formattedTargetClient,
				},
			},
			Ingress: make([]v1.NetworkPolicyIngressRule, 0),
			Egress: []v1.NetworkPolicyEgressRule{
				{
					To: []v1.NetworkPolicyPeer{
						{
							IPBlock: &v1.IPBlock{
								CIDR: ips[0],
							},
						},
						{
							IPBlock: &v1.IPBlock{
								CIDR: ips[1],
							},
						},
					},
					Ports: []v1.NetworkPolicyPort{},
				},
			},
		},
	}
	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), gomock.Any(), gomock.Any())
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(newPolicy)).Return(nil)

	s.ignoreRemoveOrphan()

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedEgressNetworkPolicies)
}

func (s *InternetNetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicyForDNS() {
	clientIntentsName := "client-intents"
	policyName := "egress-to-internet-from-test-client"
	serviceName := "test-client"
	clientNamespace := testClientNamespace
	formattedTargetClient := "test-client-test-client-namespac-edb3a2"
	dns := "wiki.otters.com"
	ips := []string{"10.1.2.2/32", "254.3.4.0/24"}

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
				Type: otterizev1alpha3.IntentTypeInternet,
				Internet: &otterizev1alpha3.Internet{
					Dns: dns,
				},
			},
		},
	}

	intentsStatus := otterizev1alpha3.IntentsStatus{
		ResolvedIPs: []otterizev1alpha3.ResolvedIPs{
			{
				DNS: dns,
				IPs: ips,
			},
		},
	}
	clientIntents := otterizev1alpha3.ClientIntents{
		Spec:   intentsSpec,
		Status: intentsStatus,
	}
	clientIntents.Namespace = clientNamespace
	clientIntents.Name = clientIntentsName
	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntents})

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: clientNamespace,
		Name:      policyName,
	}
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			return apierrors.NewNotFound(v1.Resource("networkpolicy"), name.Name)
		})

	newPolicy := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: clientNamespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeInternetNetworkPolicy: formattedTargetClient,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha3.OtterizeClientLabelKey: formattedTargetClient,
				},
			},
			Egress: []v1.NetworkPolicyEgressRule{
				{
					To: []v1.NetworkPolicyPeer{
						{
							IPBlock: &v1.IPBlock{
								CIDR: ips[0],
							},
						},
						{
							IPBlock: &v1.IPBlock{
								CIDR: ips[1],
							},
						},
					},
					Ports: []v1.NetworkPolicyPort{},
				},
			},
		},
	}
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(newPolicy)).Return(nil)

	s.ignoreRemoveOrphan()

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedInternetEgressNetworkPolicies)
}

func (s *InternetNetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicyFromDNSAndIP() {
	clientIntentsName := "client-intents"
	policyName := "egress-to-internet-from-test-client"
	serviceName := "test-client"
	clientNamespace := testClientNamespace
	formattedTargetClient := "test-client-test-client-namespac-edb3a2"
	dns := "wiki.otters.com"
	ips := []string{"10.1.2.2/32", "254.3.4.0/24"}
	resolvedIP := ips[0]
	declaredIP := ips[1]

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
				Type: otterizev1alpha3.IntentTypeInternet,
				Internet: &otterizev1alpha3.Internet{
					Dns: dns,
					Ips: []string{declaredIP},
				},
			},
		},
	}

	intentsStatus := otterizev1alpha3.IntentsStatus{
		ResolvedIPs: []otterizev1alpha3.ResolvedIPs{
			{
				DNS: dns,
				IPs: []string{resolvedIP},
			},
		},
	}
	clientIntents := otterizev1alpha3.ClientIntents{
		Spec:   intentsSpec,
		Status: intentsStatus,
	}
	clientIntents.Namespace = clientNamespace
	clientIntents.Name = clientIntentsName
	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntents})

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: clientNamespace,
		Name:      policyName,
	}
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			return apierrors.NewNotFound(v1.Resource("networkpolicy"), name.Name)
		})

	newPolicy := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: clientNamespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeInternetNetworkPolicy: formattedTargetClient,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha3.OtterizeClientLabelKey: formattedTargetClient,
				},
			},
			Egress: []v1.NetworkPolicyEgressRule{
				{
					To: []v1.NetworkPolicyPeer{
						{
							IPBlock: &v1.IPBlock{
								CIDR: ips[0],
							},
						},
						{
							IPBlock: &v1.IPBlock{
								CIDR: ips[1],
							},
						},
					},
					Ports: []v1.NetworkPolicyPort{},
				},
			},
		},
	}
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(newPolicy)).Return(nil)

	s.ignoreRemoveOrphan()

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedInternetEgressNetworkPolicies)
}

func (s *InternetNetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicyMultipleEndpoints() {
	s.Reconciler.EnforcementDefaultState = true
	clientIntentsName := "client-intents"
	policyName := "test-client-access"
	serviceName := "test-client"
	clientNamespace := testClientNamespace
	formattedTargetClient := "test-client-test-client-namespac-edb3a2"
	namespacedName := types.NamespacedName{
		Namespace: testClientNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	// Enable access to any IP on endpoint A only at port 80 and endpoint B only at port 443
	endpointAIp1 := "10.1.2.2/32"
	endpointAip2 := "10.1.3.0/24"
	endpointAPort := 80
	endpointBIp := "254.3.4.0/24"
	endpointBPort := 443

	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Type: otterizev1alpha3.IntentTypeInternet,
				Internet: &otterizev1alpha3.Internet{
					Ips:   []string{endpointAIp1, endpointAip2},
					Ports: []int{endpointAPort},
				},
			},
			{
				Type: otterizev1alpha3.IntentTypeInternet,
				Internet: &otterizev1alpha3.Internet{
					Ips:   []string{endpointBIp},
					Ports: []int{endpointBPort},
				},
			},
		},
	}

	clientIntents := otterizev1alpha3.ClientIntents{Spec: intentsSpec}
	clientIntents.Namespace = clientNamespace
	clientIntents.Name = clientIntentsName
	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntents})

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: clientNamespace,
		Name:      policyName,
	}
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			return apierrors.NewNotFound(v1.Resource("networkpolicy"), name.Name)
		})

	newPolicy := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: clientNamespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeNetworkPolicy: formattedTargetClient,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha3.OtterizeServiceLabelKey: formattedTargetClient,
				},
			},
			Ingress: make([]v1.NetworkPolicyIngressRule, 0),
			Egress: []v1.NetworkPolicyEgressRule{
				{
					To: []v1.NetworkPolicyPeer{
						{
							IPBlock: &v1.IPBlock{
								CIDR: endpointAIp1,
							},
						},
						{
							IPBlock: &v1.IPBlock{
								CIDR: endpointAip2,
							},
						},
					},
					Ports: []v1.NetworkPolicyPort{
						{
							Port: &intstr.IntOrString{
								Type:   intstr.Int,
								IntVal: int32(endpointAPort),
							},
						},
					},
				},
				{
					To: []v1.NetworkPolicyPeer{
						{
							IPBlock: &v1.IPBlock{
								CIDR: endpointBIp,
							},
						},
					},
					Ports: []v1.NetworkPolicyPort{
						{
							Port: &intstr.IntOrString{
								Type:   intstr.Int,
								IntVal: int32(endpointBPort),
							},
						},
					},
				},
			},
		},
	}

	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), gomock.Any(), gomock.Any())
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(newPolicy)).Return(nil)

	s.ignoreRemoveOrphan()

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedEgressNetworkPolicies)
}

func (s *InternetNetworkPolicyReconcilerTestSuite) TestNetworkPolicyDeletedCleanup() {
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	clientNamespace := testClientNamespace
	formattedTargetClient := "test-client-test-client-namespac-edb3a2"
	ips := []string{"10.1.2.2/32", "254.3.4.0/24"}

	namespacedName := types.NamespacedName{
		Namespace: clientNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Type: otterizev1alpha3.IntentTypeInternet,
				Internet: &otterizev1alpha3.Internet{
					Ips: ips,
				}},
		},
	}

	clientIntentsObj := otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:              clientIntentsName,
			Namespace:         clientNamespace,
			DeletionTimestamp: &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)},
		},
		Spec: intentsSpec,
	}

	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntentsObj})

	// Remove network policy:
	// 1. get all effective policies - this intent will not create a policy cause it is being deleted
	// 2. remove the "old" policy by calling the removeNetworkPoliciesThatShouldNotExist function

	policyName := "test-client-access"

	existingPolicy := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: clientNamespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeNetworkPolicy: formattedTargetClient,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha3.OtterizeServiceLabelKey: formattedTargetClient,
				},
			},
			Ingress: make([]v1.NetworkPolicyIngressRule, 0),
			Egress: []v1.NetworkPolicyEgressRule{
				{
					To: []v1.NetworkPolicyPeer{
						{
							IPBlock: &v1.IPBlock{
								CIDR: ips[0],
							},
						},
						{
							IPBlock: &v1.IPBlock{
								CIDR: ips[1],
							},
						},
					},
					Ports: []v1.NetworkPolicyPort{},
				},
			},
		},
	}

	s.expectRemoveOrphanFindsPolicies([]v1.NetworkPolicy{*existingPolicy})

	s.ignoreRemoveDeprecatedPolicies()
	s.externalNetpolHandler.EXPECT().HandleBeforeAccessPolicyRemoval(gomock.Any(), gomock.Eq(existingPolicy))
	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(existingPolicy)).Return(nil)

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *InternetNetworkPolicyReconcilerTestSuite) TestUpdateNetworkPolicy() {
	s.Reconciler.EnforcementDefaultState = true

	clientIntentsName := "client-intents"
	policyName := "test-client-access"
	serviceName := "test-client"
	clientNamespace := testClientNamespace
	formattedTargetClient := "test-client-test-client-namespac-edb3a2"
	ips := []string{"10.1.2.2/32", "254.3.4.0/24"}

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
				Type: otterizev1alpha3.IntentTypeInternet,
				Internet: &otterizev1alpha3.Internet{
					Ips: ips,
				},
			},
		},
	}

	clientIntents := otterizev1alpha3.ClientIntents{Spec: intentsSpec}
	clientIntents.Namespace = clientNamespace
	clientIntents.Name = clientIntentsName
	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntents})

	s.ignoreRemoveOrphan()

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: clientNamespace,
		Name:      policyName,
	}

	newPolicy := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: clientNamespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeNetworkPolicy: formattedTargetClient,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha3.OtterizeServiceLabelKey: formattedTargetClient,
				},
			},
			Ingress: make([]v1.NetworkPolicyIngressRule, 0),
			Egress: []v1.NetworkPolicyEgressRule{
				{
					To: []v1.NetworkPolicyPeer{
						{
							IPBlock: &v1.IPBlock{
								CIDR: ips[0],
							},
						},
						{
							IPBlock: &v1.IPBlock{
								CIDR: ips[1],
							},
						},
					},
					Ports: []v1.NetworkPolicyPort{},
				},
			},
		},
	}
	existingBadPolicy := newPolicy.DeepCopy()
	existingBadPolicy.Spec.Egress = []v1.NetworkPolicyEgressRule{
		{
			To: []v1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"another-label": "just to create diff from real policy",
						},
					},
				},
			},
		},
	}
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			existingBadPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	// Update NetworkPolicy
	s.Client.EXPECT().Patch(gomock.Any(), gomock.Eq(newPolicy), intents_reconcilers.MatchPatch(client.MergeFrom(existingBadPolicy))).Return(nil)

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedEgressNetworkPolicies)
}

func (s *InternetNetworkPolicyReconcilerTestSuite) TestRemoveOrphanNetworkPolicy() {
	s.Reconciler.EnforcementDefaultState = true

	clientIntentsName := "client-intents"
	policyName := "test-client-access"
	serviceName := "test-client"
	clientNamespace := testClientNamespace
	formattedTargetClient := "test-client-test-client-namespac-edb3a2"
	ips := []string{"10.1.2.2/32"}

	namespacedName := types.NamespacedName{
		Namespace: testClientNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	clientIntents := otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientIntentsName,
			Namespace: clientNamespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeNetworkPolicy: formattedTargetClient,
			},
		},
		Spec: &otterizev1alpha3.IntentsSpec{
			Service: otterizev1alpha3.Service{Name: serviceName},
			Calls: []otterizev1alpha3.Intent{
				{
					Type: otterizev1alpha3.IntentTypeInternet,
					Internet: &otterizev1alpha3.Internet{
						Ips: ips,
					},
				},
			},
		},
	}

	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntents})

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: clientNamespace,
		Name:      policyName,
	}

	existingValidPolicy := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: clientNamespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeNetworkPolicy: formattedTargetClient,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha3.OtterizeServiceLabelKey: formattedTargetClient,
				},
			},
			Ingress: make([]v1.NetworkPolicyIngressRule, 0),
			Egress: []v1.NetworkPolicyEgressRule{
				{
					To: []v1.NetworkPolicyPeer{
						{
							IPBlock: &v1.IPBlock{
								CIDR: ips[0],
							},
						},
					},
					Ports: []v1.NetworkPolicyPort{},
				},
			},
		},
	}

	// Remove orphan network policy
	nonExistingClient := "old-non-existing-client"
	existingInvalidPolicy := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName + "invalid",
			Namespace: clientNamespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeNetworkPolicy: nonExistingClient,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha3.OtterizeServiceLabelKey: nonExistingClient,
				},
			},
			Ingress: make([]v1.NetworkPolicyIngressRule, 0),
			Egress: []v1.NetworkPolicyEgressRule{
				{
					To: []v1.NetworkPolicyPeer{
						{
							IPBlock: &v1.IPBlock{
								CIDR: "8.8.8.8",
							},
						},
					},
					Ports: []v1.NetworkPolicyPort{},
				},
			},
		},
	}

	labelSelector := metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha3.OtterizeNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
		{
			Key:      otterizev1alpha3.OtterizeNetworkPolicyExternalTraffic,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
		{
			Key:      otterizev1alpha3.OtterizeNetworkPolicyServiceDefaultDeny,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
	}}
	selector, err := metav1.LabelSelectorAsSelector(&labelSelector)
	s.Require().NoError(err)

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), gomock.Any()).DoAndReturn(
		func(ctx context.Context, list *v1.NetworkPolicyList, opts *client.ListOptions) error {
			s.Require().NotNil(opts)
			s.Require().NotNil(opts.LabelSelector)
			s.Require().Equal(selector.String(), opts.LabelSelector.String())
			list.Items = []v1.NetworkPolicy{*existingValidPolicy, *existingInvalidPolicy}
			return nil
		})

	s.ignoreRemoveDeprecatedPolicies()
	s.externalNetpolHandler.EXPECT().HandleBeforeAccessPolicyRemoval(gomock.Any(), gomock.Eq(existingInvalidPolicy))
	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(existingInvalidPolicy)).Return(nil)

	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			existingValidPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *InternetNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateEnforcementDisabled() {
	s.Reconciler.EnableNetworkPolicyCreation = false
	s.testEnforcementDisabled()
	s.ExpectEvent(consts.ReasonNetworkPolicyCreationDisabled)
}

func (s *InternetNetworkPolicyReconcilerTestSuite) TestNetworkGlobalEnforcementDisabled() {
	s.Reconciler.EnforcementDefaultState = false

	s.testEnforcementDisabled()
	s.ExpectEvent(consts.ReasonEnforcementDefaultOff)
}

func (s *InternetNetworkPolicyReconcilerTestSuite) TestNotInWatchedNamespaces() {
	s.Reconciler.RestrictToNamespaces = []string{"namespace-you-never-heard-of"}

	s.testEnforcementDisabled()
	s.ExpectEvent(consts.ReasonNamespaceNotAllowed)
}

func (s *InternetNetworkPolicyReconcilerTestSuite) testEnforcementDisabled() {
	clientIntentsName := "client-intents"
	serviceName := "test-client"

	namespacedName := types.NamespacedName{
		Namespace: testServerNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Type: otterizev1alpha3.IntentTypeInternet,
				Internet: &otterizev1alpha3.Internet{
					Ips: []string{"1.1.1.1/32"},
				},
			},
		},
	}

	clientIntents := otterizev1alpha3.ClientIntents{Spec: intentsSpec}
	clientIntents.Namespace = testServerNamespace
	clientIntents.Name = clientIntentsName
	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntents})

	s.ignoreRemoveOrphan()

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func TestInternetNetworkPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(InternetNetworkPolicyReconcilerTestSuite))
}
