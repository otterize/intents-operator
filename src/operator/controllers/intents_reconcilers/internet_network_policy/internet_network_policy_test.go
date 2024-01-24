package internet_network_policy

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
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

const (
	testServerNamespace = "test-server-namespace"
	testClientNamespace = "test-client-namespace"
)

type InternetNetworkPolicyReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	Reconciler            *InternetNetworkPolicyReconciler
	externalNetpolHandler *mocks.MockexternalNetpolHandler
}

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

func (s *InternetNetworkPolicyReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.externalNetpolHandler = mocks.NewMockexternalNetpolHandler(s.Controller)
	restrictToNamespaces := make([]string, 0)

	s.Reconciler = NewInternetNetworkPolicyReconciler(
		s.Client,
		&runtime.Scheme{},
		restrictToNamespaces,
		true,
		true,
	)

	s.Reconciler.Recorder = s.Recorder
}

func (s *InternetNetworkPolicyReconcilerTestSuite) TearDownTest() {
	viper.Reset()
	s.Reconciler = nil
	s.externalNetpolHandler = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *InternetNetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicySingleEndpoint() {
	s.Reconciler.enforcementDefaultState = true

	clientIntentsName := "client-intents"
	policyName := "egress-to-internet-from-test-client"
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

	// Initial call to get the ClientIntents object when reconciler starts
	emptyIntents := &otterizev1alpha3.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha3.ClientIntents, options ...client.ListOption) error {
			intents.Namespace = clientNamespace
			intents.Spec = intentsSpec
			return nil
		})

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

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedInternetEgressNetworkPolicies)
}

func (s *InternetNetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicyMultipleEndpoints() {
	s.Reconciler.enforcementDefaultState = true
	clientIntentsName := "client-intents"
	policyName := "egress-to-internet-from-test-client"
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

	// Initial call to get the ClientIntents object when reconciler starts
	emptyIntents := &otterizev1alpha3.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha3.ClientIntents, options ...client.ListOption) error {
			intents.Namespace = clientNamespace
			intents.Spec = intentsSpec
			return nil
		})

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

	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(newPolicy)).Return(nil)

	s.ignoreRemoveOrphan()

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedInternetEgressNetworkPolicies)
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

	// Initial call to get the ClientIntents object when reconciler starts
	emptyIntents := &otterizev1alpha3.ClientIntents{}
	clientIntentsObj := otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:              clientIntentsName,
			Namespace:         clientNamespace,
			DeletionTimestamp: &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)},
		},
		Spec: intentsSpec,
	}

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha3.ClientIntents, options ...client.ListOption) error {
			clientIntentsObj.DeepCopyInto(intents)
			return nil
		})

	s.ignoreRemoveOrphan()

	// Remove network policy:
	// 1. get the network policy
	// 2. delete it
	// 3.call external netpol handler

	policyName := "egress-to-internet-from-test-client"
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: clientNamespace,
		Name:      policyName,
	}

	existingPolicy := &v1.NetworkPolicy{
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

	emptyNetworkPolicy := &v1.NetworkPolicy{}
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			existingPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(existingPolicy)).Return(nil)

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *InternetNetworkPolicyReconcilerTestSuite) TestUpdateNetworkPolicy() {
	s.Reconciler.enforcementDefaultState = true

	clientIntentsName := "client-intents"
	policyName := "egress-to-internet-from-test-client"
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

	// Initial call to get the ClientIntents object when reconciler starts
	emptyIntents := &otterizev1alpha3.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha3.ClientIntents, options ...client.ListOption) error {
			intents.Namespace = clientNamespace
			intents.Spec = intentsSpec
			return nil
		})

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

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedInternetEgressNetworkPolicies)
}

func (s *InternetNetworkPolicyReconcilerTestSuite) TestRemoveOrphanNetworkPolicy() {
	s.Reconciler.enforcementDefaultState = true

	clientIntentsName := "client-intents"
	policyName := "egress-to-internet-from-test-client"
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
				otterizev1alpha3.OtterizeInternetNetworkPolicy: formattedTargetClient,
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
			Name:      policyName,
			Namespace: clientNamespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeInternetNetworkPolicy: nonExistingClient,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha3.OtterizeClientLabelKey: nonExistingClient,
				},
			},
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
			Key:      otterizev1alpha3.OtterizeInternetNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
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

	serverLabelSelector := client.MatchingFields{otterizev1alpha3.OtterizeFormattedTargetServerIndexField: formattedTargetClient}
	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.Eq(&otterizev1alpha3.ClientIntentsList{}),
		&serverLabelSelector).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha3.ClientIntentsList, options ...client.ListOption) error {
			list.Items = []otterizev1alpha3.ClientIntents{clientIntents}
			return nil
		})

	orphanServerLabelSelector := client.MatchingFields{otterizev1alpha3.OtterizeFormattedTargetServerIndexField: nonExistingClient}
	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.Eq(&otterizev1alpha3.ClientIntentsList{}),
		&orphanServerLabelSelector).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha3.ClientIntentsList, options ...client.ListOption) error {
			// There are no ClientIntents for this NetworkPolicy, so it should be deleted
			list.Items = []otterizev1alpha3.ClientIntents{}
			return nil
		})

	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(existingInvalidPolicy)).Return(nil)

	// Initial call to get the ClientIntents object when reconciler starts
	emptyIntents := &otterizev1alpha3.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha3.ClientIntents, options ...client.ListOption) error {
			clientIntents.DeepCopyInto(intents)
			return nil
		})

	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			existingValidPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedInternetEgressNetworkPolicies)
}

func (s *InternetNetworkPolicyReconcilerTestSuite) ignoreRemoveOrphan() {
	labelSelector := metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha3.OtterizeInternetNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
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
}

func (s *InternetNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateEnforcementDisabled() {
	s.Reconciler.enableNetworkPolicyCreation = false

	s.testEnforcementDisabled()
	s.ExpectEvent(consts.ReasonNetworkPolicyCreationDisabled)
}

func (s *InternetNetworkPolicyReconcilerTestSuite) TestNetworkGlobalEnforcementDisabled() {
	s.Reconciler.enforcementDefaultState = false

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

	// Initial call to get the ClientIntents object when reconciler starts
	emptyIntents := &otterizev1alpha3.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha3.ClientIntents, options ...client.ListOption) error {
			intents.Namespace = testClientNamespace
			intents.Spec = intentsSpec
			return nil
		})

	s.ignoreRemoveOrphan()

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func TestInternetNetworkPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(InternetNetworkPolicyReconcilerTestSuite))
}
