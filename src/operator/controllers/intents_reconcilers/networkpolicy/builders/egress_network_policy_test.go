package builders

import (
	"context"
	"fmt"
	otterizev2 "github.com/otterize/intents-operator/src/operator/api/v2"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"
)

type EgressNetworkPolicyReconcilerTestSuite struct {
	RulesBuilderTestSuiteBase
	Builder *EgressNetworkPolicyBuilder
}

func (s *EgressNetworkPolicyReconcilerTestSuite) SetupTest() {
	s.RulesBuilderTestSuiteBase.SetupTest()
	s.Builder = NewEgressNetworkPolicyBuilder()
	s.Builder.Recorder = s.Recorder
	s.Reconciler.AddEgressRuleBuilder(s.Builder)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TearDownTest() {
	s.RulesBuilderTestSuiteBase.TearDownTest()
	s.Builder = nil
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicy() {
	clientIntentsName := "client-intents"
	policyName := "test-client-access"
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
	policyName := "test-client-access"
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
	policyName := "test-client-access"
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
	policyName := "test-client-access"
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
	policyName := "test-client-access"
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
	intentsSpec := &otterizev2.IntentsSpec{
		Workload: otterizev2.Workload{Name: serviceName},
		Targets: []otterizev2.Target{
			{
				Kubernetes: &otterizev2.KubernetesTarget{Name: serverName},
			},
		},
	}

	clientIntents := otterizev2.ClientIntents{Spec: intentsSpec}
	clientIntents.Namespace = testClientNamespace
	clientIntents.Name = clientIntentsName

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: testClientNamespace,
		Name:      policyName,
	}

	newPolicy := networkPolicyEgressTemplate(
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
	s.expectGetAllEffectivePolicies([]otterizev2.ClientIntents{clientIntents})

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedEgressNetworkPolicies)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestRemoveOrphanNetworkPolicy() {
	clientIntentsName := "client-intents"
	policyName := "test-client-access"
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
	intentsSpec := &otterizev2.IntentsSpec{
		Workload: otterizev2.Workload{Name: serviceName},
		Targets: []otterizev2.Target{
			{
				Kubernetes: &otterizev2.KubernetesTarget{Name: serverName},
			},
		},
	}
	clientIntents := otterizev2.ClientIntents{
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

	existingPolicy := networkPolicyEgressTemplate(
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
			Key:      otterizev2.OtterizeNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
		{
			Key:      otterizev2.OtterizeNetworkPolicyExternalTraffic,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
		{
			Key:      otterizev2.OtterizeNetworkPolicyServiceDefaultDeny,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
	}})
	s.Require().NoError(err)

	nonExistingServer := "old-non-existing-server"
	orphanPolicy := networkPolicyEgressTemplate(
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

	s.externalNetpolHandler.EXPECT().HandleBeforeAccessPolicyRemoval(gomock.Any(), gomock.Eq(orphanPolicy))
	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(orphanPolicy)).Return(nil)
	s.expectGetAllEffectivePolicies([]otterizev2.ClientIntents{clientIntents})
	s.ignoreRemoveDeprecatedPolicies()
	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
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
	intentsSpec := &otterizev2.IntentsSpec{
		Workload: otterizev2.Workload{Name: serviceName},
		Targets: []otterizev2.Target{
			{
				Kubernetes: &otterizev2.KubernetesTarget{Name: serverName},
			},
		},
	}

	clientIntentsObj := otterizev2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:              clientIntentsName,
			Namespace:         clientNamespace,
			DeletionTimestamp: &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)},
		},
		Spec: intentsSpec,
	}
	s.expectGetAllEffectivePolicies([]otterizev2.ClientIntents{clientIntentsObj})

	// Remove network policy:
	// 1. get the network policy
	// 2. delete it
	// 3.call external netpol handler

	existingPolicy := networkPolicyEgressTemplate(
		policyName,
		clientNamespace,
		"test-client-test-client-namespac-edb3a2",
		formattedTargetServer,
		testServerNamespace,
	)

	s.externalNetpolHandler.EXPECT().HandleBeforeAccessPolicyRemoval(gomock.Any(), gomock.Eq(existingPolicy))
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
	protectedServices []otterizev2.ProtectedService,
) {
	s.Reconciler.EnforcementDefaultState = defaultEnforcementState
	namespacedName := types.NamespacedName{
		Namespace: testClientNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev2.IntentsSpec{
		Workload: otterizev2.Workload{Name: serviceName},
		Targets: []otterizev2.Target{
			{
				Kubernetes: &otterizev2.KubernetesTarget{Name: serverName},
			},
		},
	}

	// Initial call to get the ClientIntents object when reconciler starts
	clientIntents := otterizev2.ClientIntents{Spec: intentsSpec}
	clientIntents.Namespace = clientNamespace
	clientIntents.Name = clientIntentsName

	if defaultEnforcementState == false {
		s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev2.ProtectedServiceList{}), gomock.Any()).DoAndReturn(
			func(ctx context.Context, list *otterizev2.ProtectedServiceList, opts ...client.ListOption) error {
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
	newPolicy := networkPolicyEgressTemplate(
		policyName,
		serverNamespace,
		"test-client-test-client-namespac-edb3a2",
		formattedTargetServer,
		testClientNamespace,
	)
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(newPolicy)).Return(nil)

	s.ignoreRemoveOrphan()

	s.expectGetAllEffectivePolicies([]otterizev2.ClientIntents{clientIntents})
	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), gomock.Any(), gomock.Any())
	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func networkPolicyEgressTemplate(
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
				otterizev2.OtterizeNetworkPolicy: formattedTargetClient,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev2.OtterizeServiceLabelKey: formattedTargetClient,
				},
			},
			Egress: []v1.NetworkPolicyEgressRule{
				{
					To: []v1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									otterizev2.OtterizeServiceLabelKey: formattedTargetServer,
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									otterizev2.KubernetesStandardNamespaceNameLabelKey: targetNamespace,
								},
							},
						},
					},
				},
				{
					To: []v1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									testServerServiceLabelKey: testServerServiceLabelValue,
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									otterizev2.KubernetesStandardNamespaceNameLabelKey: targetNamespace,
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
	s.Reconciler.EnableNetworkPolicyCreation = false

	s.testEnforcementDisabled()
	s.ExpectEvent(consts.ReasonNetworkPolicyCreationDisabled)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestNetworkGlobalEnforcementDisabled() {
	s.Reconciler.EnforcementDefaultState = false

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
	intentsSpec := &otterizev2.IntentsSpec{
		Workload: otterizev2.Workload{Name: serviceName},
		Targets: []otterizev2.Target{
			{
				Kubernetes: &otterizev2.KubernetesTarget{Name: serverName},
			},
		},
	}

	clientIntents := otterizev2.ClientIntents{Spec: intentsSpec}
	clientIntents.Name = clientIntentsName
	clientIntents.Namespace = testClientNamespace

	s.ignoreRemoveOrphan()
	s.expectGetAllEffectivePolicies([]otterizev2.ClientIntents{clientIntents})

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func TestEgressNetworkPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(EgressNetworkPolicyReconcilerTestSuite))
}
