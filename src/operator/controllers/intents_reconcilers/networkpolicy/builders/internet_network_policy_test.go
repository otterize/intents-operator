package builders

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/networkpolicy"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"
)

type InternetNetworkPolicyReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	Builder               *InternetEgressRulesBuilder
	Reconciler            *networkpolicy.Reconciler
	externalNetpolHandler *mocks.MockexternalNetpolHandler
	EPIntentsReconciler   *intents_reconcilers.ServiceEffectivePolicyIntentsReconciler
}

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

func (s *InternetNetworkPolicyReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.externalNetpolHandler = mocks.NewMockexternalNetpolHandler(s.Controller)
	restrictToNamespaces := make([]string, 0)

	scheme := &runtime.Scheme{}
	s.Builder = NewInternetEgressRulesBuilder()
	s.Reconciler = networkpolicy.NewReconciler(s.Client,
		scheme,
		s.externalNetpolHandler,
		restrictToNamespaces,
		true,
		true,
		nil,
		[]networkpolicy.EgressRuleBuilder{s.Builder})

	EPIntentsReconciler := effectivepolicy.NewGroupReconciler(s.Client,
		scheme, s.Reconciler)
	s.EPIntentsReconciler = intents_reconcilers.NewServiceEffectiveIntentsReconciler(s.Client,
		scheme, EPIntentsReconciler)

	s.Builder.Recorder = s.Recorder
	EPIntentsReconciler.InjectableRecorder.Recorder = s.Recorder
	s.EPIntentsReconciler.Recorder = s.Recorder
}

func (s *InternetNetworkPolicyReconcilerTestSuite) TearDownTest() {
	viper.Reset()
	s.Builder = nil
	s.externalNetpolHandler = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *InternetNetworkPolicyReconcilerTestSuite) expectGetAllEffectivePolicies(clientIntents []otterizev1alpha3.ClientIntents) {
	var intentsList otterizev1alpha3.ClientIntentsList

	s.Client.EXPECT().List(gomock.Any(), &intentsList).DoAndReturn(func(_ context.Context, intents *otterizev1alpha3.ClientIntentsList, _ ...any) error {
		intents.Items = append(intents.Items, clientIntents...)
		return nil
	})

	// create service to ClientIntents pointing to it
	services := make(map[string][]otterizev1alpha3.ClientIntents)
	for _, clientIntent := range clientIntents {
		for _, intentCall := range clientIntent.GetCallsList() {
			server := otterizev1alpha3.GetFormattedOtterizeIdentity(intentCall.GetTargetServerName(), intentCall.GetTargetServerNamespace(clientIntent.Namespace))
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

func (s *InternetNetworkPolicyReconcilerTestSuite) ignoreRemoveOrphan() {
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
		func(ctx context.Context, list client.ObjectList, opts *client.ListOptions) error {
			s.Require().NotNil(opts)
			s.Require().NotNil(opts.LabelSelector)
			s.Require().Equal(selector.String(), opts.LabelSelector.String())
			return nil
		})

	s.ignoreRemoveDeprecatedPolicies()
}

func (s *InternetNetworkPolicyReconcilerTestSuite) ignoreRemoveDeprecatedPolicies() {
	deprecatedLabels := []string{otterizev1alpha3.OtterizeEgressNetworkPolicy, otterizev1alpha3.OtterizeSvcEgressNetworkPolicy, otterizev1alpha3.OtterizeInternetNetworkPolicy, otterizev1alpha3.OtterizeSvcNetworkPolicy}
	for _, label := range deprecatedLabels {
		selectorRequirement := metav1.LabelSelectorRequirement{
			Key:      label,
			Operator: metav1.LabelSelectorOpExists,
		}
		selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
			selectorRequirement,
		}})

		s.Require().NoError(err)

		s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), &client.ListOptions{LabelSelector: selector}).Return(nil).AnyTimes()
	}
}

func (s *InternetNetworkPolicyReconcilerTestSuite) expectRemoveOrphanFindsPolicies(netpols []v1.NetworkPolicy) {
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

	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), &client.ListOptions{LabelSelector: selector},
	).DoAndReturn(
		func(_ context.Context, netpolList *v1.NetworkPolicyList, _ ...any) error {
			netpolList.Items = append(netpolList.Items, netpols...)
			return nil
		},
	)
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
