package egress_network_policy

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
	v1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"
)

const (
	testServerNamespace = "test-server-namespace"
	testClientNamespace = "test-client-namespace"
)

type EgressNetworkPolicyReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	Reconciler            *EgressNetworkPolicyReconciler
	EPIntentsReconciler   *intents_reconcilers.ServiceEffectivePolicyIntentsReconciler
	externalNetpolHandler *mocks.MockexternalNetpolHandler
}

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.externalNetpolHandler = mocks.NewMockexternalNetpolHandler(s.Controller)
	restrictToNamespaces := make([]string, 0)

	scheme := &runtime.Scheme{}
	s.Reconciler = NewEgressNetworkPolicyReconciler(
		s.Client,
		scheme,
		restrictToNamespaces,
		true,
		true,
	)

	s.Reconciler.Recorder = s.Recorder

	epReconciler := effectivepolicy.NewGroupReconciler(s.Client,
		scheme, s.Reconciler)
	s.EPIntentsReconciler = intents_reconcilers.NewServiceEffectiveIntentsReconciler(s.Client,
		scheme, epReconciler)

	s.Reconciler.Recorder = s.Recorder
	epReconciler.InjectableRecorder.Recorder = s.Recorder
	s.EPIntentsReconciler.Recorder = s.Recorder
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TearDownTest() {
	viper.Reset()
	s.Reconciler = nil
	s.externalNetpolHandler = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *EgressNetworkPolicyReconcilerTestSuite) expectGetAllEffectivePolicies(clientIntents []otterizev1alpha3.ClientIntents) {
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

func (s *EgressNetworkPolicyReconcilerTestSuite) expectRemoveOrphanFindsPolicies(netpols []v1.NetworkPolicy) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha3.OtterizeEgressNetworkPolicy,
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

func (s *EgressNetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicy() {
	clientIntentsName := "client-intents"
	policyName := "egress-from-test-client"
	serviceName := "test-client"
	serverNamespace := testServerNamespace
	clientNamespace := testClientNamespace
	formattedTargetServer := "test-server-test-server-namespac-48aee4"

	s.testCreateNetworkPolicy(
		clientIntentsName,
		clientNamespace,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
		true,
		nil,
	)
	s.ExpectEvent(consts.ReasonCreatedEgressNetworkPolicies)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateCrossNamespace() {
	clientIntentsName := "client-intents"
	policyName := "egress-from-test-client"
	serviceName := "test-client"
	serverNamespace := "other-namespace"
	clientNamespace := testClientNamespace
	formattedTargetServer := "test-server-other-namespace-f6a461"

	s.testCreateNetworkPolicy(
		clientIntentsName,
		clientNamespace,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
		true,
		nil,
	)
	s.ExpectEvent(consts.ReasonCreatedEgressNetworkPolicies)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCleanup() {
	clientIntentsName := "client-intents"
	policyName := "egress-from-test-client"
	serviceName := "test-client"
	clientNamespace := testClientNamespace
	serverNamespace := testServerNamespace
	formattedTargetServer := "test-server-test-server-namespac-48aee4"

	s.testCleanNetworkPolicy(
		clientIntentsName,
		clientNamespace,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
	)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCleanupCrossNamespace() {
	clientIntentsName := "client-intents"
	policyName := "egress-from-test-client"
	serviceName := "test-client"
	serverNamespace := "other-namespace"
	clientNamespace := testClientNamespace
	formattedTargetServer := "test-server-other-namespace-f6a461"

	s.testCleanNetworkPolicy(
		clientIntentsName,
		clientNamespace,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
	)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestUpdateNetworkPolicy() {
	clientIntentsName := "client-intents"
	policyName := "egress-from-test-client"
	serviceName := "test-client"
	serverNamespace := testServerNamespace
	formattedTargetServer := "test-server-test-server-namespac-48aee4"

	namespacedName := types.NamespacedName{
		Namespace: testClientNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Name: serverName,
			},
		},
	}

	clientIntents := otterizev1alpha3.ClientIntents{Spec: intentsSpec}
	clientIntents.Namespace = testClientNamespace
	clientIntents.Name = clientIntentsName

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: testClientNamespace,
		Name:      policyName,
	}

	newPolicy := networkPolicyTemplate(
		policyName,
		serverNamespace,
		"test-client-test-client-namespac-edb3a2",
		formattedTargetServer,
		testClientNamespace,
	)
	existingBadPolicy := newPolicy.DeepCopy()
	existingBadPolicy.Spec.Egress[0].To[0].PodSelector.MatchLabels["another-label"] = "just to create diff from real policy"
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			existingBadPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	// Update NetworkPolicy
	s.Client.EXPECT().Patch(gomock.Any(), gomock.Eq(newPolicy), intents_reconcilers.MatchPatch(client.MergeFrom(existingBadPolicy))).Return(nil)
	s.ignoreRemoveOrphan()
	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntents})

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedEgressNetworkPolicies)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestRemoveOrphanNetworkPolicy() {
	clientIntentsName := "client-intents"
	policyName := "egress-from-test-client"
	serviceName := "test-client"
	serverNamespace := testServerNamespace
	formattedTargetServer := "test-server-test-server-namespac-48aee4"

	namespacedName := types.NamespacedName{
		Namespace: testServerNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Name: serverName,
			},
		},
	}
	clientIntents := otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientIntentsName,
			Namespace: testServerNamespace,
		},
		Spec: intentsSpec,
	}

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      policyName,
	}

	existingPolicy := networkPolicyTemplate(
		policyName,
		serverNamespace,
		"test-client-test-server-namespac-8e2cac",
		formattedTargetServer,
		testServerNamespace,
	)
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			existingPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha3.OtterizeEgressNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
	}})
	s.Require().NoError(err)

	nonExistingServer := "old-non-existing-server"
	orphanPolicy := networkPolicyTemplate(
		"egress-from-test-server-namespace",
		serverNamespace,
		"test-client-test-server-namespac-8e2cac",
		nonExistingServer,
		testServerNamespace,
	)
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), &client.ListOptions{LabelSelector: selector}).DoAndReturn(
		func(ctx context.Context, list *v1.NetworkPolicyList, options ...client.ListOption) error {
			list.Items = []v1.NetworkPolicy{*existingPolicy, *orphanPolicy}
			return nil
		})

	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(orphanPolicy)).Return(nil)
	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntents})
	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedEgressNetworkPolicies)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) testCleanNetworkPolicy(clientIntentsName string, clientNamespace string, serverNamespace string, serviceName string, policyName string, formattedTargetServer string) {
	namespacedName := types.NamespacedName{
		Namespace: clientNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
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
			Namespace:         clientNamespace,
			DeletionTimestamp: &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)},
		},
		Spec: intentsSpec,
	}
	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntentsObj})

	// Remove network policy:
	// 1. get the network policy
	// 2. delete it
	// 3.call external netpol handler

	existingPolicy := networkPolicyTemplate(
		policyName,
		clientNamespace,
		"test-client-test-client-namespac-edb3a2",
		formattedTargetServer,
		testServerNamespace,
	)

	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(existingPolicy)).Return(nil)

	s.expectRemoveOrphanFindsPolicies([]v1.NetworkPolicy{*existingPolicy})
	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) testCreateNetworkPolicy(
	clientIntentsName string,
	clientNamespace string,
	serverNamespace string,
	serviceName string,
	policyName string,
	formattedTargetServer string,
	defaultEnforcementState bool,
	protectedServices []otterizev1alpha3.ProtectedService,
) {
	s.Reconciler.enforcementDefaultState = defaultEnforcementState
	namespacedName := types.NamespacedName{
		Namespace: testClientNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Name: serverName,
			},
		},
	}

	// Initial call to get the ClientIntents object when reconciler starts
	clientIntents := otterizev1alpha3.ClientIntents{Spec: intentsSpec}
	clientIntents.Namespace = clientNamespace
	clientIntents.Name = clientIntentsName

	if defaultEnforcementState == false {
		s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha3.ProtectedServiceList{}), gomock.Any()).DoAndReturn(
			func(ctx context.Context, list *otterizev1alpha3.ProtectedServiceList, opts ...client.ListOption) error {
				list.Items = append(list.Items, protectedServices...)
				return nil
			})
	}

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

	// Create NetworkPolicy
	newPolicy := networkPolicyTemplate(
		policyName,
		serverNamespace,
		"test-client-test-client-namespac-edb3a2",
		formattedTargetServer,
		testClientNamespace,
	)
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(newPolicy)).Return(nil)

	s.ignoreRemoveOrphan()

	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntents})
	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) ignoreRemoveOrphan() {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha3.OtterizeEgressNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
	}})
	s.Require().NoError(err)

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), &client.ListOptions{LabelSelector: selector}).Return(nil)
}

func networkPolicyTemplate(
	policyName string,
	targetNamespace string,
	formattedTargetClient string,
	formattedTargetServer string,
	intentsObjNamespace string,
) *v1.NetworkPolicy {
	return &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: intentsObjNamespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeEgressNetworkPolicy: formattedTargetClient,
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
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									otterizev1alpha3.OtterizeServerLabelKey: formattedTargetServer,
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									otterizev1alpha3.KubernetesStandardNamespaceNameLabelKey: targetNamespace,
								},
							},
						},
					},
				},
			},
		},
	}
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateEnforcementDisabled() {
	s.Reconciler.enableNetworkPolicyCreation = false

	s.testEnforcementDisabled()
	s.ExpectEvent(consts.ReasonNetworkPolicyCreationDisabled)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestNetworkGlobalEnforcementDisabled() {
	s.Reconciler.enforcementDefaultState = false

	s.testEnforcementDisabled()
	s.ExpectEvent(consts.ReasonEnforcementDefaultOff)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestNotInWatchedNamespaces() {
	s.Reconciler.RestrictToNamespaces = []string{"namespace-you-never-heard-of"}

	s.testEnforcementDisabled()
	s.ExpectEvent(consts.ReasonNamespaceNotAllowed)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) testEnforcementDisabled() {
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	serverNamespace := "other-namespace"

	namespacedName := types.NamespacedName{
		Namespace: testServerNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
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

	s.ignoreRemoveOrphan()
	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntents})

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func TestEgressNetworkPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(EgressNetworkPolicyReconcilerTestSuite))
}
