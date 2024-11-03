package builders

import (
	"context"
	"fmt"
	"github.com/amit7itz/goset"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"
)

type NetworkPolicyReconcilerTestSuite struct {
	RulesBuilderTestSuiteBase
	Builder *IngressNetpolBuilder
}

func (s *NetworkPolicyReconcilerTestSuite) SetupTest() {
	s.RulesBuilderTestSuiteBase.SetupTest()
	s.Builder = NewIngressNetpolBuilder()
	s.Builder.Recorder = s.Recorder
	s.Reconciler.AddIngressRuleBuilder(s.Builder)
}

func (s *NetworkPolicyReconcilerTestSuite) TearDownTest() {
	s.RulesBuilderTestSuiteBase.TearDownTest()
	s.Builder = nil
}

func (s *NetworkPolicyReconcilerTestSuite) testCreateNetworkPolicy(
	clientIntentsName string,
	serverNamespace string,
	serviceName string,
	policyName string,
	formattedTargetServer string,
	defaultEnforcementState bool,
	protectedServices []otterizev2alpha1.ProtectedService,
	enforcedNamespaces *goset.Set[string],
) {
	s.Reconciler.EnforcedNamespaces = enforcedNamespaces
	s.Reconciler.EnforcementDefaultState = defaultEnforcementState
	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}
	intentsSpec := &otterizev2alpha1.IntentsSpec{
		Workload: otterizev2alpha1.Workload{Name: serviceName},
		Targets: []otterizev2alpha1.Target{
			{
				Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: fmt.Sprintf("test-server.%s", serverNamespace)},
			},
		},
	}

	s.expectGetAllEffectivePolicies([]otterizev2alpha1.ClientIntents{{Spec: intentsSpec, ObjectMeta: metav1.ObjectMeta{Name: namespacedName.Name, Namespace: namespacedName.Namespace}}})

	if defaultEnforcementState == false {
		s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev2alpha1.ProtectedServiceList{}), gomock.Any()).DoAndReturn(
			func(ctx context.Context, list *otterizev2alpha1.ProtectedServiceList, opts ...client.ListOption) error {
				list.Items = append(list.Items, protectedServices...)
				return nil
			}).AnyTimes()
	}

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
	newPolicy := networkPolicyIngressTemplate(
		policyName,
		serverNamespace,
		formattedTargetServer,
		testNamespace,
	)
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(newPolicy)).Return(nil)

	selector := labels.SelectorFromSet(labels.Set(map[string]string{
		otterizev2alpha1.OtterizeServiceLabelKey: formattedTargetServer,
	}))

	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), serverNamespace, selector)
	s.ignoreRemoveOrphan()

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *NetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicyIgnoreAWS() {
	clientIntentsName := "client-intents"
	policyName := "test-server-access"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedTargetServer := "test-server-test-namespace-8ddecb"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}
	intentsSpec := &otterizev2alpha1.IntentsSpec{
		Workload: otterizev2alpha1.Workload{Name: serviceName},
		Targets: []otterizev2alpha1.Target{
			{
				Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: fmt.Sprintf("test-server.%s", serverNamespace)},
			},
			{
				AWS: &otterizev2alpha1.AWSTarget{
					ARN: "ARN:test123",
				},
			},
		},
	}

	s.expectGetAllEffectivePolicies([]otterizev2alpha1.ClientIntents{{Spec: intentsSpec, ObjectMeta: metav1.ObjectMeta{Name: namespacedName.Name, Namespace: namespacedName.Namespace}}})

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
	newPolicy := networkPolicyIngressTemplate(
		policyName,
		serverNamespace,
		formattedTargetServer,
		testNamespace,
	)
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(newPolicy)).Return(nil)

	selector := labels.SelectorFromSet(labels.Set(map[string]string{
		otterizev2alpha1.OtterizeServiceLabelKey: formattedTargetServer,
	}))

	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), serverNamespace, selector)
	s.ignoreRemoveOrphan()

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicy() {
	clientIntentsName := "client-intents"
	policyName := "test-server-access"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedTargetServer := "test-server-test-namespace-8ddecb"

	s.testCreateNetworkPolicy(
		clientIntentsName,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
		true,
		nil,
		goset.NewSet[string](),
	)
	s.ExpectEvent(consts.ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicyActiveNamespace() {
	clientIntentsName := "client-intents"
	policyName := "test-server-access"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedTargetServer := "test-server-test-namespace-8ddecb"

	s.testCreateNetworkPolicy(
		clientIntentsName,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
		false,
		nil,
		goset.FromSlice([]string{serverNamespace}),
	)
	s.ExpectEvent(consts.ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicyWithProtectedServices() {
	clientIntentsName := "client-intents"
	policyName := "test-server-access"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedTargetServer := "test-server-test-namespace-8ddecb"

	protectedService := []otterizev2alpha1.ProtectedService{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-protected-services",
			Namespace: testNamespace,
		},
		Spec: otterizev2alpha1.ProtectedServiceSpec{
			Name: "test-server",
		},
	}}

	s.testCreateNetworkPolicy(
		clientIntentsName,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
		false,
		protectedService,
		goset.NewSet[string](),
	)
	s.ExpectEvent(consts.ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicyWithProtectedServicesMultipleResources() {
	clientIntentsName := "client-intents"
	policyName := "test-server-access"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedTargetServer := "test-server-test-namespace-8ddecb"

	protectedServices := []otterizev2alpha1.ProtectedService{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-protected-services",
				Namespace: testNamespace,
			},
			Spec: otterizev2alpha1.ProtectedServiceSpec{
				Name: "other-server",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-protected-services-2",
				Namespace: testNamespace,
			},
			Spec: otterizev2alpha1.ProtectedServiceSpec{
				Name: "test-server",
			},
		},
	}

	s.testCreateNetworkPolicy(
		clientIntentsName,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
		false,
		protectedServices,
		goset.NewSet[string](),
	)
	s.ExpectEvent(consts.ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateCrossNamespace() {
	clientIntentsName := "client-intents"
	policyName := "test-server-access"
	serviceName := "test-client"
	serverNamespace := "other-namespace"
	formattedTargetServer := "test-server-other-namespace-f6a461"

	s.testCreateNetworkPolicy(
		clientIntentsName,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
		true,
		nil,
		goset.NewSet[string](),
	)
	s.ExpectEvent(consts.ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNetworkPolicyCleanup() {
	clientIntentsName := "client-intents"
	policyName := "test-server-access"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedTargetServer := "test-server-test-namespace-8ddecb"

	s.testCleanNetworkPolicy(
		clientIntentsName,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
	)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNetworkPolicyCleanupCrossNamespace() {
	clientIntentsName := "client-intents"
	policyName := "test-server-access"
	serviceName := "test-client"
	serverNamespace := "other-namespace"
	formattedTargetServer := "test-server-other-namespace-f6a461"

	s.testCleanNetworkPolicy(
		clientIntentsName,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
	)
}

func (s *NetworkPolicyReconcilerTestSuite) TestUpdateNetworkPolicy() {
	clientIntentsName := "client-intents"
	policyName := "test-server-access"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedTargetServer := "test-server-test-namespace-8ddecb"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev2alpha1.IntentsSpec{
		Workload: otterizev2alpha1.Workload{Name: serviceName},
		Targets: []otterizev2alpha1.Target{
			{
				Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: serverName},
			},
		},
	}

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      policyName,
	}

	newPolicy := networkPolicyIngressTemplate(
		policyName,
		serverNamespace,
		formattedTargetServer,
		testNamespace,
	)
	existingBadPolicy := newPolicy.DeepCopy()
	existingBadPolicy.Spec.Ingress[0].From[0].PodSelector.MatchLabels["another-label"] = "just to create diff from real policy"
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			existingBadPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	// Update NetworkPolicy
	s.Client.EXPECT().Patch(gomock.Any(), gomock.Eq(newPolicy), intents_reconcilers.MatchPatch(client.MergeFrom(existingBadPolicy))).Return(nil)
	s.ignoreRemoveOrphan()

	s.expectGetAllEffectivePolicies([]otterizev2alpha1.ClientIntents{{ObjectMeta: metav1.ObjectMeta{Namespace: req.Namespace, Name: req.Name}, Spec: intentsSpec}})
	selector, err := metav1.LabelSelectorAsSelector(&newPolicy.Spec.PodSelector)
	s.Require().NoError(err)
	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), serverNamespace, gomock.Eq(selector))
	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) TestRemoveOrphanNetworkPolicy() {
	clientIntentsName := "client-intents"
	policyName := "test-server-access"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedTargetServer := "test-server-test-namespace-8ddecb"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev2alpha1.IntentsSpec{
		Workload: otterizev2alpha1.Workload{Name: serviceName},
		Targets: []otterizev2alpha1.Target{
			{
				Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: serverName},
			},
		},
	}

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      policyName,
	}

	existingPolicy := networkPolicyIngressTemplate(
		policyName,
		serverNamespace,
		formattedTargetServer,
		testNamespace,
	)
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			existingPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev2alpha1.OtterizeNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
		{
			Key:      otterizev2alpha1.OtterizeNetworkPolicyExternalTraffic,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
		{
			Key:      otterizev2alpha1.OtterizeNetworkPolicyServiceDefaultDeny,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
	}})
	s.Require().NoError(err)

	nonExistingServer := "old-non-existing-server"
	orphanPolicy := networkPolicyIngressTemplate(
		"access-to-old-non-existing-server",
		serverNamespace,
		nonExistingServer,
		testNamespace,
	)
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), &client.ListOptions{LabelSelector: selector}).DoAndReturn(
		func(ctx context.Context, list *v1.NetworkPolicyList, options ...client.ListOption) error {
			list.Items = []v1.NetworkPolicy{*existingPolicy, *orphanPolicy}
			return nil
		})

	s.externalNetpolHandler.EXPECT().HandleBeforeAccessPolicyRemoval(gomock.Any(), orphanPolicy)
	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(orphanPolicy)).Return(nil)

	s.expectGetAllEffectivePolicies([]otterizev2alpha1.ClientIntents{{ObjectMeta: metav1.ObjectMeta{Namespace: req.Namespace, Name: req.Name}, Spec: intentsSpec}})
	s.ignoreRemoveDeprecatedPolicies()

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *NetworkPolicyReconcilerTestSuite) testCleanNetworkPolicy(clientIntentsName string, serverNamespace string, serviceName string, policyName string, formattedTargetServer string) {
	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev2alpha1.IntentsSpec{
		Workload: otterizev2alpha1.Workload{Name: serviceName},
		Targets: []otterizev2alpha1.Target{
			{
				Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: serverName},
			},
		},
	}

	clientIntentsObj := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:              clientIntentsName,
			Namespace:         testNamespace,
			DeletionTimestamp: &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)},
		},
		Spec: intentsSpec,
	}

	// Remove network policy:
	// 1. Calls remove orphan
	// 2. delete it
	// 3.call external netpol handler

	existingPolicy := networkPolicyIngressTemplate(
		policyName,
		serverNamespace,
		formattedTargetServer,
		testNamespace,
	)

	s.externalNetpolHandler.EXPECT().HandleBeforeAccessPolicyRemoval(gomock.Any(), existingPolicy)
	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(existingPolicy)).Return(nil)

	s.expectGetAllEffectivePolicies([]otterizev2alpha1.ClientIntents{clientIntentsObj})
	s.expectRemoveOrphanFindsPolicies([]v1.NetworkPolicy{*existingPolicy})
	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func networkPolicyIngressTemplate(
	policyName string,
	targetNamespace string,
	formattedTargetServer string,
	intentsObjNamespaces ...string,
) *v1.NetworkPolicy {
	ingressRules := lo.Map(intentsObjNamespaces, func(namespace string, _ int) v1.NetworkPolicyIngressRule {
		return v1.NetworkPolicyIngressRule{
			From: []v1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							fmt.Sprintf(
								otterizev2alpha1.OtterizeAccessLabelKey, formattedTargetServer): "true",
						},
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							otterizev2alpha1.KubernetesStandardNamespaceNameLabelKey: namespace,
						},
					},
				},
			},
		}
	})
	return &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: targetNamespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeNetworkPolicy: formattedTargetServer,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev2alpha1.OtterizeServiceLabelKey: formattedTargetServer,
				},
			},
			Ingress: ingressRules,
		},
	}
}

func (s *NetworkPolicyReconcilerTestSuite) TestServerNotInProtectionList() {
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	serverNamespace := "other-namespace"
	serverName := "test-server"
	s.Reconciler.EnforcementDefaultState = false

	s.testServerNotProtected(clientIntentsName, serverName, serverNamespace, serviceName)
	s.ExpectEvent(consts.ReasonEnforcementDefaultOff)
}

func (s *NetworkPolicyReconcilerTestSuite) testServerNotProtected(clientIntentsName string, serverName string, serverNamespace string, serviceName string) {
	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverFullName := fmt.Sprintf("%s.%s", serverName, serverNamespace)
	intentsSpec := &otterizev2alpha1.IntentsSpec{
		Workload: otterizev2alpha1.Workload{Name: serviceName},
		Targets: []otterizev2alpha1.Target{
			{
				Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: serverFullName},
			},
		},
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev2alpha1.ProtectedServiceList{}), gomock.Any()).DoAndReturn(
		func(ctx context.Context, list *otterizev2alpha1.ProtectedServiceList, opts ...client.ListOption) error {
			return nil
		}).AnyTimes()

	s.ignoreRemoveOrphan()

	s.expectGetAllEffectivePolicies([]otterizev2alpha1.ClientIntents{{ObjectMeta: metav1.ObjectMeta{Namespace: req.Namespace, Name: req.Name}, Spec: intentsSpec}})

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateEnforcementDisabled() {
	s.Reconciler.EnableNetworkPolicyCreation = false

	s.testEnforcementDisabled()
	s.ExpectEvent(consts.ReasonNetworkPolicyCreationDisabled)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNetworkGlobalEnforcementDisabled() {
	s.Reconciler.EnforcementDefaultState = false

	s.testEnforcementDisabled()
	s.ExpectEvent(consts.ReasonEnforcementDefaultOff)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNotInWatchedNamespaces() {
	s.Reconciler.RestrictToNamespaces = []string{"namespace-you-never-heard-of"}

	s.testEnforcementDisabled()
	s.ExpectEvent(consts.ReasonNamespaceNotAllowed)
}

func (s *NetworkPolicyReconcilerTestSuite) testEnforcementDisabled() {
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	serverNamespace := "other-namespace"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev2alpha1.IntentsSpec{
		Workload: otterizev2alpha1.Workload{Name: serviceName},
		Targets: []otterizev2alpha1.Target{
			{
				Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: serverName},
			},
		},
	}

	s.expectGetAllEffectivePolicies([]otterizev2alpha1.ClientIntents{{Spec: intentsSpec, ObjectMeta: metav1.ObjectMeta{Name: namespacedName.Name, Namespace: namespacedName.Namespace}}})

	s.ignoreRemoveOrphan()
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev2alpha1.ProtectedServiceList{}), gomock.Any()).AnyTimes().DoAndReturn(
		func(ctx context.Context, list *otterizev2alpha1.ProtectedServiceList, opts ...client.ListOption) error {
			return nil
		})

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *NetworkPolicyReconcilerTestSuite) TestPolicyNotDeletedForTwoClientsWithSameServer() {
	// This test checks that if a netpol created by 2 different intents and 1 of them is being deleted,
	// then the netpol won't be deleted.
	// The flow of the reconciler should be:
	// 	  1. get all effective policies
	//    2. get existing policy and not update because it is equal
	//    3. check if deletion is needed (the answer should be no)
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	serverNamespace := testNamespace
	policyName := "test-server-access"
	formattedTargetServer := "test-server-test-namespace-8ddecb"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev2alpha1.IntentsSpec{
		Workload: otterizev2alpha1.Workload{Name: serviceName},
		Targets: []otterizev2alpha1.Target{
			{
				Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: serverName},
			},
		},
	}

	clientIntentsObj := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:              clientIntentsName,
			Namespace:         testNamespace,
			DeletionTimestamp: &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)},
		},
		Spec: intentsSpec,
	}

	// Search for client intents with the same target server and delete them if it's the last client intent pointing to the server

	otherClientIntentsInTheNamespace := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-client-intents",
			Namespace: testNamespace,
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{Name: "other-client"},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: serverName},
				},
			},
		},
	}

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      policyName,
	}

	newPolicy := networkPolicyIngressTemplate(
		policyName,
		serverNamespace,
		formattedTargetServer,
		testNamespace,
	)

	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			newPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	s.expectGetAllEffectivePolicies([]otterizev2alpha1.ClientIntents{clientIntentsObj, otherClientIntentsInTheNamespace})
	s.expectRemoveOrphanFindsPolicies([]v1.NetworkPolicy{*newPolicy})

	// Shouldn't remove NetworkPolicy from this namespace
	// because there is another client with the same target server
	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

// This test checks that for 2 different intents from different namespaces to the same target server
// only one networkPolicy will be creted, and it will have 2 ingress rules (one for each namespace)
func (s *NetworkPolicyReconcilerTestSuite) TestTwoClientsFromDifferentNamespacesWithSameServer() {
	clientIntentsName := "client-intents"
	otherClientIntentsName := "other-client-intents"
	otherClientIntentsNamespace := "other-namespace"
	serviceName := "test-client"
	serverNamespace := testNamespace
	policyName := "test-server-access"
	formattedTargetServer := "test-server-test-namespace-8ddecb"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev2alpha1.IntentsSpec{
		Workload: otterizev2alpha1.Workload{Name: serviceName},
		Targets: []otterizev2alpha1.Target{
			{
				Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: serverName},
			},
		},
	}

	clientIntentsObj := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientIntentsName,
			Namespace: testNamespace,
		},
		Spec: intentsSpec,
	}

	otherClientIntentsDifferentNamespace := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      otherClientIntentsName,
			Namespace: otherClientIntentsNamespace,
		},
		Spec: &otterizev2alpha1.IntentsSpec{
			Workload: otterizev2alpha1.Workload{Name: "other-client"},
			Targets: []otterizev2alpha1.Target{
				{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: serverName},
				},
			},
		},
	}

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      policyName,
	}

	newPolicy := networkPolicyIngressTemplate(
		policyName,
		serverNamespace,
		formattedTargetServer,
		testNamespace,
		otherClientIntentsNamespace,
	)

	s.expectGetAllEffectivePolicies([]otterizev2alpha1.ClientIntents{clientIntentsObj, otherClientIntentsDifferentNamespace})
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).Return(apierrors.NewNotFound(v1.Resource("networkpolicy"), networkPolicyNamespacedName.Name))
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(newPolicy)).Return(nil)
	selector, err := metav1.LabelSelectorAsSelector(&newPolicy.Spec.PodSelector)
	s.Require().NoError(err)
	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), serverNamespace, gomock.Eq(selector))
	s.ignoreRemoveOrphan()

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Empty(res)

	// expect two events for the two clients
	s.ExpectEvent(consts.ReasonCreatedNetworkPolicies)
	s.ExpectEvent(consts.ReasonCreatedNetworkPolicies)

}

func TestNetworkPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(NetworkPolicyReconcilerTestSuite))
}
